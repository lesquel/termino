// Package cli wires the Cobra command tree for the termino binary.
package cli

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/lesquel/termino/internal/features/agent/application"
	agentinfra "github.com/lesquel/termino/internal/features/agent/infrastructure"
	boardapp "github.com/lesquel/termino/internal/features/board/application"
	boardinfra "github.com/lesquel/termino/internal/features/board/infrastructure"
	memberapp "github.com/lesquel/termino/internal/features/member/application"
	memberinfra "github.com/lesquel/termino/internal/features/member/infrastructure"
	projectapp "github.com/lesquel/termino/internal/features/project/application"
	projectinfra "github.com/lesquel/termino/internal/features/project/infrastructure"
	shareapp "github.com/lesquel/termino/internal/features/share/application"
	shareinfra "github.com/lesquel/termino/internal/features/share/infrastructure"
	sprintapp "github.com/lesquel/termino/internal/features/sprint/application"
	sprintinfra "github.com/lesquel/termino/internal/features/sprint/infrastructure"
	taskapp "github.com/lesquel/termino/internal/features/task/application"
	taskinfra "github.com/lesquel/termino/internal/features/task/infrastructure"
	widgetapp "github.com/lesquel/termino/internal/features/widget/application"
	widgetinfra "github.com/lesquel/termino/internal/features/widget/infrastructure"
	"github.com/lesquel/termino/internal/platform/config"
	"github.com/lesquel/termino/internal/platform/events"
	"github.com/lesquel/termino/internal/platform/sqlite"
)

// Container holds lazily-initialised application dependencies.
// DB is opened only when a command that requires data access is executed.
// Running `termino --help` or `termino theme` never opens the DB.
type Container struct {
	mu  sync.Mutex
	db  *sql.DB
	cfg *config.Config

	projectUC *projectapp.ProjectUseCases
	boardUC   *boardapp.BoardUseCases
	taskUC    *taskapp.TaskUseCases
	sprintUC  *sprintapp.SprintUseCases
	memberUC  *memberapp.MemberUseCases
	widgetUC  *widgetapp.WidgetUseCases
	agentUC   *application.AgentUseCases
	shareSvc  *shareapp.ShareService
}

// Config returns the loaded configuration, loading it once.
func (c *Container) Config() (*config.Config, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cfg == nil {
		cfg, err := config.Load()
		if err != nil {
			return nil, fmt.Errorf("load config: %w", err)
		}
		c.cfg = cfg
	}
	return c.cfg, nil
}

// ReloadConfig drops the cached config so the next Config call reads the
// file again (after setup wrote it).
func (c *Container) ReloadConfig() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cfg = nil
}

// DB returns the open database, opening it lazily on first call.
func (c *Container) DB() (*sql.DB, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.db == nil {
		cfg, err := config.Load()
		if err != nil {
			return nil, fmt.Errorf("container.DB load config: %w", err)
		}
		c.cfg = cfg
		dbPath := filepath.Join(config.DataDir(), "termino.db")
		db, err := sqlite.Open(dbPath)
		if err != nil {
			return nil, fmt.Errorf("container.DB open: %w", err)
		}
		c.db = db
	}
	return c.db, nil
}

// ProjectUseCases returns the project use-case object, wiring repos lazily.
func (c *Container) ProjectUseCases() (*projectapp.ProjectUseCases, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.projectUseCasesLocked()
}

// projectUseCasesLocked wires the project use cases. Must be called with c.mu held.
func (c *Container) projectUseCasesLocked() (*projectapp.ProjectUseCases, error) {
	if c.projectUC == nil {
		db, err := c.dbLocked()
		if err != nil {
			return nil, err
		}
		bus := &events.Bus{}
		share := c.shareServiceLocked(db)
		c.projectUC = projectapp.NewProjectUseCases(
			share.Projects(projectinfra.NewProjectRepository(db)),
			share.Board(boardinfra.NewBoardRepository(db)),
			bus,
		)
	}
	return c.projectUC, nil
}

// TaskUseCases returns the task use-case object.
func (c *Container) TaskUseCases() (*taskapp.TaskUseCases, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.taskUC == nil {
		db, err := c.dbLocked()
		if err != nil {
			return nil, err
		}
		// Every move honours the project's Methodology.CanMove rule.
		projectUC, err := c.projectUseCasesLocked()
		if err != nil {
			return nil, err
		}
		bus := &events.Bus{}
		share := c.shareServiceLocked(db)
		c.taskUC = taskapp.NewTaskUseCases(
			share.Tasks(taskinfra.NewTaskRepository(db)),
			taskinfra.NewCommentRepository(db),
			taskinfra.NewActivityRepository(db),
			share.Board(boardinfra.NewBoardRepository(db)),
			bus,
		).WithMethodologies(projectUC).WithLabels(share.Labels(taskinfra.NewLabelRepository(db)))
	}
	return c.taskUC, nil
}

// BoardUseCases returns the board use-case object.
func (c *Container) BoardUseCases() (*boardapp.BoardUseCases, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.boardUC == nil {
		db, err := c.dbLocked()
		if err != nil {
			return nil, err
		}
		share := c.shareServiceLocked(db)
		c.boardUC = boardapp.NewBoardUseCases(
			share.Board(boardinfra.NewBoardRepository(db)),
			share.Tasks(taskinfra.NewTaskRepository(db)),
		)
	}
	return c.boardUC, nil
}

// SprintUseCases returns the sprint use-case object.
func (c *Container) SprintUseCases() (*sprintapp.SprintUseCases, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sprintUC == nil {
		db, err := c.dbLocked()
		if err != nil {
			return nil, err
		}
		share := c.shareServiceLocked(db)
		c.sprintUC = sprintapp.NewSprintUseCases(
			share.Sprints(sprintinfra.NewSprintRepository(db)),
			share.Tasks(taskinfra.NewTaskRepository(db)),
		)
	}
	return c.sprintUC, nil
}

// MemberUseCases returns the member use-case object.
func (c *Container) MemberUseCases() (*memberapp.MemberUseCases, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.memberUC == nil {
		db, err := c.dbLocked()
		if err != nil {
			return nil, err
		}
		c.memberUC = memberapp.NewMemberUseCases(
			c.shareServiceLocked(db).Members(memberinfra.NewMemberRepository(db)),
		)
	}
	return c.memberUC, nil
}

// ShareService returns the repository-sharing service. Its repositories are
// the undecorated ones: import must write through them (or through use cases
// built on undecorated repositories) so applying shared files never exports.
func (c *Container) ShareService() (*shareapp.ShareService, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	db, err := c.dbLocked()
	if err != nil {
		return nil, err
	}
	return c.shareServiceLocked(db), nil
}

// shareServiceLocked wires the share service once. Every use case writes
// through repositories wrapped by it, so TUI, CLI and MCP mutations of a
// project with a share root are exported to <share_root>/.termino/. Export
// failures go to the termino log file, never to stdout (MCP stdio). Must be
// called with c.mu held.
func (c *Container) shareServiceLocked(db *sql.DB) *shareapp.ShareService {
	if c.shareSvc == nil {
		c.shareSvc = shareapp.NewShareService(shareapp.Deps{
			Projects: projectinfra.NewProjectRepository(db),
			Board:    boardinfra.NewBoardRepository(db),
			Tasks:    taskinfra.NewTaskRepository(db),
			Members:  memberinfra.NewMemberRepository(db),
			Labels:   taskinfra.NewLabelRepository(db),
			Sprints:  sprintinfra.NewSprintRepository(db),
			Files:    shareinfra.FileStore{},
			Hashes:   shareinfra.NewHashRepository(db),
			Codec:    shareinfra.TOMLCodec{},
			State:    shareinfra.NewImportStateRepository(db),
			Logger:   fileLogger,
		})
	}
	return c.shareSvc
}

// WidgetUseCases returns the widget use-case object.
func (c *Container) WidgetUseCases() (*widgetapp.WidgetUseCases, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.widgetUC == nil {
		db, err := c.dbLocked()
		if err != nil {
			return nil, err
		}
		c.widgetUC = widgetapp.NewWidgetUseCases(
			widgetinfra.NewWidgetRepository(db),
			projectinfra.NewPathDetector(db),
		)
	}
	return c.widgetUC, nil
}

// AgentUseCases returns the agent use-case object.
func (c *Container) AgentUseCases() (*application.AgentUseCases, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.agentUC == nil {
		db, err := c.dbLocked()
		if err != nil {
			return nil, err
		}
		c.agentUC = application.NewAgentUseCases(
			agentinfra.NewAgentSessionRepository(db),
		)
	}
	return c.agentUC, nil
}

// dbLocked opens the DB. Must be called with c.mu held.
func (c *Container) dbLocked() (*sql.DB, error) {
	if c.db == nil {
		if c.cfg == nil {
			cfg, err := config.Load()
			if err != nil {
				return nil, fmt.Errorf("container.dbLocked load config: %w", err)
			}
			c.cfg = cfg
		}
		dbPath := filepath.Join(config.DataDir(), "termino.db")
		db, err := sqlite.Open(dbPath)
		if err != nil {
			return nil, fmt.Errorf("container.dbLocked open: %w", err)
		}
		c.db = db
	}
	return c.db, nil
}

// globalContainer is the singleton DI container for the CLI process.
var globalContainer = &Container{}
