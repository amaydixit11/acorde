// Package api provides an HTTP REST API for acorde.
package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/amaydixit11/acorde/pkg/engine"
	"github.com/google/uuid"
)

// Server is the HTTP API server
type Server struct {
	engine    engine.Engine
	blobs     BlobStore
	mux       *http.ServeMux
	identity  func() (string, []string)
	peers     func() []PeerInfo
	peerCount func() int
	invite    func() (string, error)
	pair      func(string) error
}

// PeerInfo describes a connected libp2p peer
type PeerInfo struct {
	ID       string   `json:"id"`
	Addrs    []string `json:"addrs"`
	Protocol string   `json:"protocol,omitempty"`
}

// BlobStore is a subset of internal/blob.Store
type BlobStore interface {
	Put(data []byte) (string, error)
	Get(cid string) ([]byte, error)
}

func New(e engine.Engine) *Server {
	s := &Server{
		engine: e,
		mux:    http.NewServeMux(),
	}
	s.setupRoutes()
	return s
}

// Config allows customizing the API server with optional capabilities
type Config struct {
	Identity  func() (string, []string)
	Peers     func() []PeerInfo
	PeerCount func() int
	Invite    func() (string, error)
	Pair      func(string) error
	Blobs     BlobStore
}

// Configure adds optional capabilities to the server
func (s *Server) Configure(cfg Config) {
	s.identity = cfg.Identity
	s.peers = cfg.Peers
	s.peerCount = cfg.PeerCount
	s.invite = cfg.Invite
	s.pair = cfg.Pair
	s.blobs = cfg.Blobs
}

func (s *Server) handleBlobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.blobs == nil {
		http.Error(w, "Blobs not configured", http.StatusNotImplemented)
		return
	}

	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	cid, err := s.blobs.Put(data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{
		"cid": cid,
	})
}

func (s *Server) handleBlob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.blobs == nil {
		http.Error(w, "Blobs not configured", http.StatusNotImplemented)
		return
	}

	cid := strings.TrimPrefix(r.URL.Path, "/blobs/")
	if cid == "" {
		http.Error(w, "Missing CID", http.StatusBadRequest)
		return
	}

	data, err := s.blobs.Get(cid)
	if err != nil {
		http.Error(w, "Blob not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

func (s *Server) setupRoutes() {
	s.mux.HandleFunc("/entries", s.handleEntries)
	s.mux.HandleFunc("/entries/", s.handleEntry)
	s.mux.HandleFunc("/search", s.handleSearch)
	s.mux.HandleFunc("/status", s.handleStatus)
	s.mux.HandleFunc("/identity", s.handleIdentity)
	s.mux.HandleFunc("/peers", s.handlePeers)
	s.mux.HandleFunc("/events", s.handleEvents)
	s.mux.HandleFunc("/invite", s.handleInvite)
	s.mux.HandleFunc("/pair", s.handlePair)
	s.mux.HandleFunc("/blobs", s.handleBlobs)
	s.mux.HandleFunc("/blobs/", s.handleBlob)
}

// ServeHTTP implements http.Handler
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	s.mux.ServeHTTP(w, r)
}

// ListenAndServe starts the HTTP server
func (s *Server) ListenAndServe(addr string) error {
	return http.ListenAndServe(addr, s)
}

// handleEntries handles GET /entries and POST /entries
func (s *Server) handleEntries(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listEntries(w, r)
	case http.MethodPost:
		s.createEntry(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleEntry handles GET/PUT/DELETE /entries/:id and POST /entries/:id/authorize
func (s *Server) handleEntry(w http.ResponseWriter, r *http.Request) {
	// Extract ID from path
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/entries/"), "/")
	if len(parts) < 1 || parts[0] == "" {
		http.Error(w, "Missing entry ID", http.StatusBadRequest)
		return
	}

	id, err := uuid.Parse(parts[0])
	if err != nil {
		http.Error(w, "Invalid entry ID", http.StatusBadRequest)
		return
	}

	// Route based on sub-path
	if len(parts) > 1 {
		switch parts[1] {
		case "authorize":
			if r.Method != http.MethodPost {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			s.authorizeEntry(w, r, id)
			return
		default:
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
	}

	switch r.Method {
	case http.MethodGet:
		s.getEntry(w, r, id)
	case http.MethodPut:
		s.updateEntry(w, r, id)
	case http.MethodDelete:
		s.deleteEntry(w, r, id)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) listEntries(w http.ResponseWriter, r *http.Request) {
	filter := engine.ListFilter{}

	// Parse query params
	if t := r.URL.Query().Get("type"); t != "" {
		entryType := engine.EntryType(t)
		filter.Type = &entryType
	}
	if tag := r.URL.Query().Get("tag"); tag != "" {
		filter.Tag = &tag
	}

	entries, err := s.engine.ListEntries(filter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSON(w, http.StatusOK, entries)
}

func (s *Server) createEntry(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Type    string   `json:"type"`
		Content string   `json:"content"`
		Tags    []string `json:"tags"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	entry, err := s.engine.AddEntry(engine.AddEntryInput{
		Type:    engine.EntryType(req.Type),
		Content: []byte(req.Content),
		Tags:    req.Tags,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	respondJSON(w, http.StatusCreated, entry)
}

func (s *Server) getEntry(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	entry, err := s.engine.GetEntry(id)
	if err != nil {
		writeEngineError(w, err)
		return
	}

	respondJSON(w, http.StatusOK, entry)
}

func (s *Server) updateEntry(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	var req struct {
		Content *string   `json:"content"`
		Tags    *[]string `json:"tags"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	input := engine.UpdateEntryInput{}
	if req.Content != nil {
		content := []byte(*req.Content)
		input.Content = &content
	}
	if req.Tags != nil {
		input.Tags = req.Tags
	}

	if err := s.engine.UpdateEntry(id, input); err != nil {
		writeEngineError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteEntry(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	if err := s.engine.DeleteEntry(id); err != nil {
		writeEngineError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) authorizeEntry(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	var req struct {
		PeerID string `json:"peer_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.PeerID == "" {
		http.Error(w, "Missing peer_id", http.StatusBadRequest)
		return
	}

	if err := s.engine.GrantWrite(id, req.PeerID); err != nil {
		writeEngineError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "Missing query parameter 'q'", http.StatusBadRequest)
		return
	}

	opts := engine.SearchOptions{
		Limit: 20,
	}

	if t := r.URL.Query().Get("type"); t != "" {
		entryType := engine.EntryType(t)
		opts.Type = &entryType
	}

	result, err := s.engine.Search(query, opts)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSON(w, http.StatusOK, result.Entries)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	entries, _ := s.engine.ListEntries(engine.ListFilter{})

	status := map[string]interface{}{
		"status":      "ok",
		"entry_count": len(entries),
	}

	if s.peerCount != nil {
		status["peer_count"] = s.peerCount()
	}

	if s.identity != nil {
		id, _ := s.identity()
		status["peer_id"] = id
	}

	respondJSON(w, http.StatusOK, status)
}

func (s *Server) handleIdentity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.identity == nil {
		http.Error(w, "Identity capability not configured", http.StatusNotImplemented)
		return
	}

	id, addrs := s.identity()
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"peer_id": id,
		"addrs":   addrs,
	})
}

func (s *Server) handlePeers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.peers == nil {
		http.Error(w, "Peers capability not configured", http.StatusNotImplemented)
		return
	}

	peers := s.peers()
	respondJSON(w, http.StatusOK, peers)
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	// Server-Sent Events
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	sub := s.engine.Subscribe()
	defer sub.Close()

	for {
		select {
		case event, ok := <-sub.Events():
			if !ok {
				return
			}
			data, _ := json.Marshal(event)
			w.Write([]byte("data: "))
			w.Write(data)
			w.Write([]byte("\n\n"))
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (s *Server) handleInvite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.invite == nil {
		http.Error(w, "Invite capability not configured", http.StatusNotImplemented)
		return
	}

	code, err := s.invite()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{
		"code": code,
	})
}

func (s *Server) handlePair(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.pair == nil {
		http.Error(w, "Pair capability not configured", http.StatusNotImplemented)
		return
	}

	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if err := s.pair(req.Code); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{
		"status": "paired",
	})
}

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeEngineError(w http.ResponseWriter, err error) {
	var notFound engine.ErrNotFound
	if errors.As(err, &notFound) {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	var denied engine.ErrAccessDenied
	if errors.As(err, &denied) {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	http.Error(w, err.Error(), http.StatusInternalServerError)
}
