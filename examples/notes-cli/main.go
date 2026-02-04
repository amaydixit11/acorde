// Package main is a simple CLI notes app demonstrating vaultd as an embedded library.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/amaydixit11/vaultd/pkg/engine"
	"github.com/google/uuid"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	// Initialize vaultd engine
	e, err := engine.New(engine.Config{
		DataDir: "./notes-data",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize: %v\n", err)
		os.Exit(1)
	}
	defer e.Close()

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "add":
		if len(args) < 1 {
			fmt.Println("Usage: notes-cli add <content> [tags...]")
			os.Exit(1)
		}
		content := args[0]
		tags := args[1:]

		entry, err := e.AddEntry(engine.AddEntryInput{
			Type:    engine.Note,
			Content: []byte(content),
			Tags:    tags,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to add note: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✅ Added note: %s\n", entry.ID)

	case "list":
		var filter engine.ListFilter
		noteType := engine.Note
		filter.Type = &noteType

		entries, err := e.ListEntries(filter)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to list notes: %v\n", err)
			os.Exit(1)
		}

		if len(entries) == 0 {
			fmt.Println("No notes found.")
			return
		}

		fmt.Printf("📝 Notes (%d):\n", len(entries))
		fmt.Println("─────────────────────────────────")
		for _, entry := range entries {
			content := string(entry.Content)
			if len(content) > 50 {
				content = content[:50] + "..."
			}
			tags := ""
			if len(entry.Tags) > 0 {
				tags = " [" + strings.Join(entry.Tags, ", ") + "]"
			}
			fmt.Printf("• %s%s\n  ID: %s\n\n", content, tags, entry.ID)
		}

	case "get":
		if len(args) < 1 {
			fmt.Println("Usage: notes-cli get <id>")
			os.Exit(1)
		}
		id, err := parseUUID(args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid ID: %v\n", err)
			os.Exit(1)
		}

		entry, err := e.GetEntry(id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Note not found: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("ID:      %s\n", entry.ID)
		fmt.Printf("Content: %s\n", string(entry.Content))
		fmt.Printf("Tags:    %v\n", entry.Tags)
		fmt.Printf("Created: %d\n", entry.CreatedAt)

	case "delete":
		if len(args) < 1 {
			fmt.Println("Usage: notes-cli delete <id>")
			os.Exit(1)
		}
		id, err := parseUUID(args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid ID: %v\n", err)
			os.Exit(1)
		}

		if err := e.DeleteEntry(id); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to delete: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ Note deleted")

	case "watch":
		fmt.Println("👀 Watching for changes (Ctrl+C to stop)...")
		sub := e.Subscribe()
		defer sub.Close()

		for event := range sub.Events() {
			fmt.Printf("[%s] Entry %s (%s)\n", 
				event.Type, 
				event.EntryID, 
				event.EntryType,
			)
		}

	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`notes-cli - A simple notes app powered by vaultd

Usage: notes-cli <command> [args]

Commands:
  add <content> [tags...]   Add a new note
  list                      List all notes
  get <id>                  Get a note by ID
  delete <id>               Delete a note
  watch                     Watch for changes (live events)

Example:
  notes-cli add "Buy groceries" shopping todo
  notes-cli list
  notes-cli watch`)
}

func parseUUID(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}
