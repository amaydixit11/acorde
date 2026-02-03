package sync

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	gosync "sync"
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

	mdnsService mdns.Service
	peers       map[peer.ID]struct{}
	peersMu     gosync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc
	wg     gosync.WaitGroup
}

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

	return &p2pService{
		host:     h,
		provider: provider,
		config:   cfg,
		peers:    make(map[peer.ID]struct{}),
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
	}

	// Start periodic sync
	s.wg.Add(1)
	go s.syncLoop()

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

// SyncWith triggers a sync with a specific peer
func (s *p2pService) SyncWith(ctx context.Context, peerID peer.ID) error {
	// Open stream to peer
	stream, err := s.host.NewStream(ctx, peerID, protocol.ID(ProtocolID))
	if err != nil {
		return fmt.Errorf("failed to open stream: %w", err)
	}
	defer stream.Close()

	// Set deadline
	stream.SetDeadline(time.Now().Add(30 * time.Second))

	// Send our state hash
	hash := s.provider.StateHash()
	msg := &Message{
		Type:      MsgStateHash,
		StateHash: hash,
	}

	if err := writeMessage(stream, msg); err != nil {
		return fmt.Errorf("failed to send state hash: %w", err)
	}

	// Read response
	resp, err := readMessage(stream)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// Handle response
	switch resp.Type {
	case MsgStateHash:
		// Hashes match, nothing to do
		return nil

	case MsgState:
		// Apply remote state
		var state crdt.ReplicaState
		if err := json.Unmarshal(resp.State, &state); err != nil {
			return fmt.Errorf("failed to decode state: %w", err)
		}
		return s.provider.ApplyState(state)
	}

	return nil
}

// HandlePeerFound is called by mDNS when a peer is discovered
func (s *p2pService) HandlePeerFound(pi peer.AddrInfo) {
	// Skip self
	if pi.ID == s.host.ID() {
		return
	}

	s.peersMu.Lock()
	s.peers[pi.ID] = struct{}{}
	s.peersMu.Unlock()

	// Connect to peer
	if err := s.host.Connect(s.ctx, pi); err != nil {
		// Connection failed, remove from peers
		s.peersMu.Lock()
		delete(s.peers, pi.ID)
		s.peersMu.Unlock()
		return
	}

	// Trigger sync
	go s.SyncWith(s.ctx, pi.ID)
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
		if string(msg.StateHash) == string(ourHash) {
			// Hashes match
			resp = &Message{
				Type:      MsgStateHash,
				StateHash: ourHash,
			}
		} else {
			// Hashes differ, send our state
			state := s.provider.GetState()
			stateData, _ := json.Marshal(state)
			resp = &Message{
				Type:  MsgState,
				State: stateData,
			}
		}

	case MsgStateRequest:
		// Send full state
		state := s.provider.GetState()
		stateData, _ := json.Marshal(state)
		resp = &Message{
			Type:  MsgState,
			State: stateData,
		}

	case MsgState:
		// Apply incoming state
		var state crdt.ReplicaState
		if err := json.Unmarshal(msg.State, &state); err == nil {
			s.provider.ApplyState(state)
		}
		resp = &Message{
			Type:      MsgStateHash,
			StateHash: s.provider.StateHash(),
		}
	}

	if resp != nil {
		writeMessage(stream, resp)
	}
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
				go s.SyncWith(s.ctx, peerID)
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
