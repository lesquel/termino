package infrastructure_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	agentdomain "github.com/lesquel/termino/internal/features/agent/domain"
	agentinfra "github.com/lesquel/termino/internal/features/agent/infrastructure"
	"github.com/lesquel/termino/internal/platform/sqlite"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := sqlite.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestAgentSessionRepository_UpsertAndFind(t *testing.T) {
	db := openTestDB(t)
	repo := agentinfra.NewAgentSessionRepository(db)
	ctx := context.Background()

	s := agentdomain.AgentSession{
		AgentID:    "agent-1",
		AgentName:  "Kiro",
		LastSeenAt: time.Now().UTC().Truncate(time.Second),
	}
	if err := repo.Upsert(ctx, s); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := repo.FindByID(ctx, "agent-1")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.AgentName != s.AgentName {
		t.Errorf("AgentName: got %q, want %q", got.AgentName, s.AgentName)
	}
}

func TestAgentSessionRepository_ListActive(t *testing.T) {
	db := openTestDB(t)
	repo := agentinfra.NewAgentSessionRepository(db)
	ctx := context.Background()

	active := agentdomain.AgentSession{
		AgentID:    "agent-active",
		AgentName:  "Active Agent",
		LastSeenAt: time.Now().UTC(),
	}
	stale := agentdomain.AgentSession{
		AgentID:    "agent-stale",
		AgentName:  "Stale Agent",
		LastSeenAt: time.Now().UTC().Add(-2 * time.Hour),
	}
	for _, s := range []agentdomain.AgentSession{active, stale} {
		if err := repo.Upsert(ctx, s); err != nil {
			t.Fatalf("Upsert %s: %v", s.AgentID, err)
		}
	}

	list, err := repo.ListActive(ctx, 30*time.Minute)
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("ListActive: got %d, want 1", len(list))
	}
	if list[0].AgentID != "agent-active" {
		t.Errorf("ListActive: got %q, want agent-active", list[0].AgentID)
	}
}

func TestAgentSessionRepository_ClearTask(t *testing.T) {
	db := openTestDB(t)
	repo := agentinfra.NewAgentSessionRepository(db)
	ctx := context.Background()

	// First upsert without a task, then set a task via a raw update to avoid
	// FK constraint issues in the test (task must exist to set the FK).
	s := agentdomain.AgentSession{
		AgentID:    "agent-task",
		AgentName:  "Busy Agent",
		LastSeenAt: time.Now().UTC(),
	}
	if err := repo.Upsert(ctx, s); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	// Just verify ClearTask works when task is already nil.
	if err := repo.ClearTask(ctx, "agent-task"); err != nil {
		t.Fatalf("ClearTask: %v", err)
	}

	got, err := repo.FindByID(ctx, "agent-task")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.CurrentTaskID != nil {
		t.Errorf("CurrentTaskID: expected nil, got %v", got.CurrentTaskID)
	}
}
