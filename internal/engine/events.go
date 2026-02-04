package engine

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// EventType represents the type of change event
type EventType string

const (
	EventCreated EventType = "created"
	EventUpdated EventType = "updated"
	EventDeleted EventType = "deleted"
	EventSynced  EventType = "synced"
)

// Event represents a change notification
type Event struct {
	Type      EventType `json:"type"`
	EntryID   uuid.UUID `json:"entry_id"`
	EntryType string    `json:"entry_type,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// SubscriptionOptions configures a subscription
type SubscriptionOptions struct {
	// Events filters by event type (nil = all events)
	Events []EventType
	// EntryType filters by entry type (empty = all types)
	EntryType string
}

// Subscription represents an active event subscription
type Subscription interface {
	// Events returns the channel to receive events on
	Events() <-chan Event
	// Close stops the subscription and closes the channel
	Close()
}

// subscriptionImpl is the concrete implementation
type subscriptionImpl struct {
	ch      chan Event
	closed  bool
	mu      sync.Mutex
	filter  SubscriptionOptions
}

func newSubscription(bufferSize int, opts SubscriptionOptions) *subscriptionImpl {
	return &subscriptionImpl{
		ch:     make(chan Event, bufferSize),
		filter: opts,
	}
}

func (s *subscriptionImpl) Events() <-chan Event {
	return s.ch
}

func (s *subscriptionImpl) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		close(s.ch)
	}
}

func (s *subscriptionImpl) matches(event Event) bool {
	// Check event type filter
	if len(s.filter.Events) > 0 {
		found := false
		for _, et := range s.filter.Events {
			if et == event.Type {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	
	// Check entry type filter
	if s.filter.EntryType != "" && event.EntryType != s.filter.EntryType {
		return false
	}
	
	return true
}

func (s *subscriptionImpl) send(event Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed && s.matches(event) {
		select {
		case s.ch <- event:
		default:
			// Buffer full, drop event (non-blocking)
		}
	}
}

// EventBus manages subscriptions and broadcasts events
type EventBus struct {
	subs []*subscriptionImpl
	mu   sync.RWMutex
}

// NewEventBus creates a new event bus
func NewEventBus() *EventBus {
	return &EventBus{}
}

// Subscribe creates a new subscription (all events)
func (b *EventBus) Subscribe() Subscription {
	return b.SubscribeWithOptions(SubscriptionOptions{})
}

// SubscribeWithOptions creates a new subscription with filtering
func (b *EventBus) SubscribeWithOptions(opts SubscriptionOptions) Subscription {
	sub := newSubscription(100, opts) // Buffer 100 events
	b.mu.Lock()
	b.subs = append(b.subs, sub)
	b.mu.Unlock()
	return sub
}

// Publish sends an event to all subscribers
func (b *EventBus) Publish(event Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, sub := range b.subs {
		sub.send(event)
	}
}

// Unsubscribe removes a subscription
func (b *EventBus) Unsubscribe(sub Subscription) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i, s := range b.subs {
		if s == sub {
			s.Close()
			b.subs = append(b.subs[:i], b.subs[i+1:]...)
			return
		}
	}
}

// Close closes all subscriptions
func (b *EventBus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, sub := range b.subs {
		sub.Close()
	}
	b.subs = nil
}
