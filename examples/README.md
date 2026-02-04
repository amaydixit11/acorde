# acorde Examples

This directory contains example applications built with acorde.

## Examples

### 1. notes-cli
A minimal command-line notes app demonstrating acorde as an **embedded Go library**.

```bash
cd notes-cli
go build
./notes-cli add "My first note"
./notes-cli list
./notes-cli search "first"
```

### 2. notes-web
A simple web UI that uses the **REST API** to manage notes.

```bash
# Terminal 1: Start acorde REST server
cd ../..
acorde serve --port 8080

# Terminal 2: Open the web app
cd notes-web
# Open index.html in browser
```

## Using acorde in Your App

### Option 1: Embed as Go Library
```go
import "github.com/amaydixit11/acorde/pkg/engine"

e, _ := engine.New(engine.Config{DataDir: "./data"})
defer e.Close()

entry, _ := e.AddEntry(engine.AddEntryInput{
    Type:    engine.Note,
    Content: []byte("Hello World"),
    Tags:    []string{"example"},
})
```

### Option 2: Use REST API
```bash
# Start server
acorde serve --port 8080

# Create entry
curl -X POST http://localhost:8080/entries \
  -H "Content-Type: application/json" \
  -d '{"type":"note","content":"Hello World","tags":["example"]}'

# List entries
curl http://localhost:8080/entries

# Subscribe to events (SSE)
curl http://localhost:8080/events
```
