package sync

import (
	"context"
	"fmt"
	gosync "sync"
	"time"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	drouting "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	dutil "github.com/libp2p/go-libp2p/p2p/discovery/util"
)

// RendezvousNamespace is the namespace for acorde peer discovery
const RendezvousNamespace = "/acorde/1.0.0"

// DHTDiscovery provides global peer discovery via Kademlia DHT
type DHTDiscovery struct {
	host       host.Host
	dht        *dht.IpfsDHT
	discovery  *drouting.RoutingDiscovery
	logger     Logger
	peerNotify func(peer.AddrInfo)

	ctx    context.Context
	cancel context.CancelFunc
	wg     gosync.WaitGroup
}

// NewDHTDiscovery creates a new DHT-based discovery service
func NewDHTDiscovery(h host.Host, bootstrapPeers []peer.AddrInfo, logger Logger) (*DHTDiscovery, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Create DHT in client mode (not serving records, just discovering)
	kadDHT, err := dht.New(ctx, h,
		dht.Mode(dht.ModeAutoServer),
		dht.BootstrapPeers(bootstrapPeers...),
	)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create DHT: %w", err)
	}

	return &DHTDiscovery{
		host:   h,
		dht:    kadDHT,
		logger: logger,
		ctx:    ctx,
		cancel: cancel,
	}, nil
}

// Start begins DHT bootstrapping and peer discovery
func (d *DHTDiscovery) Start(peerNotify func(peer.AddrInfo)) error {
	d.peerNotify = peerNotify

	// Bootstrap the DHT
	d.logger.Infof("DHT: bootstrapping...")
	if err := d.dht.Bootstrap(d.ctx); err != nil {
		return fmt.Errorf("failed to bootstrap DHT: %w", err)
	}

	// Wait for bootstrap to complete (connect to at least one peer)
	d.wg.Add(1)
	go d.waitForBootstrap()

	return nil
}

// waitForBootstrap waits for DHT to connect to peers, then starts discovery
func (d *DHTDiscovery) waitForBootstrap() {
	defer d.wg.Done()

	// Wait for at least one connection
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	// User requested "DHT Bootstrap Will Fail on Fresh Install".
	// We should probably wait less for interactive feel, or not block.
	// But getting at least one peer is crucial for DHT to work.
	timeout := time.After(15 * time.Second)
	for {
		select {
		case <-d.ctx.Done():
			return
		case <-timeout:
			d.logger.Infof("DHT: bootstrap timeout (0 peers). Discovery may be limited until better connectivity.")
			goto startDiscovery
		case <-ticker.C:
			if len(d.host.Network().Peers()) > 0 {
				d.logger.Infof("DHT: connected to %d peers", len(d.host.Network().Peers()))
				goto startDiscovery
			}
		}
	}

startDiscovery:
	d.discovery = drouting.NewRoutingDiscovery(d.dht)

	// Advertise ourselves
	d.logger.Infof("DHT: advertising at %s", RendezvousNamespace)
	dutil.Advertise(d.ctx, d.discovery, RendezvousNamespace)

	// Start discovering peers
	d.wg.Add(1)
	go d.discoverPeers()
}

// discoverPeers continuously searches for acorde peers
func (d *DHTDiscovery) discoverPeers() {
	defer d.wg.Done()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			d.findPeers()
		}
	}
}

// findPeers searches for acorde peers in the DHT
func (d *DHTDiscovery) findPeers() {
	if d.discovery == nil {
		return
	}

	ctx, cancel := context.WithTimeout(d.ctx, 10*time.Second)
	defer cancel()

	peerCh, err := d.discovery.FindPeers(ctx, RendezvousNamespace)
	if err != nil {
		return
	}

	for pi := range peerCh {
		if pi.ID == d.host.ID() {
			continue // Skip self
		}
		if len(pi.Addrs) == 0 {
			continue // No addresses
		}

		d.logger.Debugf("DHT: found peer %s", pi.ID.String()[:8])
		if d.peerNotify != nil {
			d.peerNotify(pi)
		}
	}
}

// Stop shuts down the DHT discovery
func (d *DHTDiscovery) Stop() error {
	d.cancel()
	d.wg.Wait()
	return d.dht.Close()
}

// GetDefaultBootstrapPeers returns the default IPFS bootstrap peers
func GetDefaultBootstrapPeers() []peer.AddrInfo {
	// Use libp2p's default bootstrap peers (IPFS nodes)
	bootstrapPeers := dht.DefaultBootstrapPeers
	
	result := make([]peer.AddrInfo, 0, len(bootstrapPeers))
	for _, addr := range bootstrapPeers {
		pi, err := peer.AddrInfoFromP2pAddr(addr)
		if err != nil {
			continue
		}
		result = append(result, *pi)
	}
	return result
}
