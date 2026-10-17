package application

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	sharedomain "github.com/lesquel/termino/internal/domain"
	memberdomain "github.com/lesquel/termino/internal/features/member/domain"
	projectapp "github.com/lesquel/termino/internal/features/project/application"
	projectdomain "github.com/lesquel/termino/internal/features/project/domain"
	setupdomain "github.com/lesquel/termino/internal/features/setup/domain"
	"github.com/lesquel/termino/internal/platform/config"
)

// Mode says whether a setup creates a new project or edits the current one.
type Mode string

const (
	// ModeCreate creates a new project (first run, or "add another project").
	ModeCreate Mode = "create"
	// ModeEdit updates the current project in place; it never creates one.
	ModeEdit Mode = "edit"
)

// SetupInput is everything a setup front end (TUI wizard, line prompts or
// flags) collects. Service.Apply validates all of it before writing anything.
type SetupInput struct {
	Mode        Mode
	ProjectID   string // ModeEdit: the project to update
	UserName    string // "" falls back to $USER
	ProjectName string
	ProjectKey  string // ModeCreate only; read-only when editing
	Methodology projectdomain.MethodologyKind
	// Associate links the project to the working directory. When editing,
	// false keeps the project's existing association untouched.
	Associate     bool
	Theme         string // "" means "default"
	InstallWidget bool
}

// SetupResult reports what Apply did.
type SetupResult struct {
	Project         projectdomain.Project
	Created         bool   // false when an existing project was updated
	WidgetInstalled bool   // the widget block is present in the shell rc
	WidgetNote      string // why the widget was skipped or failed, if it was
}

// ShellInfo describes the user's shell for the widget step.
type ShellInfo struct {
	Name      string
	Supported bool
	Note      string // why the widget cannot be installed, when unsupported
}

// SetupState is the current installation, used to prefill a setup.
type SetupState struct {
	Configured      bool
	UserName        string
	Theme           string
	Current         *projectdomain.Project // nil when no current project
	ExistingKeys    map[string]string      // key -> project name
	Themes          []string
	Cwd             string
	DefaultUser     string // $USER
	Shell           ShellInfo
	WidgetInstalled bool
}

// Defaults returns a prefilled input for mode. ModeEdit falls back to
// ModeCreate defaults when there is no current project.
func (s SetupState) Defaults(mode Mode) SetupInput {
	theme := s.Theme
	if theme == "" {
		theme = "default"
	}
	name := s.UserName
	if name == "" {
		name = s.DefaultUser
	}
	if mode == ModeEdit && s.Current != nil {
		return SetupInput{
			Mode:          ModeEdit,
			ProjectID:     s.Current.ID,
			UserName:      name,
			ProjectName:   s.Current.Name,
			ProjectKey:    s.Current.Key,
			Methodology:   s.Current.Methodology,
			Associate:     s.Cwd != "" && s.Current.AssociatedPath == s.Cwd,
			Theme:         theme,
			InstallWidget: s.WidgetInstalled,
		}
	}
	project := DefaultProjectName(s.Cwd)
	return SetupInput{
		Mode:          ModeCreate,
		UserName:      name,
		ProjectName:   project,
		ProjectKey:    SuggestKey(project),
		Methodology:   projectdomain.MethodologyKanban,
		Associate:     s.Cwd != "",
		Theme:         theme,
		InstallWidget: s.WidgetInstalled,
	}
}

// Service applies a SetupInput. It is UI-agnostic: every front end collects
// an input and hands it here.
type Service struct {
	projects *projectapp.ProjectUseCases
	members  MemberRegistrar
	themes   []string
	registry *setupdomain.InstallationRegistry
	getwd    func() (string, error)
	getenv   func(string) string
}

// NewService constructs a Service.
func NewService(projects *projectapp.ProjectUseCases, registry *setupdomain.InstallationRegistry) *Service {
	return &Service{projects: projects, registry: registry, getwd: os.Getwd, getenv: os.Getenv}
}

// WithMembers makes Apply register the user as a human member.
func (s *Service) WithMembers(m MemberRegistrar) *Service {
	s.members = m
	return s
}

// WithThemes sets the theme names a setup may choose from.
func (s *Service) WithThemes(names []string) *Service {
	s.themes = names
	return s
}

// Themes returns the selectable theme names.
func (s *Service) Themes() []string {
	if len(s.themes) == 0 {
		return defaultThemes
	}
	return s.themes
}

// State reads the current configuration, projects and environment.
func (s *Service) State(ctx context.Context) (SetupState, error) {
	cfg, err := config.Load()
	if err != nil {
		return SetupState{}, fmt.Errorf("load config: %w", err)
	}
	st := SetupState{
		Configured:   cfg.UserName != "",
		UserName:     cfg.UserName,
		Theme:        cfg.Theme,
		ExistingKeys: map[string]string{},
		Themes:       s.Themes(),
		DefaultUser:  strings.TrimSpace(s.getenv("USER")),
		Shell:        DetectShell(s.getenv("SHELL")),
	}
	if wd, err := s.getwd(); err == nil && wd != "" {
		st.Cwd = filepath.Clean(wd)
	}
	projects, err := s.projects.ListProjects(ctx)
	if err != nil {
		return SetupState{}, fmt.Errorf("list projects: %w", err)
	}
	for i, p := range projects {
		st.ExistingKeys[p.Key] = p.Name
		if p.ID == cfg.CurrentProjectID {
			st.Current = &projects[i]
		}
	}
	// The project associated with the working directory wins, as it does for
	// the board and the CLI, so a re-run edits the project the user is in.
	if p := associatedProject(projects, st.Cwd); p != nil {
		st.Current = p
	}
	if s.registry != nil {
		for _, item := range s.registry.Items {
			if item.Marker == widgetMarker {
				st.WidgetInstalled = true
			}
		}
	}
	return st, nil
}

// associatedProject returns the project whose associated path is cwd or its
// closest ancestor (longest match wins), or nil.
func associatedProject(projects []projectdomain.Project, cwd string) *projectdomain.Project {
	if cwd == "" {
		return nil
	}
	var best *projectdomain.Project
	bestLen := -1
	for i, p := range projects {
		root := strings.TrimRight(p.AssociatedPath, "/")
		if root == "" {
			continue
		}
		if (cwd == root || strings.HasPrefix(cwd, root+"/")) && len(root) > bestLen {
			best, bestLen = &projects[i], len(root)
		}
	}
	return best
}

// Apply validates in completely, then writes: member, project, config and
// (optionally) the shell widget. A validation error writes nothing.
func (s *Service) Apply(ctx context.Context, in SetupInput) (SetupResult, error) {
	in, existing, err := s.validate(ctx, in)
	if err != nil {
		return SetupResult{}, err
	}

	actorID := in.UserName
	if s.members != nil {
		member, err := s.members.EnsureMember(ctx, in.UserName, memberdomain.KindHuman)
		if err != nil {
			return SetupResult{}, fmt.Errorf("register member %q: %w", in.UserName, err)
		}
		actorID = member.ID
	}
	actor := sharedomain.Actor{ID: actorID, Name: in.UserName, Kind: sharedomain.ActorKindHuman}

	cwd := ""
	if wd, err := s.getwd(); err == nil && wd != "" {
		cwd = filepath.Clean(wd)
	}
	var res SetupResult
	if in.Mode == ModeEdit {
		path := existing.AssociatedPath
		if in.Associate && cwd != "" {
			path = cwd
		}
		res.Project, err = s.projects.UpdateProject(ctx, actor, projectapp.UpdateProjectInput{
			ID: existing.ID, Name: in.ProjectName, Methodology: in.Methodology, AssociatedPath: path,
		})
	} else {
		path := ""
		if in.Associate {
			path = cwd
		}
		res.Created = true
		res.Project, err = s.projects.CreateProject(ctx, actor, projectapp.CreateProjectInput{
			Name: in.ProjectName, Key: in.ProjectKey, Methodology: in.Methodology, AssociatedPath: path,
		})
	}
	if err != nil {
		return SetupResult{}, err
	}

	cfg, err := config.Load()
	if err != nil {
		return SetupResult{}, fmt.Errorf("load config: %w", err)
	}
	cfg.UserName = in.UserName
	cfg.Theme = in.Theme
	cfg.CurrentProjectID = res.Project.ID
	if err := config.Save(cfg); err != nil {
		return SetupResult{}, fmt.Errorf("save config: %w", err)
	}

	if in.InstallWidget {
		shell := DetectShell(s.getenv("SHELL"))
		switch {
		case !shell.Supported:
			res.WidgetNote = shell.Note
		default:
			if err := s.installWidget(shell.Name); err != nil {
				res.WidgetNote = "widget install failed: " + err.Error()
			} else {
				res.WidgetInstalled = true
			}
		}
	}
	return res, nil
}

// validate normalizes in and checks every field, returning all problems at
// once. For ModeEdit it also returns the project being edited.
func (s *Service) validate(ctx context.Context, in SetupInput) (SetupInput, projectdomain.Project, error) {
	var errs []error
	var existing projectdomain.Project

	in.UserName = strings.TrimSpace(in.UserName)
	if in.UserName == "" {
		in.UserName = strings.TrimSpace(s.getenv("USER"))
	}
	if in.UserName == "" {
		errs = append(errs, errors.New("name is required (no $USER to fall back on)"))
	}
	in.ProjectName = strings.TrimSpace(in.ProjectName)
	if in.ProjectName == "" {
		errs = append(errs, errors.New("project name is required"))
	}
	if in.Methodology == "" {
		in.Methodology = projectdomain.MethodologyKanban
	}
	if in.Methodology != projectdomain.MethodologyKanban && in.Methodology != projectdomain.MethodologyScrum {
		errs = append(errs, fmt.Errorf("unknown methodology %q — use kanban or scrum", in.Methodology))
	}
	in.Theme = strings.TrimSpace(in.Theme)
	if in.Theme == "" {
		in.Theme = "default"
	}
	if !contains(s.Themes(), in.Theme) {
		errs = append(errs, fmt.Errorf("unknown theme %q — available: %s", in.Theme, strings.Join(s.Themes(), ", ")))
	}

	switch in.Mode {
	case ModeEdit:
		p, err := s.projects.GetProject(ctx, in.ProjectID)
		if err != nil {
			errs = append(errs, fmt.Errorf("project to edit not found: %q", in.ProjectID))
		}
		existing = p
		in.ProjectKey = p.Key
	case ModeCreate, "":
		in.Mode = ModeCreate
		in.ProjectKey = strings.TrimSpace(in.ProjectKey)
		if err := ValidateKey(in.ProjectKey); err != nil {
			errs = append(errs, err)
		} else if p, err := s.projects.ProjectByKey(ctx, in.ProjectKey); err == nil {
			errs = append(errs, fmt.Errorf("key %s is already used by project %q", in.ProjectKey, p.Name))
		}
	default:
		errs = append(errs, fmt.Errorf("unknown setup mode %q", in.Mode))
	}
	return in, existing, errors.Join(errs...)
}

var keyPattern = regexp.MustCompile(`^[A-Z]{2,5}$`)

// ValidateKey checks the project key format (2-5 uppercase letters A-Z).
func ValidateKey(key string) error {
	if !keyPattern.MatchString(key) {
		return fmt.Errorf("key %q must be 2-5 uppercase letters (A-Z)", key)
	}
	return nil
}

// SuggestKey derives a project key from a name: the initials of its words
// ("Web App" -> "WA"), or the first three letters of a single word
// ("termino" -> "TER"). Only ASCII letters count; "PROJ" when none fit.
func SuggestKey(name string) string {
	words := strings.FieldsFunc(name, func(r rune) bool {
		return !(r < unicode.MaxASCII && unicode.IsLetter(r))
	})
	var key string
	if len(words) >= 2 {
		for _, w := range words {
			if len(key) == 5 {
				break
			}
			key += w[:1]
		}
	} else if len(words) == 1 {
		key = words[0]
		if len(key) > 3 {
			key = key[:3]
		}
	}
	key = strings.ToUpper(key)
	if ValidateKey(key) != nil {
		return "PROJ"
	}
	return key
}

// DefaultProjectName is the working directory's base name, or "My Project".
func DefaultProjectName(cwd string) string {
	base := filepath.Base(filepath.Clean(cwd))
	if cwd == "" || base == "/" || base == "." {
		return "My Project"
	}
	return base
}

// DetectShell classifies $SHELL for the widget. An empty $SHELL is treated
// as bash, matching `termino widget install`.
func DetectShell(shellEnv string) ShellInfo {
	name := filepath.Base(strings.TrimSpace(shellEnv))
	if strings.TrimSpace(shellEnv) == "" {
		name = "bash"
	}
	switch name {
	case "bash", "zsh":
		return ShellInfo{Name: name, Supported: true}
	case "fish":
		return ShellInfo{Name: name, Note: "fish is not supported yet — the widget supports bash and zsh"}
	default:
		return ShellInfo{Name: name, Note: fmt.Sprintf("%s is not supported — the widget supports bash and zsh", name)}
	}
}

// installWidget appends the widget block to the shell's rc file (once) and
// records it in the installation registry.
func (s *Service) installWidget(shell string) error {
	home := s.getenv("HOME")
	if home == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		home = h
	}
	rcPath := filepath.Join(home, "."+shell+"rc")
	return installWidgetBlock(s.registry, rcPath)
}
