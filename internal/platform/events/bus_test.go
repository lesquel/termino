package events_test

import (
	"context"
	"testing"

	"github.com/lesquel/termino/internal/platform/events"
)

// testEvent is a minimal Event implementation for testing.
type testEvent struct {
	eventType string
	payload   string
}

func (e testEvent) Type() string { return e.eventType }

func TestBus_SubscribeAndPublish(t *testing.T) {
	bus := events.NewBus()

	var received []string
	bus.Subscribe("task.moved", func(ev events.Event) {
		te, ok := ev.(testEvent)
		if !ok {
			t.Errorf("unexpected event type %T", ev)
			return
		}
		received = append(received, te.payload)
	})

	bus.Publish(context.Background(), testEvent{eventType: "task.moved", payload: "hello"})
	bus.Publish(context.Background(), testEvent{eventType: "task.moved", payload: "world"})

	if len(received) != 2 {
		t.Fatalf("received %d events; want 2", len(received))
	}
	if received[0] != "hello" {
		t.Errorf("received[0] = %q; want %q", received[0], "hello")
	}
	if received[1] != "world" {
		t.Errorf("received[1] = %q; want %q", received[1], "world")
	}
}

func TestBus_UnrelatedEventNotDelivered(t *testing.T) {
	bus := events.NewBus()

	called := false
	bus.Subscribe("task.moved", func(ev events.Event) {
		called = true
	})

	bus.Publish(context.Background(), testEvent{eventType: "sprint.closed"})

	if called {
		t.Error("handler was called for a different event type")
	}
}

func TestBus_MultipleSubscribers(t *testing.T) {
	bus := events.NewBus()

	counts := make([]int, 3)
	for i := range counts {
		idx := i
		bus.Subscribe("task.created", func(ev events.Event) {
			counts[idx]++
		})
	}

	bus.Publish(context.Background(), testEvent{eventType: "task.created"})

	for i, c := range counts {
		if c != 1 {
			t.Errorf("subscriber %d called %d times; want 1", i, c)
		}
	}
}

func TestBus_SynchronousDelivery(t *testing.T) {
	bus := events.NewBus()

	order := make([]int, 0, 3)
	for i := 1; i <= 3; i++ {
		n := i
		bus.Subscribe("ordered", func(ev events.Event) {
			order = append(order, n)
		})
	}

	bus.Publish(context.Background(), testEvent{eventType: "ordered"})

	if len(order) != 3 {
		t.Fatalf("order len = %d; want 3", len(order))
	}
	for i, v := range order {
		if v != i+1 {
			t.Errorf("order[%d] = %d; want %d", i, v, i+1)
		}
	}
}

func TestBus_ZeroValueUsable(t *testing.T) {
	var bus events.Bus

	called := false
	bus.Subscribe("ping", func(ev events.Event) { called = true })
	bus.Publish(context.Background(), testEvent{eventType: "ping"})

	if !called {
		t.Error("zero-value Bus did not call subscriber")
	}
}
