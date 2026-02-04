// Package hooks provides webhook and callback functionality.
package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
)

// EventType represents the type of hook event
type EventType string

const (
	EventCreate EventType = "create"
	EventUpdate EventType = "update"
	EventDelete EventType = "delete"
	EventSync   EventType = "sync"
)

// HookEvent contains event data passed to callbacks
type HookEvent struct {
	Type      EventType `json:"type"`
	EntryID   uuid.UUID `json:"entry_id"`
	EntryType string    `json:"entry_type"`
	Content   []byte    `json:"content,omitempty"`
	Tags      []string  `json:"tags,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	PeerID    string    `json:"peer_id,omitempty"` // For sync events
}

// Callback is a function called when an event occurs
type Callback func(event HookEvent)

// WebhookConfig configures an HTTP webhook
type WebhookConfig struct {
	ID         string            `json:"id"`
	URL        string            `json:"url"`
	Events     []EventType       `json:"events"`      // Events to listen for
	Headers    map[string]string `json:"headers"`     // Custom headers
	Secret     string            `json:"secret"`      // HMAC secret for signing
	MaxRetries int               `json:"max_retries"` // Retry count (default 3)
	Timeout    time.Duration     `json:"timeout"`     // Request timeout
	Async      bool              `json:"async"`       // Non-blocking
}

// Manager manages hooks and webhooks
type Manager struct {
	callbacks map[EventType][]Callback
	webhooks  map[string]*WebhookConfig
	client    *http.Client
	mu        sync.RWMutex
}

// NewManager creates a new hook manager
func NewManager() *Manager {
	return &Manager{
		callbacks: make(map[EventType][]Callback),
		webhooks:  make(map[string]*WebhookConfig),
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// OnCreate registers a callback for entry creation
func (m *Manager) OnCreate(cb Callback) {
	m.registerCallback(EventCreate, cb)
}

// OnUpdate registers a callback for entry updates
func (m *Manager) OnUpdate(cb Callback) {
	m.registerCallback(EventUpdate, cb)
}

// OnDelete registers a callback for entry deletion
func (m *Manager) OnDelete(cb Callback) {
	m.registerCallback(EventDelete, cb)
}

// OnSync registers a callback for sync events
func (m *Manager) OnSync(cb Callback) {
	m.registerCallback(EventSync, cb)
}

// On registers a callback for a specific event type
func (m *Manager) On(eventType EventType, cb Callback) {
	m.registerCallback(eventType, cb)
}

func (m *Manager) registerCallback(eventType EventType, cb Callback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callbacks[eventType] = append(m.callbacks[eventType], cb)
}

// RegisterWebhook adds an HTTP webhook
func (m *Manager) RegisterWebhook(config WebhookConfig) error {
	if config.URL == "" {
		return fmt.Errorf("webhook URL is required")
	}
	if config.ID == "" {
		config.ID = uuid.New().String()
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}
	if config.Timeout == 0 {
		config.Timeout = 10 * time.Second
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.webhooks[config.ID] = &config
	return nil
}

// UnregisterWebhook removes a webhook
func (m *Manager) UnregisterWebhook(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.webhooks, id)
}

// ListWebhooks returns all registered webhooks
func (m *Manager) ListWebhooks() []WebhookConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()

	configs := make([]WebhookConfig, 0, len(m.webhooks))
	for _, wh := range m.webhooks {
		configs = append(configs, *wh)
	}
	return configs
}

// Trigger fires an event to all registered callbacks and webhooks
func (m *Manager) Trigger(event HookEvent) {
	m.mu.RLock()
	callbacks := m.callbacks[event.Type]
	webhooks := make([]*WebhookConfig, 0)
	for _, wh := range m.webhooks {
		for _, et := range wh.Events {
			if et == event.Type {
				webhooks = append(webhooks, wh)
				break
			}
		}
	}
	m.mu.RUnlock()

	// Execute callbacks
	for _, cb := range callbacks {
		cb(event)
	}

	// Execute webhooks
	for _, wh := range webhooks {
		if wh.Async {
			go m.executeWebhook(wh, event)
		} else {
			m.executeWebhook(wh, event)
		}
	}
}

// TriggerAsync fires an event asynchronously
func (m *Manager) TriggerAsync(event HookEvent) {
	go m.Trigger(event)
}

func (m *Manager) executeWebhook(config *WebhookConfig, event HookEvent) error {
	payload, _ := json.Marshal(event)

	var lastErr error
	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			time.Sleep(time.Duration(attempt*attempt) * time.Second)
		}

		ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
		req, err := http.NewRequestWithContext(ctx, "POST", config.URL, bytes.NewReader(payload))
		if err != nil {
			cancel()
			lastErr = err
			continue
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-VaultD-Event", string(event.Type))

		for k, v := range config.Headers {
			req.Header.Set(k, v)
		}

		resp, err := m.client.Do(req)
		cancel()

		if err != nil {
			lastErr = err
			continue
		}
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}

		lastErr = fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return lastErr
}

// Helper functions to create events

// NewCreateEvent creates a create event
func NewCreateEvent(entryID uuid.UUID, entryType string, content []byte, tags []string) HookEvent {
	return HookEvent{
		Type:      EventCreate,
		EntryID:   entryID,
		EntryType: entryType,
		Content:   content,
		Tags:      tags,
		Timestamp: time.Now(),
	}
}

// NewUpdateEvent creates an update event
func NewUpdateEvent(entryID uuid.UUID, entryType string, content []byte, tags []string) HookEvent {
	return HookEvent{
		Type:      EventUpdate,
		EntryID:   entryID,
		EntryType: entryType,
		Content:   content,
		Tags:      tags,
		Timestamp: time.Now(),
	}
}

// NewDeleteEvent creates a delete event
func NewDeleteEvent(entryID uuid.UUID) HookEvent {
	return HookEvent{
		Type:      EventDelete,
		EntryID:   entryID,
		Timestamp: time.Now(),
	}
}

// NewSyncEvent creates a sync event
func NewSyncEvent(peerID string) HookEvent {
	return HookEvent{
		Type:      EventSync,
		PeerID:    peerID,
		Timestamp: time.Now(),
	}
}
