package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	sharedomain "github.com/lesquel/termino/internal/domain"
	boarddomain "github.com/lesquel/termino/internal/features/board/domain"
	memberdomain "github.com/lesquel/termino/internal/features/member/domain"
	projectdomain "github.com/lesquel/termino/internal/features/project/domain"
	sprintdomain "github.com/lesquel/termino/internal/features/sprint/domain"
	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
	widgetapp "github.com/lesquel/termino/internal/features/widget/application"
	"github.com/lesquel/termino/internal/platform/sanitize"
)

// cliActor is the actor recorded for changes made from the command line: the
// setup user's member when registered, else the built-in "cli" member.
func cliActor() sharedomain.Actor {
	userName := ""
	if cfg, err := globalContainer.Config(); err == nil {
		userName = cfg.UserName
	}
	uc, err := globalContainer.MemberUseCases()
	if err != nil {
		uc = nil
	}
	return uc.LocalActor(context.Background(), userName)
}

// cleanName sanitizes a single-line name typed by the user and rejects one
// that is empty once escape sequences and control characters are removed.
func cleanName(what, raw string) (string, error) {
	name := sanitize.Line(raw)
	if name == "" {
		return "", fmt.Errorf("%s is empty after removing control characters", what)
	}
	return name, nil
}

// errNoCurrentProject is returned when neither the working directory nor the
// config names a project.
var errNoCurrentProject = errors.New("no current project set — run `termino project new` first")

// currentProjectID returns the project commands act on: the project
// associated with the working directory (longest path prefix) wins, the
// configured current project is the fallback. The widget uses the same rule.
func currentProjectID() (string, error) {
	cfg, err := globalContainer.Config()
	if err != nil {
		return "", err
	}
	cwd, _ := os.Getwd()
	if cwd != "" {
		uc, err := globalContainer.WidgetUseCases()
		if err != nil {
			return "", err
		}
		projectID, err := uc.ResolveProject(cwd, cfg.CurrentProjectID)
		if errors.Is(err, widgetapp.ErrNoProject) {
			return "", errNoCurrentProject
		}
		if err != nil {
			return "", err
		}
		return projectID, nil
	}
	if cfg.CurrentProjectID == "" {
		return "", errNoCurrentProject
	}
	return cfg.CurrentProjectID, nil
}

// projectKey returns the key of the project with the given ID.
func projectKey(ctx context.Context, projectID string) (string, error) {
	keys, err := projectKeys(ctx)
	if err != nil {
		return "", err
	}
	key, ok := keys[projectID]
	if !ok {
		return "", fmt.Errorf("project %s not found (archived?)", projectID)
	}
	return key, nil
}

// projectKeys maps project IDs to their keys.
func projectKeys(ctx context.Context) (map[string]string, error) {
	uc, err := globalContainer.ProjectUseCases()
	if err != nil {
		return nil, err
	}
	projects, err := uc.ListProjects(ctx)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	keys := make(map[string]string, len(projects))
	for _, p := range projects {
		keys[p.ID] = p.Key
	}
	return keys, nil
}

// projectByKey finds a non-archived project by key, ignoring case.
func projectByKey(ctx context.Context, key string) (projectdomain.Project, error) {
	uc, err := globalContainer.ProjectUseCases()
	if err != nil {
		return projectdomain.Project{}, err
	}
	projects, err := uc.ListProjects(ctx)
	if err != nil {
		return projectdomain.Project{}, fmt.Errorf("list projects: %w", err)
	}
	for _, p := range projects {
		if strings.EqualFold(p.Key, key) {
			return p, nil
		}
	}
	return projectdomain.Project{}, fmt.Errorf("no project with key %q", key)
}

// resolveTask finds a task by reference: "KEY-N" (any project), "N" or "#N"
// (current project), or a task ID.
func resolveTask(ctx context.Context, ref string) (taskdomain.Task, error) {
	uc, err := globalContainer.TaskUseCases()
	if err != nil {
		return taskdomain.Task{}, err
	}
	ref = strings.TrimSpace(ref)
	notFound := fmt.Errorf("task %q not found", sanitize.Line(ref))

	if n, err := strconv.Atoi(strings.TrimPrefix(ref, "#")); err == nil {
		projectID, err := currentProjectID()
		if err != nil {
			return taskdomain.Task{}, err
		}
		t, err := uc.FindTaskByNumber(ctx, projectID, n)
		if err != nil || t.DeletedAt != nil {
			return taskdomain.Task{}, notFound
		}
		return t, nil
	}
	if i := strings.LastIndex(ref, "-"); i > 0 {
		if n, err := strconv.Atoi(ref[i+1:]); err == nil {
			p, err := projectByKey(ctx, ref[:i])
			if err == nil {
				t, err := uc.FindTaskByNumber(ctx, p.ID, n)
				if err != nil || t.DeletedAt != nil {
					return taskdomain.Task{}, notFound
				}
				return t, nil
			}
		}
	}
	t, err := uc.FindTaskByID(ctx, ref)
	if err != nil || t.DeletedAt != nil {
		return taskdomain.Task{}, notFound
	}
	return t, nil
}

// resolveBoardColumn finds a column of the project by ID or by name
// (ignoring case).
func resolveBoardColumn(ctx context.Context, projectID, ref string) (boarddomain.Column, []boarddomain.Column, error) {
	uc, err := globalContainer.BoardUseCases()
	if err != nil {
		return boarddomain.Column{}, nil, err
	}
	board, err := uc.GetBoard(ctx, projectID)
	if err != nil {
		return boarddomain.Column{}, nil, fmt.Errorf("get board: %w", err)
	}
	ref = strings.TrimSpace(ref)
	for _, c := range board.Columns {
		if c.ID == ref {
			return c, board.Columns, nil
		}
	}
	for _, c := range board.Columns {
		if strings.EqualFold(c.Name, ref) {
			return c, board.Columns, nil
		}
	}
	return boarddomain.Column{}, board.Columns, fmt.Errorf("column %q not found", sanitize.Line(ref))
}

// resolveMember finds a member by ID or by name (ignoring case). A name
// shared by several members is ambiguous and must be given by ID.
func resolveMember(ctx context.Context, ref string) (memberdomain.Member, error) {
	uc, err := globalContainer.MemberUseCases()
	if err != nil {
		return memberdomain.Member{}, err
	}
	members, err := uc.ListMembers(ctx)
	if err != nil {
		return memberdomain.Member{}, fmt.Errorf("list members: %w", err)
	}
	ref = strings.TrimSpace(ref)
	for _, m := range members {
		if m.ID == ref {
			return m, nil
		}
	}
	var matches []memberdomain.Member
	for _, m := range members {
		if strings.EqualFold(m.Name, ref) {
			matches = append(matches, m)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return memberdomain.Member{}, fmt.Errorf("member %q not found — see `termino member list`", sanitize.Line(ref))
	default:
		return memberdomain.Member{}, fmt.Errorf("several members are named %q — use the member ID", sanitize.Line(ref))
	}
}

// resolveSprint finds a sprint of the project by ID or by name (ignoring case).
func resolveSprint(ctx context.Context, projectID, ref string) (sprintdomain.Sprint, error) {
	uc, err := globalContainer.SprintUseCases()
	if err != nil {
		return sprintdomain.Sprint{}, err
	}
	sprints, err := uc.ListSprints(ctx, projectID)
	if err != nil {
		return sprintdomain.Sprint{}, fmt.Errorf("list sprints: %w", err)
	}
	ref = strings.TrimSpace(ref)
	for _, s := range sprints {
		if s.ID == ref {
			return s, nil
		}
	}
	for _, s := range sprints {
		if strings.EqualFold(s.Name, ref) {
			return s, nil
		}
	}
	return sprintdomain.Sprint{}, fmt.Errorf("sprint %q not found — see `termino sprint list`", sanitize.Line(ref))
}
