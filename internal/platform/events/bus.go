// Package events provides a simple in-process synchronous event bus.
// Handlers run in the same goroutine as Publish — there is no fan-out,
// no buffering, and no goroutine management. This is intentional for
// the MVP: it keeps the event system trivial to reason about, and the
// domain events fired in this version are low-frequency.
package events

import (
	"context"
	"sync"
)

// Event is the interface every domain event must implement.
type Event interface {
	// Type returns a dot-separated string that identifies the event,
	// e.g. "task.moved" or "sprint.closed".
	Type() string
}

// Bus is a synchronous event bus. The zero value is usable.
type Bus struct {
	mu          sync.RWMutex
	subscribers map[string][]func(Event)
}

// NewBus returns a new, empty Bus.
func NewBus() *Bus {
	return &Bus{
		subscribers: make(map[string][]func(Event)),
	}
}

// Subscribe registers handler to be called whenever an event of the given
// eventType is published. The handler runs synchronously inside Publish.
func (b *Bus) Subscribe(eventType string, handler func(Event)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.subscribers == nil {
		b.subscribers = make(map[string][]func(Event))
	}
	b.subscribers[eventType] = append(b.subscribers[eventType], handler)
}

// Publish calls all handlers registered for event.Type() in the order they
// were subscribed. The ctx argument is reserved for future use (e.g. tracing)
// and is not currently passed to handlers.
func (b *Bus) Publish(_ context.Context, event Event) {
	b.mu.RLock()
	handlers := b.subscribers[event.Type()]
	// Copy slice to avoid holding the lock while calling handlers.
	hs := make([]func(Event), len(handlers))
	copy(hs, handlers)
	b.mu.RUnlock()

	for _, h := range hs {
		h(event)
	}
}
