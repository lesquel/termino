package tui

import (
	"context"
	"time"

	sharedomain "github.com/lesquel/termino/internal/domain"
	agentdomain "github.com/lesquel/termino/internal/features/agent/domain"
	memberdomain "github.com/lesquel/termino/internal/features/member/domain"
)

// activeAgentWindow is how recently an agent must have been seen to count as
// "working" on its current task (TRD 10.1).
const activeAgentWindow = 60 * time.Second

// TaskActions is the subset of the task use cases the board mutates through.
// *taskapp.TaskUseCases satisfies it.
type TaskActions interface {
	MoveTask(ctx context.Context, actor sharedomain.Actor, taskID, targetColumnID string, position float64) error
	AssignTask(ctx context.Context, actor sharedomain.Actor, taskID string, assigneeID *string) error
	DeleteTask(ctx context.Context, actor sharedomain.Actor, taskID string, confirm bool) error
}

// MemberLister lists workspace members for the assign picker and for
// rendering assignee names. *memberapp.MemberUseCases satisfies it.
type MemberLister interface {
	ListMembers(ctx context.Context) ([]memberdomain.Member, error)
}

// AgentPresence lists recently active agents so cards they claimed show a
// "working" marker. *agentapp.AgentUseCases satisfies it.
type AgentPresence interface {
	ListActiveAgents(ctx context.Context, since time.Duration) ([]agentdomain.AgentSession, error)
}

// Services are the optional collaborators of the board. A nil field disables
// the matching feature (e.g. no Tasks → move/assign/delete are no-ops).
type Services struct {
	Tasks   TaskActions
	Members MemberLister
	Agents  AgentPresence
	// Actor is recorded as the author of every change made from the board.
	Actor sharedomain.Actor
	// Motion enables the short Done celebration animation. Callers must
	// already fold in NO_COLOR (see config.Config.MotionEnabled).
	Motion bool
	// OnDone, when set, runs after a task was moved into the Done column
	// (e.g. to check the matching checkbox in its source file).
	OnDone func(taskID string)
}
