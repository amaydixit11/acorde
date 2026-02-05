// Package api provides an HTTP REST API for acorde.
package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/amaydixit11/acorde/pkg/engine"
	"github.com/google/uuid"
)

// Server is the HTTP API server
type Server struct {
	engine    engine.Engine
	mux       *http.ServeMux
	peerCount func() int
}

// New creates a new API server
func New(e engine.Engine, peerCount func() int) *Server {
	s := &Server{
		engine:    e,
		mux:       http.NewServeMux(),
		peerCount: peerCount,
	}
	s.setupRoutes()
	return s
}

func (s *Server) setupRoutes() {
	s.mux.HandleFunc("/entries", s.handleEntries)
	s.mux.HandleFunc("/entries/", s.handleEntry)
	s.mux.HandleFunc("/status", s.handleStatus)
	s.mux.HandleFunc("/events", s.handleEvents)
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

// handleEntry handles GET/PUT/DELETE /entries/:id
func (s *Server) handleEntry(w http.ResponseWriter, r *http.Request) {
	// Extract ID from path
	path := strings.TrimPrefix(r.URL.Path, "/entries/")
	if path == "" {
		http.Error(w, "Missing entry ID", http.StatusBadRequest)
		return
	}

	id, err := uuid.Parse(path)
	if err != nil {
		http.Error(w, "Invalid entry ID", http.StatusBadRequest)
		return
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
		http.Error(w, err.Error(), http.StatusNotFound)
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteEntry(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	if err := s.engine.DeleteEntry(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
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

	respondJSON(w, http.StatusOK, status)
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

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
