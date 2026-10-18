package application

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	projectdomain "github.com/lesquel/termino/internal/features/project/domain"
	"github.com/lesquel/termino/internal/features/share/domain"
	"github.com/lesquel/termino/internal/platform/sanitize"
)

// AdoptAction says what Adopt did with a repository's shared board.
type AdoptAction int

const (
	// AdoptNone: the repository holds no shared board.
	AdoptNone AdoptAction = iota
	// AdoptCreated: no local project had the key; one was created and imported.
	AdoptCreated
	// AdoptLinked: a local project with the key was not shared; it now is,
	// merged with the shared files by task UUID.
	AdoptLinked
	// AdoptAlready: the project with the key is already shared from root.
	AdoptAlready
)

// AdoptResult reports the outcome of Adopt.
type AdoptResult struct {
	ProjectID string
	Action    AdoptAction
}

// ErrSharedElsewhere is returned by Adopt (and InitFor) when the project
// with the shared key is already shared from another repository.
var ErrSharedElsewhere = errors.New("project is already shared from another repository")

// Adopt connects the board shared in <root>/.termino with the local
// database, so a teammate who clones the repository and runs termino gets
// the board without any setup:
//   - no local project with the key: it is created with root as share root
//     and associated path, and everything is imported;
//   - a local project with the key that is not shared: it is linked (root
//     becomes its share root), the files are imported and merged by task
//     UUID, and its local-only tasks are exported — unless `share off`
//     stopped it sharing from root;
//   - a project with the key already shared from another root: an error
//     naming both roots; nothing changes.
func (s *ShareService) Adopt(ctx context.Context, root string) (AdoptResult, error) {
	root = filepath.Clean(root)
	data, err := s.d.Files.Read(filepath.Join(root, domain.Dir, domain.ProjectFileName))
	if errors.Is(err, os.ErrNotExist) {
		return AdoptResult{}, nil
	}
	if err != nil {
		return AdoptResult{}, err
	}
	f, err := s.d.Codec.DecodeProject(data)
	if err != nil {
		return AdoptResult{}, fmt.Errorf("read %s: %w", filepath.Join(root, domain.Dir, domain.ProjectFileName), err)
	}
	projects, err := s.d.Projects.List(ctx)
	if err != nil {
		return AdoptResult{}, fmt.Errorf("list projects: %w", err)
	}
	var p *projectdomain.Project
	for i := range projects {
		if strings.EqualFold(projects[i].Key, f.Key) {
			p = &projects[i]
		}
	}
	switch {
	case p == nil:
		return s.adoptNew(ctx, root, f)
	case p.ShareRoot == "" && s.optedOut(ctx, p.ID, root):
		return AdoptResult{ProjectID: p.ID, Action: AdoptNone}, nil
	case p.ShareRoot == "":
		p.ShareRoot = root
		if p.AssociatedPath == "" {
			p.AssociatedPath = root
		}
		if err := s.d.Projects.Save(ctx, *p); err != nil {
			return AdoptResult{}, fmt.Errorf("save project: %w", err)
		}
		if _, err := s.Import(ctx, p.ID); err != nil {
			return AdoptResult{ProjectID: p.ID, Action: AdoptLinked}, err
		}
		return AdoptResult{ProjectID: p.ID, Action: AdoptLinked}, s.ExportAll(ctx, p.ID)
	case filepath.Clean(p.ShareRoot) == root:
		return AdoptResult{ProjectID: p.ID, Action: AdoptAlready}, nil
	default:
		return AdoptResult{ProjectID: p.ID}, sharedElsewhere(*p, root)
	}
}

func sharedElsewhere(p projectdomain.Project, root string) error {
	return fmt.Errorf("%w: %s is shared from %s, not from %s — run `termino share off` in %s first",
		ErrSharedElsewhere, p.Key, p.ShareRoot, root, p.ShareRoot)
}

func (s *ShareService) adoptNew(ctx context.Context, root string, f domain.ProjectFile) (AdoptResult, error) {
	if !projectKeyRe.MatchString(f.Key) {
		return AdoptResult{}, fmt.Errorf("%w: project key %q", domain.ErrInvalidFile, f.Key)
	}
	name := sanitize.Line(f.Name)
	if name == "" {
		name = f.Key
	}
	m := projectdomain.MethodologyKind(f.Methodology)
	if m != projectdomain.MethodologyScrum {
		m = projectdomain.MethodologyKanban
	}
	p := projectdomain.Project{
		ID: uuid.NewString(), Key: f.Key, Name: name, Methodology: m,
		AssociatedPath: root, ShareRoot: root, CreatedAt: s.now(),
	}
	if err := s.d.Projects.Save(ctx, p); err != nil {
		return AdoptResult{}, fmt.Errorf("create project: %w", err)
	}
	res := AdoptResult{ProjectID: p.ID, Action: AdoptCreated}
	_, err := s.Import(ctx, p.ID)
	return res, err
}

// InitFor starts sharing a project from root like Init, refusing a project
// already shared from another root.
func (s *ShareService) InitFor(ctx context.Context, projectID, root string) error {
	root = filepath.Clean(root)
	p, err := s.d.Projects.FindByID(ctx, projectID)
	if err != nil {
		return fmt.Errorf("find project: %w", err)
	}
	if p.ShareRoot != "" && filepath.Clean(p.ShareRoot) != root {
		return sharedElsewhere(p, root)
	}
	return s.Init(ctx, projectID, root)
}

// Off stops sharing a project: its share root is cleared, so later changes
// are no longer exported or imported. The files stay on disk, and Adopt no
// longer links the project to them (until Init shares it again).
func (s *ShareService) Off(ctx context.Context, projectID string) error {
	p, err := s.d.Projects.FindByID(ctx, projectID)
	if err != nil {
		return fmt.Errorf("find project: %w", err)
	}
	if p.ShareRoot == "" {
		return nil
	}
	root := filepath.Clean(p.ShareRoot)
	p.ShareRoot = ""
	if err := s.d.Projects.Save(ctx, p); err != nil {
		return fmt.Errorf("save project: %w", err)
	}
	if s.d.State != nil {
		return s.d.State.SetOptOut(ctx, p.ID, root, true)
	}
	return nil
}

// optedOut reports whether `share off` stopped the project sharing from root.
func (s *ShareService) optedOut(ctx context.Context, projectID, root string) bool {
	if s.d.State == nil {
		return false
	}
	out, err := s.d.State.OptedOut(ctx, projectID, root)
	return err == nil && out
}
