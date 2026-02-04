package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"path/filepath"

	"golang.org/x/term"

	"github.com/amaydixit11/vaultd/internal/crdt"
	"github.com/amaydixit11/vaultd/pkg/api"
	"github.com/amaydixit11/vaultd/pkg/crypto"
	"github.com/amaydixit11/vaultd/internal/sync"
	"github.com/amaydixit11/vaultd/pkg/engine"
	"github.com/google/uuid"
)

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
	case "add", "get", "list", "update", "delete":
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
	fmt.Println(`vaultd - Local-first data engine with P2P sync

Usage: vaultd <command> [options]

Commands:
  daemon   Start sync daemon (auto-discovers peers on LAN)
  serve    Start REST API server (--port 8080)
  status   Show vault status (entry count, sync state)
  export   Export all entries to JSON
  add      Add a new entry
  get      Get an entry by ID  
  list     List entries
  update   Update an entry
  delete   Delete an entry
  help     Show this help

Encryption:
  vaultd init   Initialize new encrypted vault

Daemon Mode:
  vaultd daemon --name node1 --data ~/.vaultd-node1
  vaultd daemon --name node2 --data ~/.vaultd-node2

Entry Commands:
  vaultd add --type note --content "Hello World" --tags work,important
  vaultd list --type note
  vaultd get <uuid>
  vaultd update <uuid> --content "Updated"
  vaultd delete <uuid>`)
}

func runWithEngine(cmd string, args []string) {
	// 1. Determine data dir
	// We need to parse args manually or peek at them because flag.Parse consumes them
	// For simplicity, we assume default data dir if not specified, 
	// OR we enforce standard flag usage. Let's stick to default.
	home, _ := os.UserHomeDir()
	dataDir := filepath.Join(home, ".vaultd") 
	
	// Check for custom data dir in args (simple check)
	for i, arg := range args {
		if arg == "--data" && i+1 < len(args) {
			dataDir = args[i+1]
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

	switch cmd {
	case "add":
		cmdAdd(e, args)
	case "get":
		cmdGet(e, args)
	case "list":
		cmdList(e, args)
	case "update":
		cmdUpdate(e, args)
	case "delete":
		cmdDelete(e, args)
	}
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

func (s *syncableEngine) ApplySyncState(state crdt.ReplicaState) error {
	payload, _ := json.Marshal(state)
	return s.ApplyRemotePayload(payload)
}

type stdLogger struct{}

func (stdLogger) Printf(format string, v ...interface{}) {
	log.Printf(format, v...)
}

func cmdDaemon(args []string) {
	fs := flag.NewFlagSet("daemon", flag.ExitOnError)
	name := fs.String("name", "vaultd", "Node name for logging")
	dataDir := fs.String("data", "", "Data directory (default: ~/.vaultd)")
	port := fs.Int("port", 0, "Port to listen on (0 = random)")
	enableDHT := fs.Bool("dht", false, "Enable DHT for global peer discovery")
	fs.Parse(args)

	log.Printf("🚀 Starting vaultd daemon [%s]...", *name)

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
	syncCfg.Logger = stdLogger{}
	syncCfg.EnableDHT = *enableDHT

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

	log.Printf("✅ Daemon started! Discovering peers on LAN...")
	log.Printf("📋 Add entries in another terminal:")
	log.Printf("   go run ./cmd/vaultd add --type note --content 'Hello!'")

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
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	printEntry(entry)
}

func cmdGet(e engine.Engine, args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: vaultd get <uuid>")
		os.Exit(1)
	}
	id, _ := uuid.Parse(args[0])
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
		fmt.Printf("%s [%s] %s\n", entry.ID.String()[:8], entry.Type, string(entry.Content)[:min(40, len(entry.Content))])
	}
}

func cmdUpdate(e engine.Engine, args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: vaultd update <uuid> --content <new>")
		os.Exit(1)
	}
	id, _ := uuid.Parse(args[0])
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
		fmt.Fprintln(os.Stderr, "Usage: vaultd delete <uuid>")
		os.Exit(1)
	}
	id, _ := uuid.Parse(args[0])
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
	if a < b { return a }
	return b
}

func cmdInvite(args []string) {
	fs := flag.NewFlagSet("invite", flag.ExitOnError)
	dataDir := fs.String("data", "", "Data directory")
	expiry := fs.Duration("expiry", 24*time.Hour, "Invite expiry duration")
	fs.Parse(args)

	cfg := engine.Config{DataDir: *dataDir}
	e, err := engine.New(cfg)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	defer e.Close()

	// Create sync service just for the host
	syncCfg := sync.DefaultConfig()
	syncCfg.EnableMDNS = false
	provider := sync.NewEngineAdapter(&syncableEngine{e})
	svc, err := sync.NewP2PService(provider, syncCfg)
	if err != nil {
		log.Fatalf("Failed to create service: %v", err)
	}
	defer svc.Stop()

	// Get the host from the service
	// Use interface method
	invite, err := sync.CreateInvite(svc.GetHost(), *expiry)
	if err != nil {
		log.Fatalf("Failed to create invite: %v", err)
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
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: vaultd pair <invite-code> [options]\n")
		os.Exit(1)
	}
	inviteCode := args[0]
	
	fs := flag.NewFlagSet("pair", flag.ExitOnError)
	dataDir := fs.String("data", "", "Data directory")
	fs.Parse(args[1:])

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
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("Failed to get user home directory: %v", err)
		}
		dir = filepath.Join(home, ".vaultd")
	}

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
	home, _ := os.UserHomeDir()
	dataDir := filepath.Join(home, ".vaultd")

	for i, arg := range args {
		if arg == "--data" && i+1 < len(args) {
			dataDir = args[i+1]
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
	fmt.Printf("  Data Dir:    %s\n", dataDir)
	fmt.Printf("  Encrypted:   %v\n", store.IsInitialized())
	fmt.Printf("  Entries:     %d\n", len(entries))
}

func cmdExport(args []string) {
	home, _ := os.UserHomeDir()
	dataDir := filepath.Join(home, ".vaultd")
	outputFile := "vaultd-export.json"

	for i, arg := range args {
		if arg == "--data" && i+1 < len(args) {
			dataDir = args[i+1]
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
	home, _ := os.UserHomeDir()
	dataDir := filepath.Join(home, ".vaultd")
	port := "8080"

	for i, arg := range args {
		if arg == "--data" && i+1 < len(args) {
			dataDir = args[i+1]
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

