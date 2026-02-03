package sync

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	gosync "sync"
	"sync/atomic"
	"time"

	"github.com/amaydixit11/vaultd/internal/crdt"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	"github.com/multiformats/go-multiaddr"
)

// p2pService implements SyncService using libp2p
type p2pService struct {
	host     host.Host
	provider StateProvider
	config   Config
	logger   Logger

	mdnsService  mdns.Service
	dhtDiscovery *DHTDiscovery
	peers        map[peer.ID]struct{}
	peersMu      gosync.RWMutex

	// Active sync sessions to prevent duplicates
	activeSyncs   map[string]struct{}
	activeSyncsMu gosync.Mutex

	// Metrics
	syncAttempts  int64
	syncSuccesses int64
	syncFailures  int64

	ctx    context.Context
	cancel context.CancelFunc
	wg     gosync.WaitGroup
}

// noopLogger is a no-op logger
type noopLogger struct{}

func (noopLogger) Printf(format string, v ...interface{}) {}

// NewP2PService creates a new libp2p-based sync service
func NewP2PService(provider StateProvider, cfg Config) (SyncService, error) {
	// Parse listen addresses
	listenAddrs := make([]multiaddr.Multiaddr, len(cfg.ListenAddrs))
	for i, addr := range cfg.ListenAddrs {
		ma, err := multiaddr.NewMultiaddr(addr)
		if err != nil {
			return nil, fmt.Errorf("invalid listen address %s: %w", addr, err)
		}
		listenAddrs[i] = ma
	}

	// Create libp2p host
	h, err := libp2p.New(
		libp2p.ListenAddrs(listenAddrs...),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create libp2p host: %w", err)
	}

	logger := cfg.Logger
	if logger == nil {
		logger = noopLogger{}
	}

	return &p2pService{
		host:        h,
		provider:    provider,
		config:      cfg,
		logger:      logger,
		peers:       make(map[peer.ID]struct{}),
		activeSyncs: make(map[string]struct{}),
	}, nil
}

// Start begins listening and discovering peers
func (s *p2pService) Start(ctx context.Context) error {
	s.ctx, s.cancel = context.WithCancel(ctx)

	// Register protocol handler
	s.host.SetStreamHandler(protocol.ID(ProtocolID), s.handleStream)

	// Start mDNS discovery
	if s.config.EnableMDNS {
		mdnsService := mdns.NewMdnsService(s.host, ServiceName, s)
		if err := mdnsService.Start(); err != nil {
			return fmt.Errorf("failed to start mDNS: %w", err)
		}
		s.mdnsService = mdnsService
		s.logger.Printf("mDNS discovery enabled")
	}

	// Start DHT discovery
	if s.config.EnableDHT {
		bootstrapPeers := GetDefaultBootstrapPeers()
		dhtDiscovery, err := NewDHTDiscovery(s.host, bootstrapPeers, s.logger)
		if err != nil {
			return fmt.Errorf("failed to create DHT: %w", err)
		}
		if err := dhtDiscovery.Start(s.HandlePeerFound); err != nil {
			return fmt.Errorf("failed to start DHT: %w", err)
		}
		s.dhtDiscovery = dhtDiscovery
		s.logger.Printf("DHT discovery enabled (global)")
	}

	// Start periodic sync
	s.wg.Add(1)
	go s.syncLoop()

	s.logger.Printf("sync service started, listening on %v", s.host.Addrs())
	return nil
}

// Stop gracefully shuts down the service
func (s *p2pService) Stop() error {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()

	if s.mdnsService != nil {
		s.mdnsService.Close()
	}

	if s.dhtDiscovery != nil {
		s.dhtDiscovery.Stop()
	}

	return s.host.Close()
}

// Peers returns the list of connected peers
func (s *p2pService) Peers() []peer.ID {
	s.peersMu.RLock()
	defer s.peersMu.RUnlock()

	result := make([]peer.ID, 0, len(s.peers))
	for p := range s.peers {
		result = append(result, p)
	}
	return result
}

// Metrics returns sync statistics
func (s *p2pService) Metrics() SyncMetrics {
	return SyncMetrics{
		SyncAttempts:  atomic.LoadInt64(&s.syncAttempts),
		SyncSuccesses: atomic.LoadInt64(&s.syncSuccesses),
		SyncFailures:  atomic.LoadInt64(&s.syncFailures),
	}
}

// SyncWith triggers a sync with a specific peer
func (s *p2pService) SyncWith(ctx context.Context, peerID peer.ID) error {
	atomic.AddInt64(&s.syncAttempts, 1)

	// Generate session ID
	sessionID := GenerateSessionID()

	// Check for active sync with this peer
	// Session ID prevents duplicate syncs
	s.activeSyncsMu.Lock()
	if _, active := s.activeSyncs[peerID.String()]; active {
		s.activeSyncsMu.Unlock()
		return nil // Already syncing with this peer
	}
	s.activeSyncs[peerID.String()] = struct{}{}
	s.activeSyncsMu.Unlock()

	defer func() {
		s.activeSyncsMu.Lock()
		delete(s.activeSyncs, peerID.String())
		s.activeSyncsMu.Unlock()
	}()

	// Open stream to peer
	stream, err := s.host.NewStream(ctx, peerID, protocol.ID(ProtocolID))
	if err != nil {
		atomic.AddInt64(&s.syncFailures, 1)
		return fmt.Errorf("failed to open stream: %w", err)
	}
	defer stream.Close()

	// Set deadline
	stream.SetDeadline(time.Now().Add(30 * time.Second))

	// Send our state hash with session ID
	hash := s.provider.StateHash()
	msg := &Message{
		Type:      MsgStateHash,
		SessionID: sessionID,
		StateHash: hash,
	}

	if err := writeMessage(stream, msg); err != nil {
		atomic.AddInt64(&s.syncFailures, 1)
		return fmt.Errorf("failed to send state hash: %w", err)
	}

	// Read response
	resp, err := readMessage(stream)
	if err != nil {
		atomic.AddInt64(&s.syncFailures, 1)
		return fmt.Errorf("failed to read response: %w", err)
	}

	// Handle response
	switch resp.Type {
	case MsgStateHash:
		// Hashes match, nothing to do
		atomic.AddInt64(&s.syncSuccesses, 1)
		return nil

	case MsgState:
		// Apply remote state
		var state crdt.ReplicaState
		if err := json.Unmarshal(resp.State, &state); err != nil {
			atomic.AddInt64(&s.syncFailures, 1)
			return fmt.Errorf("failed to decode state: %w", err)
		}
		if err := s.provider.ApplyState(state); err != nil {
			atomic.AddInt64(&s.syncFailures, 1)
			return err
		}
		atomic.AddInt64(&s.syncSuccesses, 1)
		s.logger.Printf("synced with peer %s", peerID.String()[:8])
		return nil

	case MsgStateRequest:
		// They want our state - send it
		state := s.provider.GetState()
		stateData, _ := json.Marshal(state)
		stateMsg := &Message{
			Type:      MsgState,
			SessionID: sessionID,
			State:     stateData,
		}
		if err := writeMessage(stream, stateMsg); err != nil {
			atomic.AddInt64(&s.syncFailures, 1)
			return fmt.Errorf("failed to send state: %w", err)
		}
		atomic.AddInt64(&s.syncSuccesses, 1)
		return nil
	}

	atomic.AddInt64(&s.syncSuccesses, 1)
	return nil
}

// HandlePeerFound is called by mDNS when a peer is discovered
func (s *p2pService) HandlePeerFound(pi peer.AddrInfo) {
	// Skip self
	if pi.ID == s.host.ID() {
		return
	}

	s.peersMu.Lock()
	_, exists := s.peers[pi.ID]
	s.peers[pi.ID] = struct{}{}
	s.peersMu.Unlock()

	if !exists {
		s.logger.Printf("discovered peer %s", pi.ID.String()[:8])
	}

	// Connect to peer
	if err := s.host.Connect(s.ctx, pi); err != nil {
		// Connection failed, remove from peers
		s.peersMu.Lock()
		delete(s.peers, pi.ID)
		s.peersMu.Unlock()
		return
	}

	// Trigger sync
	go func() {
		if err := s.SyncWith(s.ctx, pi.ID); err != nil {
			s.logger.Printf("sync with %s failed: %v", pi.ID.String()[:8], err)
		}
	}()
}

// handleStream handles incoming sync requests
func (s *p2pService) handleStream(stream network.Stream) {
	defer stream.Close()

	// Set deadline
	stream.SetDeadline(time.Now().Add(30 * time.Second))

	// Read incoming message
	msg, err := readMessage(stream)
	if err != nil {
		return
	}

	var resp *Message

	switch msg.Type {
	case MsgStateHash:
		// Compare hashes
		ourHash := s.provider.StateHash()
		theirHash := msg.StateHash

		if string(ourHash) == string(theirHash) {
			// States identical - respond with our hash as acknowledgement
			resp = &Message{
				Type:      MsgStateHash,
				SessionID: msg.SessionID,
				StateHash: ourHash,
			}
		} else {
			// Hashes differ - send our full state
			// CRDT merge will combine both states correctly
			state := s.provider.GetState()
			stateData, _ := json.Marshal(state)
			resp = &Message{
				Type:      MsgState,
				SessionID: msg.SessionID,
				State:     stateData,
			}
		}

	case MsgStateRequest:
		// Send full state
		state := s.provider.GetState()
		stateData, _ := json.Marshal(state)
		resp = &Message{
			Type:      MsgState,
			SessionID: msg.SessionID,
			State:     stateData,
		}

	case MsgState:
		// Apply incoming state
		var state crdt.ReplicaState
		if err := json.Unmarshal(msg.State, &state); err == nil {
			s.provider.ApplyState(state)
		}
		resp = &Message{
			Type:      MsgStateHash,
			SessionID: msg.SessionID,
			StateHash: s.provider.StateHash(),
		}
	}

	if resp != nil {
		writeMessage(stream, resp)
	}
}

// shouldSendState determines which peer should send state (deterministic tie-breaker)
func shouldSendState(ourHash, theirHash []byte) bool {
	// Compare hashes lexicographically - higher value sends state
	for i := 0; i < len(ourHash) && i < len(theirHash); i++ {
		if ourHash[i] > theirHash[i] {
			return true
		}
		if ourHash[i] < theirHash[i] {
			return false
		}
	}
	return len(ourHash) > len(theirHash)
}

// syncLoop periodically syncs with all peers
func (s *p2pService) syncLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.config.SyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			for _, peerID := range s.Peers() {
				peerID := peerID // Capture for goroutine
				go func() {
					if err := s.SyncWith(s.ctx, peerID); err != nil {
						s.logger.Printf("periodic sync with %s failed: %v", peerID.String()[:8], err)
					}
				}()
			}
		}
	}
}

// writeMessage writes a length-prefixed message to the stream
func writeMessage(w io.Writer, msg *Message) error {
	data, err := msg.Encode()
	if err != nil {
		return err
	}

	// Write 4-byte length prefix
	length := uint32(len(data))
	if err := binary.Write(w, binary.BigEndian, length); err != nil {
		return err
	}

	// Write message
	_, err = w.Write(data)
	return err
}

// readMessage reads a length-prefixed message from the stream
func readMessage(r io.Reader) (*Message, error) {
	// Read 4-byte length prefix
	var length uint32
	if err := binary.Read(r, binary.BigEndian, &length); err != nil {
		return nil, err
	}

	// Sanity check
	if length > 10*1024*1024 { // 10MB max
		return nil, fmt.Errorf("message too large: %d bytes", length)
	}

	// Read message
	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, err
	}

	return DecodeMessage(data)
}

// ComputeStateHash computes a hash of the replica state
func ComputeStateHash(state crdt.ReplicaState) []byte {
	data, _ := json.Marshal(state)
	hash := sha256.Sum256(data)
	return hash[:]
}
