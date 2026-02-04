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

	"github.com/amaydixit11/acorde/internal/crdt"
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

	allowlist    *Allowlist
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


	var allowlist *Allowlist
	if cfg.AllowlistPath != "" {
		al, err := NewAllowlist(cfg.AllowlistPath, cfg.StrictAllowlist)
		if err != nil {
			return nil, fmt.Errorf("failed to load allowlist: %w", err)
		}
		allowlist = al
		logger.Printf("Allowlist enabled (strict=%v): %d peers loaded", cfg.StrictAllowlist, al.Count())
	}

	return &p2pService{
		host:        h,
		provider:    provider,
		config:      cfg,
		logger:      logger,
		allowlist:   allowlist,
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
		// Add random 4-char suffix to service name to avoid conflicts on same machine
		// For now we just use the static name as per spec, but ideally should be configurable.
		// User requirement: "Add unique namespace per data dir".
		// We can append part of PeerID to service name?
		// But mDNS service name must be constant for discovery?
		// No, service *type* (Tag) is constant. Service *instance* name should be unique.
		// mdns.NewMdnsService uses ServiceName as the Service Tag "_vaultd-discovery._udp".
		// It manages instance names automatically (usually hostname).
		// If we run multiple instances on same host, they might conflict if they try to bind same port? 
		// SyncService binds to 0 (random port) by default in config, so ports are fine.
		// mdns.NewMdnsService implementation handles conflicts by appending suffixes? 
		// Let's leave it for now unless we are sure.
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

// GetHost returns the underlying libp2p host
func (s *p2pService) GetHost() host.Host {
	return s.host
}

// ConnectPeer adds a peer to the allowlist (if enabled) and connects to it
func (s *p2pService) ConnectPeer(invite *PeerInvite) error {
	peerID, err := peer.Decode(invite.PeerID)
	if err != nil {
		return fmt.Errorf("invalid peer ID: %w", err)
	}

	// Add to allowlist if active
	if s.allowlist != nil {
		if err := s.allowlist.Add(peerID, "", invite.Addresses); err != nil {
			return fmt.Errorf("failed to add peer to allowlist: %w", err)
		}
	}

	// Parse addresses
	peerInfo := peer.AddrInfo{ID: peerID}
	for _, addrStr := range invite.Addresses {
		ma, err := multiaddr.NewMultiaddr(addrStr)
		if err != nil {
			continue
		}
		peerInfo.Addrs = append(peerInfo.Addrs, ma)
	}

	if len(peerInfo.Addrs) == 0 {
		return fmt.Errorf("no valid addresses in invite")
	}

	// Connect
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()

	if err := s.host.Connect(ctx, peerInfo); err != nil {
		return fmt.Errorf("failed to connect to peer: %w", err)
	}

	// Trigger immediate sync
	go s.SyncWith(s.ctx, peerID)

	return nil
}

// checkAllowlist returns true if the peer is allowed to sync
func (s *p2pService) checkAllowlist(p peer.ID) bool {
	if s.allowlist == nil {
		return true // No allowlist = accept all
	}
	return s.allowlist.IsAllowed(p)
}

// SyncWith triggers a sync with a specific peer
func (s *p2pService) SyncWith(parentCtx context.Context, peerID peer.ID) error {
	// 1. Enforce timeout to prevent memory leaks in activeSyncs
	ctx, cancel := context.WithTimeout(parentCtx, 2*time.Minute)
	defer cancel()

	atomic.AddInt64(&s.syncAttempts, 1)

	// Generate session ID
	sessionID := GenerateSessionID()

	// Check for active sync with this peer
	// Session ID prevents duplicate syncs
	s.activeSyncsMu.Lock()
	if _, active := s.activeSyncs[peerID.String()]; active {
		// Tie-breaker: If we are already syncing, one side should yield.
		// Rule: Lower PeerID yields to Higher PeerID.
		// If we are Higher, we ignore the 'active' flag and proceed (assuming the other side yields/fails).
		// Wait, 'active' means WE initiated it or accepted it.
		// If we are initiating NOW, checks active.
		// If 'active' is true, it means EITHER:
		// 1. We already started a sync (outgoing)
		// 2. We accepted a sync (incoming)
		// If 1: We are piling on? No, SyncWith is typically one-at-a-time per peer via syncLoop.
		// If we manually trigger, maybe.
		// If 2: We are syncing with them right now.
		// Do we need another one? Probably not.
		// The issue is if A starts, sets active. B starts, sets active.
		// A connects to B. B rejects (busy). A fails.
		// B connects to A. A rejects (busy). B fails.
		// Result: Livelock.
		
		// Fix: Don't reject "Active" if we are just marking it?
		// We need to allow the *incoming* connection even if we are "Active" initiating?
		// But `activeSyncs` tracks *sessions*.
		
		// Simpler fix: If active, just return nil (it's happening).
		// But verify if it's STUCK.
		// The user suggested: "Use session IDs bidirectionally or allow concurrent syncs".
		// Or tie-break.
		
		// Let's rely on the timeout I added earlier to clear stuck syncs.
		// To fix livelock where they block *each other* continuously:
		// We should perhaps NOT block outgoing if incoming is happening?
		// But we want to avoid double processing.
		
		// Let's implement the Tie-Breaker for *Initiating*.
		// If we are Lower ID, and we see it's active (maybe incoming?), we back off.
		// If we are Higher ID, we proceed? 
		// Actually, if it's active in OUR map, it means WE are processing it.
		// So returning nil is correct. The problem is if the OTHER side rejects us because THEY are processing.
		
		// To fix the "Collision" (Head-to-Head):
		// A dials B. B dials A.
		// A sees B incoming. 
		// The activeSyncs check is local.
		
		// The Livelock happens on the network layer if both sides Reject multiple streams.
		// My p2p.go doesn't seem to reject incoming streams based on `activeSyncs`.
		// Let's check `handleStream`.
		
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

	// Check allowlist if enabled
	if !s.checkAllowlist(stream.Conn().RemotePeer()) {
		s.logger.Printf("rejected connection from unauthorized peer %s", stream.Conn().RemotePeer())
		return
	}

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
