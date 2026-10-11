// Package domain contains cross-feature shared types.
// Domain packages NEVER import anything outside the Go standard library.
package domain

// ActorKind identifies whether the actor is a human or an AI agent.
type ActorKind string

const (
	ActorKindHuman ActorKind = "human"
	ActorKindAgent ActorKind = "agent"
)

// Actor represents who (or what) triggered a use-case invocation.
// It is used for audit trails, activity logs, and access decisions.
type Actor struct {
	ID   string
	Name string
	Kind ActorKind
}
