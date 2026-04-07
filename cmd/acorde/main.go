package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"github.com/amaydixit11/acorde/internal/blob"
	"github.com/amaydixit11/acorde/internal/crdt"
	"github.com/amaydixit11/acorde/internal/sync"
	"github.com/amaydixit11/acorde/pkg/api"
	"github.com/amaydixit11/acorde/pkg/crypto"
	"github.com/amaydixit11/acorde/pkg/engine"
	"github.com/google/uuid"

	p2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

type daemonPeerInfo struct {
	PeerID string   `json:"peer_id"`
	Addrs  []string `json:"addrs"`
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "daemon":
		cmdDaemon(args)
	case "invite":
		cmdInvite(args)
	case "pair":
		cmdPair(args)
	case "init":
		cmdInit(args)
	case "status":
		cmdStatus(args)
	case "export":
		cmdExport(args)
	case "serve":
		cmdServe(args)
	case "add", "get", "list", "update", "delete", "authorize":
		runWithEngine(cmd, args)
	case "help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`acorde - Local-first data engine with P2P sync

Usage: acorde <command> [options]

Commands:
  daemon      Start sync daemon (auto-discovers peers on LAN)
  serve       Start REST API server (--port 7331)
  status      Show vault status (entry count, sync state)
  export      Export all entries to JSON
  add         Add a new entry
  get         Get an entry by ID  
  list        List entries
  update      Update an entry
  delete      Delete an entry
  authorize   Grant write access to a peer
  help        Show this help

Encryption:
  acorde init   Initialize new encrypted vault

Daemon Mode:
  acorde daemon --name node1 --data ~/.acorde-node1
  acorde daemon --name node2 --data ~/.acorde-node2

Entry Commands:
  acorde add --type note --content "Hello World" --tags work,important
  acorde list --type note
  acorde get <uuid>
  acorde update <uuid> --content "Updated"
  acorde delete <uuid>
  acorde authorize <uuid> <peer-id>`)
}

func runWithEngine(cmd string, args []string) {
	// 1. Determine data dir
	dataDir := resolveDataDir("")

	// Check for custom data dir in args.
	for i, arg := range args {
		if arg == "--data" && i+1 < len(args) {
			dataDir = resolveDataDir(args[i+1])
			break
		}
		if strings.HasPrefix(arg, "--data=") {
			dataDir = resolveDataDir(strings.TrimPrefix(arg, "--data="))
			break
		}
	}

	cfg := engine.Config{DataDir: dataDir}

	// 2. Unlock if needed
	keyStore := crypto.NewFileKeyStore(dataDir)
	if keyStore.IsInitialized() {
		fmt.Printf("🔒 Vault is encrypted. Enter password: ")
		password, err := readPassword() // implemented below
		if err != nil {
			fmt.Fprintf(os.Stderr, "\nError reading password: %v\n", err)
			os.Exit(1)
		}
		key, err := keyStore.Unlock(password)
		if err != nil {
			fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
			os.Exit(1)
		}
		cfg.EncryptionKey = &key
		fmt.Println("")
	}

	e, err := engine.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer e.Close()

	// 3. Filter global flags from args before passing to subcommands
	subArgs := filterGlobalFlags(args)

	switch cmd {
	case "add":
		cmdAdd(e, subArgs)
	case "get":
		cmdGet(e, subArgs)
	case "list":
		cmdList(e, subArgs)
	case "update":
		cmdUpdate(e, subArgs)
	case "delete":
		cmdDelete(e, subArgs)
	case "authorize":
		cmdAuthorize(e, subArgs)
	}
}

func cmdAuthorize(e engine.Engine, args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: acorde authorize <uuid> <peer-id>")
		os.Exit(1)
	}

	id, err := uuid.Parse(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid UUID %q\n", args[0])
		os.Exit(1)
	}

	peerID := args[1]
	if err := e.GrantWrite(id, peerID); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Authorized peer %s to write entry %s\n", peerID, id)
}

// filterGlobalFlags removes global flags (like --data) and their values from args
func filterGlobalFlags(args []string) []string {
	var filtered []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--data" {
			// Skip flag and its value
			i++
			continue
		}
		if strings.HasPrefix(arg, "--data=") {
			// Skip flag with value attached
			continue
		}
		filtered = append(filtered, arg)
	}
	return filtered
}

// syncableEngine wraps pkg/engine.Engine to implement sync.Syncable
type syncableEngine struct {
	engine.Engine
}

func (s *syncableEngine) GetSyncState() crdt.ReplicaState {
	payload, _ := s.GetSyncPayload()
	var state crdt.ReplicaState
	json.Unmarshal(payload, &state)
	return state
}

func (s *syncableEngine) ApplySyncState(state crdt.ReplicaState, senderPeerID string) error {
	payload, _ := json.Marshal(state)
	return s.Engine.ApplyRemotePayloadFromPeer(payload, senderPeerID)
}

func (s *syncableEngine) GetSyncPayload() ([]byte, error) {
	return s.Engine.GetSyncPayload()
}

type blobStoreAdapter struct {
	impl *blob.Store
}

func (a *blobStoreAdapter) Put(data []byte) (string, error) {
	cid, err := a.impl.Put(data)
	return string(cid), err
}

func (a *blobStoreAdapter) Get(cid string) ([]byte, error) {
	return a.impl.Get(blob.CID(cid))
}

type sysLogger struct {
	label   string
	verbose bool
}

func (l *sysLogger) Debugf(format string, v ...interface{}) {
	if l.verbose {
		log.Printf("[DEBUG] "+format, v...)
	}
}

func (l *sysLogger) Infof(format string, v ...interface{}) {
	log.Printf("[INFO] "+format, v...)
}

func (l *sysLogger) Errorf(format string, v ...interface{}) {
	log.Printf("[ERROR] "+format, v...)
}

func cmdDaemon(args []string) {
	fs := flag.NewFlagSet("daemon", flag.ExitOnError)
	name := fs.String("name", "acorde", "Node name for logging")
	dataDir := fs.String("data", "", "Data directory (default: ~/.acorde)")
	port := fs.Int("port", 0, "Port to listen on (0 = random)")
	apiPort := fs.Int("api-port", 0, "Port for REST API (0 = disabled)")
	enableDHT := fs.Bool("dht", false, "Enable DHT for global peer discovery")
	enableMDNS := fs.Bool("mdns", true, "Enable mDNS for local discovery")
	verbose := fs.Bool("verbose", false, "Enable verbose logging")
	fs.Parse(args)
	*dataDir = resolveDataDir(*dataDir)

	log.Printf("🚀 Starting acorde daemon [%s]...", *name)

	// Load or generate identity key BEFORE creating engine
	// This ensures node_id file exists and contains the P2P PeerID
	privKey, _, err := loadOrGenerateKey(*dataDir)
	if err != nil {
		log.Fatalf("Failed to load identity key: %v", err)
	}

	// Create engine
	cfg := engine.Config{DataDir: *dataDir}
	e, err := engine.New(cfg)
	if err != nil {
		log.Fatalf("Failed to create engine: %v", err)
	}
	defer e.Close()

	// Create sync service
	syncCfg := sync.DefaultConfig()
	if *port > 0 {
		syncCfg.ListenAddrs = []string{fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", *port)}
	}
	syncCfg.Logger = &sysLogger{label: "sync", verbose: *verbose}
	syncCfg.EnableDHT = *enableDHT
	syncCfg.EnableMDNS = *enableMDNS
	syncCfg.PrivateKey = privKey
	syncCfg.AllowlistPath = *dataDir

	adapter := sync.NewEngineAdapter(&syncableEngine{e})
	svc, err := sync.NewP2PService(adapter, syncCfg)
	if err != nil {
		log.Fatalf("Failed to create sync service: %v", err)
	}

	// Start sync
	ctx, cancel := context.WithCancel(context.Background())
	if err := svc.Start(ctx); err != nil {
		log.Fatalf("Failed to start sync: %v", err)
	}
	if err := persistDaemonPeerInfo(*dataDir, svc.GetHost().ID().String(), svc.GetHost().Addrs()); err != nil {
		log.Printf("Warning: failed to persist daemon addresses: %v", err)
	}

	log.Printf("✅ Daemon started! Discovering peers on LAN...")
	log.Printf("📋 Add entries in another terminal:")
	log.Printf("   go run ./cmd/acorde add --type note --content 'Hello!'")

	// Print peers periodically
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		for range ticker.C {
			peers := svc.Peers()
			metrics := svc.Metrics()
			if len(peers) > 0 {
				log.Printf("👥 Connected peers: %d | Syncs: %d success, %d failed",
					len(peers), metrics.SyncSuccesses, metrics.SyncFailures)
			}
		}
	}()

	// Initialize blob store
	blobStore, err := blob.NewStore(*dataDir)
	if err != nil {
		log.Printf("Warning: failed to initialize blob store: %v", err)
	}

	// Start API server if requested
	if *apiPort > 0 {
		apiServer := api.New(e)
		apiServer.Configure(api.Config{
			Identity: func() (string, []string) {
				id := svc.GetHost().ID().String()
				addrs := []string{}
				for _, a := range svc.GetHost().Addrs() {
					addrs = append(addrs, a.String())
				}
				return id, addrs
			},
			Peers: func() []api.PeerInfo {
				peers := svc.Peers()
				res := make([]api.PeerInfo, len(peers))
				for i, p := range peers {
					paddrs := []string{}
					for _, a := range svc.GetHost().Peerstore().Addrs(p) {
						paddrs = append(paddrs, a.String())
					}
					res[i] = api.PeerInfo{
						ID:    p.String(),
						Addrs: paddrs,
					}
				}
				return res
			},
			PeerCount: func() int { return len(svc.Peers()) },
			Invite: func() (string, error) {
				invite, err := sync.CreateInvite(svc.GetHost(), 24*time.Hour)
				if err != nil {
					return "", err
				}
				return invite.Encode()
			},
			Pair: func(code string) error {
				invite, err := sync.ParseInvite(code)
				if err != nil {
					return err
				}
				return svc.ConnectPeer(invite)
			},
			Blobs: &blobStoreAdapter{blobStore},
		})

		go func() {
			log.Printf("🚀 Starting API server on http://localhost:%d", *apiPort)
			if err := apiServer.ListenAndServe(fmt.Sprintf(":%d", *apiPort)); err != nil {
				log.Printf("API Server error: %v", err)
			}
		}()
	}

	// Wait for interrupt
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Printf("🛑 Shutting down...")
	cancel()
	svc.Stop()
	log.Printf("👋 Goodbye!")
}

func cmdAdd(e engine.Engine, args []string) {
	fs := flag.NewFlagSet("add", flag.ExitOnError)
	typeStr := fs.String("type", "note", "Entry type")
	content := fs.String("content", "", "Entry content")
	tagsStr := fs.String("tags", "", "Comma-separated tags")
	public := fs.Bool("public", false, "Make entry public (readable by everyone)")
	fs.Parse(args)

	var tags []string
	if *tagsStr != "" {
		tags = strings.Split(*tagsStr, ",")
		for i, t := range tags {
			tags[i] = strings.TrimSpace(t)
		}
	}

	entryType := engine.EntryType(*typeStr)
	entry, err := e.AddEntry(engine.AddEntryInput{
		Type:    entryType,
		Content: []byte(*content),
		Tags:    tags,
		Public:  *public,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	printEntry(entry)
}

func cmdGet(e engine.Engine, args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: acorde get <uuid>")
		os.Exit(1)
	}
	id, err := uuid.Parse(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid UUID %q\n", args[0])
		os.Exit(1)
	}
	entry, err := e.GetEntry(id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	printEntry(entry)
}

func cmdList(e engine.Engine, args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	typeStr := fs.String("type", "", "Filter by type")
	tag := fs.String("tag", "", "Filter by tag")
	fs.Parse(args)

	filter := engine.ListFilter{}
	if *typeStr != "" {
		t := engine.EntryType(*typeStr)
		filter.Type = &t
	}
	if *tag != "" {
		filter.Tag = tag
	}

	entries, err := e.ListEntries(filter)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if len(entries) == 0 {
		fmt.Println("No entries found.")
		return
	}
	for _, entry := range entries {
		fmt.Printf("%s [%s] %s\n", entry.ID.String(), entry.Type, string(entry.Content)[:min(40, len(entry.Content))])
	}
}

func cmdUpdate(e engine.Engine, args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: acorde update <uuid> --content <new>")
		os.Exit(1)
	}
	id, err := uuid.Parse(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid UUID %q\n", args[0])
		os.Exit(1)
	}
	fs := flag.NewFlagSet("update", flag.ExitOnError)
	content := fs.String("content", "", "New content")
	fs.Parse(args[1:])

	input := engine.UpdateEntryInput{}
	if *content != "" {
		c := []byte(*content)
		input.Content = &c
	}
	if err := e.UpdateEntry(id, input); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Updated.")
}

func cmdDelete(e engine.Engine, args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: acorde delete <uuid>")
		os.Exit(1)
	}
	id, err := uuid.Parse(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid UUID %q\n", args[0])
		os.Exit(1)
	}
	if err := e.DeleteEntry(id); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Deleted.")
}

func printEntry(entry engine.Entry) {
	data := map[string]interface{}{
		"id":      entry.ID.String(),
		"type":    string(entry.Type),
		"content": string(entry.Content),
		"tags":    entry.Tags,
	}
	out, _ := json.MarshalIndent(data, "", "  ")
	fmt.Println(string(out))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func cmdInvite(args []string) {
	fs := flag.NewFlagSet("invite", flag.ExitOnError)
	dataDir := fs.String("data", "", "Data directory")
	expiry := fs.Duration("expiry", 24*time.Hour, "Invite expiry duration")
	port := fs.Int("port", 0, "Port to listen/advertise (0 = random)")
	verbose := fs.Bool("verbose", false, "Enable verbose logging")
	fs.Parse(args)
	*dataDir = resolveDataDir(*dataDir)

	cfg := engine.Config{DataDir: *dataDir}

	// Load identity key (must match daemon if running)
	privKey, peerID, err := loadOrGenerateKey(cfg.DataDir)
	if err != nil {
		log.Fatalf("Failed to load identity key: %v", err)
	}

	var invite *sync.PeerInvite
	if info, err := loadDaemonPeerInfo(*dataDir); err == nil && info.PeerID == peerID.String() && len(info.Addrs) > 0 {
		invite, err = sync.CreateInviteForIdentity(peerID, privKey, info.Addrs, *expiry)
		if err != nil {
			log.Fatalf("Failed to create invite from daemon addresses: %v", err)
		}
	} else {
		e, err := engine.New(cfg)
		if err != nil {
			log.Fatalf("Error: %v", err)
		}
		defer e.Close()

		// Create sync service just for the host
		syncCfg := sync.DefaultConfig()
		if *port > 0 {
			syncCfg.ListenAddrs = []string{fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", *port)}
		}
		syncCfg.EnableMDNS = false
		syncCfg.Logger = &sysLogger{label: "sync", verbose: *verbose}
		syncCfg.PrivateKey = privKey

		provider := sync.NewEngineAdapter(&syncableEngine{e})
		svc, err := sync.NewP2PService(provider, syncCfg)
		if err != nil {
			log.Fatalf("Failed to create service: %v", err)
		}
		defer svc.Stop()

		invite, err = sync.CreateInvite(svc.GetHost(), *expiry)
		if err != nil {
			log.Fatalf("Failed to create invite: %v", err)
		}
	}

	// If encrypted, include key
	store := crypto.NewFileKeyStore(cfg.DataDir)
	if store.IsInitialized() {
		fmt.Printf("🔒 Vault is encrypted. Enter password to include key in invite: ")
		password, err := readPassword()
		if err != nil {
			log.Fatalf("\nError: %v", err)
		}
		fmt.Println("")

		key, err := store.Unlock(password)
		if err != nil {
			log.Fatalf("Failed to unlock: %v", err)
		}
		invite.Key = key[:]
	}

	// Print QR code
	qrStr, err := invite.ToQRString()
	if err == nil {
		fmt.Println(qrStr)
	}

	// Print minimal code
	fmt.Printf("\nInvite code: %s\n", invite.ToMinimalCode())
	fmt.Printf("Expires in: %s\n", invite.ExpiresIn().Round(time.Minute))

	// Also print full code for copy/paste
	fullCode, _ := invite.Encode()
	fmt.Printf("\nFull code (for CLI): %s\n", fullCode)
}

func cmdPair(args []string) {
	fs := flag.NewFlagSet("pair", flag.ExitOnError)
	dataDir := fs.String("data", "", "Data directory")
	verbose := fs.Bool("verbose", false, "Enable verbose logging")
	fs.Parse(args)
	*dataDir = resolveDataDir(*dataDir)

	if fs.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "Usage: acorde pair [options] <invite-code>\n")
		// Also support <invite-code> [options] if passed that way, though standard flag requires options first
		os.Exit(1)
	}
	inviteCode := fs.Arg(0)

	// Load allowlist/engine
	cfg := engine.Config{DataDir: *dataDir}
	e, err := engine.New(cfg)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	defer e.Close()

	// Create sync service
	syncCfg := sync.DefaultConfig()
	if *dataDir != "" {
		syncCfg.AllowlistPath = *dataDir // Use data dir name for peer file location
	}
	syncCfg.Logger = &sysLogger{label: "sync", verbose: *verbose}

	// Load identity key to ensure we match the daemon's ID
	privKey, _, err := loadOrGenerateKey(cfg.DataDir)
	if err != nil {
		log.Fatalf("Failed to load identity key: %v", err)
	}
	syncCfg.PrivateKey = privKey

	provider := sync.NewEngineAdapter(&syncableEngine{e})
	svc, err := sync.NewP2PService(provider, syncCfg)
	if err != nil {
		log.Fatalf("Failed to create service: %v", err)
	}
	defer svc.Stop()

	// Start service to allow connection
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.Start(ctx); err != nil {
		log.Fatalf("Failed to start service: %v", err)
	}

	// Parse invite
	invite, err := sync.ParseInvite(inviteCode)
	if err != nil {
		log.Fatalf("Invalid invite: %v", err)
	}

	// Handle key if present
	if len(invite.Key) > 0 {
		store := crypto.NewFileKeyStore(cfg.DataDir)
		if !store.IsInitialized() {
			fmt.Printf("🔑 Invite contains encryption key. Set a password to protect it: ")
			pass1, err := readPassword()
			if err != nil {
				log.Fatalf("\nError: %v", err)
			}
			fmt.Printf("\nConfirm password: ")
			pass2, err := readPassword()
			if err != nil {
				log.Fatalf("\nError: %v", err)
			}
			fmt.Println("")

			if string(pass1) != string(pass2) {
				log.Fatalf("Passwords do not match")
			}

			var key crypto.Key
			if len(invite.Key) != crypto.KeySize {
				log.Fatalf("Invalid key size in invite")
			}
			copy(key[:], invite.Key)

			if err := store.InitializeWithKey(pass1, key); err != nil {
				log.Fatalf("Failed to initialize vault with key: %v", err)
			}
			fmt.Println("✅ Vault initialized with imported key.")
		}
	}

	fmt.Printf("Connecting to peer %s...\n", invite.PeerID)

	// Connect
	if err := svc.ConnectPeer(invite); err != nil {
		if strings.Contains(err.Error(), "failed to connect to peer") {
			fmt.Printf("⚠️  Peer saved locally, but immediate connection failed: %v\n", err)
			fmt.Printf("Peer added to allowlist. Start daemon to begin syncing.\n")
			return
		}
		log.Fatalf("Failed to pair: %v", err)
	}

	fmt.Printf("✅ Successfully paired and connected!\n")
	fmt.Printf("Peer added to allowlist. Start daemon to begin syncing.\n")
}

func cmdInit(args []string) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	dataDir := fs.String("data", "", "Data directory")
	fs.Parse(args)

	dir := *dataDir
	dir = resolveDataDir(dir)

	store := crypto.NewFileKeyStore(dir)
	if store.IsInitialized() {
		fmt.Println("Vault already initialized.")
		return
	}

	fmt.Printf("Enter new password: ")
	pass1, err := readPassword()
	if err != nil {
		log.Fatalf("\nError reading password: %v", err)
	}
	fmt.Printf("\nConfirm password: ")
	pass2, err := readPassword()
	if err != nil {
		log.Fatalf("\nError reading password: %v", err)
	}
	fmt.Println("")

	if string(pass1) != string(pass2) {
		fmt.Println("Passwords do not match!")
		os.Exit(1)
	}

	if err := store.Initialize(pass1); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Vault initialized at %s\n", dir)
}

func readPassword() ([]byte, error) {
	fd := int(syscall.Stdin)
	if !term.IsTerminal(fd) {
		// Fallback for non-interactive
		var password string
		fmt.Scanln(&password)
		return []byte(password), nil
	}
	return term.ReadPassword(fd)
}

func cmdStatus(args []string) {
	dataDir := resolveDataDir("")

	for i, arg := range args {
		if arg == "--data" && i+1 < len(args) {
			dataDir = resolveDataDir(args[i+1])
			break
		}
	}

	cfg := engine.Config{DataDir: dataDir}

	// Try to unlock if encrypted
	store := crypto.NewFileKeyStore(dataDir)
	if store.IsInitialized() {
		fmt.Print("🔒 Vault is encrypted. Enter password: ")
		password, err := readPassword()
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println()

		key, err := store.Unlock(password)
		if err != nil {
			log.Fatalf("Failed to unlock: %v", err)
		}
		cfg.EncryptionKey = &key
	}

	e, err := engine.New(cfg)
	if err != nil {
		log.Fatalf("Failed to open vault: %v", err)
	}
	defer e.Close()

	entries, _ := e.ListEntries(engine.ListFilter{})

	fmt.Println("📊 Vault Status")
	fmt.Println("───────────────")
	fmt.Printf("  Local Peer ID: %s\n", e.PeerID())
	fmt.Printf("  Data Dir:      %s\n", dataDir)
	fmt.Printf("  Encrypted:     %v\n", store.IsInitialized())
	fmt.Printf("  Entries:       %d\n", len(entries))
}

func cmdExport(args []string) {
	dataDir := resolveDataDir("")
	outputFile := "acorde-export.json"

	for i, arg := range args {
		if arg == "--data" && i+1 < len(args) {
			dataDir = resolveDataDir(args[i+1])
		}
		if arg == "--file" && i+1 < len(args) {
			outputFile = args[i+1]
		}
	}

	cfg := engine.Config{DataDir: dataDir}

	store := crypto.NewFileKeyStore(dataDir)
	if store.IsInitialized() {
		fmt.Print("🔒 Vault is encrypted. Enter password: ")
		password, err := readPassword()
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println()

		key, err := store.Unlock(password)
		if err != nil {
			log.Fatalf("Failed to unlock: %v", err)
		}
		cfg.EncryptionKey = &key
	}

	e, err := engine.New(cfg)
	if err != nil {
		log.Fatalf("Failed to open vault: %v", err)
	}
	defer e.Close()

	entries, _ := e.ListEntries(engine.ListFilter{})

	// Export as JSON
	type exportEntry struct {
		ID        string   `json:"id"`
		Type      string   `json:"type"`
		Content   string   `json:"content"`
		Tags      []string `json:"tags"`
		CreatedAt uint64   `json:"created_at"`
		UpdatedAt uint64   `json:"updated_at"`
	}

	export := make([]exportEntry, len(entries))
	for i, e := range entries {
		export[i] = exportEntry{
			ID:        e.ID.String(),
			Type:      string(e.Type),
			Content:   string(e.Content),
			Tags:      e.Tags,
			CreatedAt: e.CreatedAt,
			UpdatedAt: e.UpdatedAt,
		}
	}

	data, _ := json.MarshalIndent(export, "", "  ")
	if err := os.WriteFile(outputFile, data, 0600); err != nil {
		log.Fatalf("Failed to write export: %v", err)
	}

	fmt.Printf("✅ Exported %d entries to %s\n", len(entries), outputFile)
}

func cmdServe(args []string) {
	dataDir := resolveDataDir("")
	port := "7331"

	for i, arg := range args {
		if arg == "--data" && i+1 < len(args) {
			dataDir = resolveDataDir(args[i+1])
		}
		if arg == "--port" && i+1 < len(args) {
			port = args[i+1]
		}
	}

	cfg := engine.Config{DataDir: dataDir}

	store := crypto.NewFileKeyStore(dataDir)
	if store.IsInitialized() {
		fmt.Print("🔒 Vault is encrypted. Enter password: ")
		password, err := readPassword()
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println()

		key, err := store.Unlock(password)
		if err != nil {
			log.Fatalf("Failed to unlock: %v", err)
		}
		cfg.EncryptionKey = &key
	}

	e, err := engine.New(cfg)
	if err != nil {
		log.Fatalf("Failed to open vault: %v", err)
	}
	defer e.Close()

	// Import api package
	apiServer := api.New(e)

	fmt.Printf("🚀 Starting API server on http://localhost:%s\n", port)
	fmt.Printf("   GET    /entries\n")
	fmt.Printf("   POST   /entries\n")
	fmt.Printf("   GET    /entries/:id\n")
	fmt.Printf("   PUT    /entries/:id\n")
	fmt.Printf("   DELETE /entries/:id\n")
	fmt.Printf("   GET    /status\n")
	fmt.Printf("   GET    /events (SSE)\n")

	if err := apiServer.ListenAndServe(":" + port); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

// loadOrGenerateKey loads the private key from disk or generates a new one.
// It also ensures node_id file matches the key.
func loadOrGenerateKey(dataDir string) (p2pcrypto.PrivKey, peer.ID, error) {
	dataDir = resolveDataDir(dataDir)
	keyPath := filepath.Join(dataDir, "node_key")
	nodeIDPath := filepath.Join(dataDir, "node_id")

	// Try to load key
	if keyBytes, err := os.ReadFile(keyPath); err == nil {
		privKey, err := p2pcrypto.UnmarshalPrivateKey(keyBytes)
		if err != nil {
			return nil, "", fmt.Errorf("failed to unmarshal key: %w", err)
		}

		id, err := peer.IDFromPrivateKey(privKey)
		if err != nil {
			return nil, "", fmt.Errorf("failed to derive peer ID: %w", err)
		}

		// Ensure node_id file exists and matches key
		os.WriteFile(nodeIDPath, []byte(id.String()), 0644)
		return privKey, id, nil
	}

	// Generate new key
	privKey, _, err := p2pcrypto.GenerateKeyPair(p2pcrypto.Ed25519, -1)
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate key: %w", err)
	}

	// Save key
	keyBytes, err := p2pcrypto.MarshalPrivateKey(privKey)
	if err != nil {
		return nil, "", fmt.Errorf("failed to marshal key: %w", err)
	}

	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, "", fmt.Errorf("failed to create data dir: %w", err)
	}

	// Set stricter permissions for private key
	if err := os.WriteFile(keyPath, keyBytes, 0600); err != nil {
		return nil, "", fmt.Errorf("failed to write key: %w", err)
	}

	// Save PeerID to node_id file for Engine conformance
	id, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		return nil, "", fmt.Errorf("failed to derive peer ID: %w", err)
	}

	if err := os.WriteFile(nodeIDPath, []byte(id.String()), 0644); err != nil {
		log.Printf("Warning: failed to write node_id: %v", err)
	}

	return privKey, id, nil
}

func resolveDataDir(dataDir string) string {
	if dataDir != "" {
		return dataDir
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ".acorde"
	}

	return filepath.Join(home, ".acorde")
}

func daemonPeerInfoPath(dataDir string) string {
	return filepath.Join(resolveDataDir(dataDir), "peer_addrs.json")
}

func persistDaemonPeerInfo(dataDir string, peerID string, addrs []multiaddr.Multiaddr) error {
	info := daemonPeerInfo{
		PeerID: peerID,
		Addrs:  make([]string, 0, len(addrs)),
	}
	for _, addr := range addrs {
		info.Addrs = append(info.Addrs, addr.String())
	}
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}
	return os.WriteFile(daemonPeerInfoPath(dataDir), data, 0644)
}

func loadDaemonPeerInfo(dataDir string) (*daemonPeerInfo, error) {
	data, err := os.ReadFile(daemonPeerInfoPath(dataDir))
	if err != nil {
		return nil, err
	}
	var info daemonPeerInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}
	return &info, nil
}
