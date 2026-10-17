// Package application provides the setup Service (UI-agnostic: it applies a
// SetupInput) and the line-prompt SetupWizard used when stdin is not a
// terminal.
package application

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	connectordomain "github.com/lesquel/termino/internal/features/connector/domain"
	memberdomain "github.com/lesquel/termino/internal/features/member/domain"
	projectapp "github.com/lesquel/termino/internal/features/project/application"
	projectdomain "github.com/lesquel/termino/internal/features/project/domain"
	setupdomain "github.com/lesquel/termino/internal/features/setup/domain"
)

// ConnectorLike is the subset of connectordomain.Connector used by SetupWizard.
// Having it here avoids the CLI from needing to import connectordomain.
type ConnectorLike interface {
	Agent() string
	Detect() (bool, error)
	Connect(scope connectordomain.Scope) ([]connectordomain.InstalledItem, error)
}

// MemberRegistrar registers the configured user as a workspace member.
type MemberRegistrar interface {
	EnsureMember(ctx context.Context, name string, kind memberdomain.Kind) (memberdomain.Member, error)
}

// defaultThemes is offered when no theme list is configured.
var defaultThemes = []string{"default", "spooky"}

// maxAttempts bounds re-prompts for an invalid answer.
const maxAttempts = 3

// errCancelled is returned by prompts when the user cancels.
var errCancelled = errors.New("setup cancelled")

// SetupWizard collects a SetupInput with line prompts (defaults in
// brackets, per-prompt validation) and applies it through a Service.
type SetupWizard struct {
	svc        *Service
	connectors []ConnectorLike
	registry   *setupdomain.InstallationRegistry
	overrides  func(*SetupInput)
	in         io.Reader
	out        io.Writer
}

// NewSetupWizard constructs a SetupWizard with no connectors.
func NewSetupWizard(
	projectUC *projectapp.ProjectUseCases,
	registry *setupdomain.InstallationRegistry,
) *SetupWizard {
	return NewSetupWizardWithConnectors(projectUC, nil, registry)
}

// NewSetupWizardWithConnectors constructs a SetupWizard with connectors.
func NewSetupWizardWithConnectors(
	projectUC *projectapp.ProjectUseCases,
	connectors []ConnectorLike,
	registry *setupdomain.InstallationRegistry,
) *SetupWizard {
	return &SetupWizard{
		svc:        NewService(projectUC, registry),
		connectors: connectors,
		registry:   registry,
		in:         os.Stdin,
		out:        os.Stdout,
	}
}

// WithMembers makes the wizard register the user as a human member, so
// assignments and my-tasks work for them right away.
func (w *SetupWizard) WithMembers(m MemberRegistrar) *SetupWizard {
	w.svc.WithMembers(m)
	return w
}

// WithThemes sets the theme names offered in the theme step.
func (w *SetupWizard) WithThemes(names []string) *SetupWizard {
	w.svc.WithThemes(names)
	return w
}

// WithOverrides adjusts the prompt defaults (e.g. from command-line flags).
func (w *SetupWizard) WithOverrides(f func(*SetupInput)) *SetupWizard {
	w.overrides = f
	return w
}

// Service returns the wizard's setup service.
func (w *SetupWizard) Service() *Service { return w.svc }

// Run executes the setup wizard interactively.
func (w *SetupWizard) Run(ctx context.Context) error {
	p := &prompter{scanner: bufio.NewScanner(w.in), out: w.out}
	st, err := w.svc.State(ctx)
	if err != nil {
		return err
	}

	mode := ModeCreate
	if st.Configured {
		mode, err = w.chooseMode(p, st)
		if errors.Is(err, errCancelled) {
			fmt.Fprintln(w.out, "Setup cancelled. Nothing was changed.")
			return nil
		}
		if err != nil {
			return err
		}
	} else {
		fmt.Fprintln(w.out, "Welcome to Termino! Let's set things up. Press enter to accept a [default].")
	}
	in := st.Defaults(mode)
	if w.overrides != nil {
		w.overrides(&in)
	}
	if err := w.collect(ctx, p, st, &in); err != nil {
		return err
	}

	res, err := w.svc.Apply(ctx, in)
	if err != nil {
		return err
	}
	verb := "created"
	if !res.Created {
		verb = "updated"
	}
	fmt.Fprintf(w.out, "✓ Project %s: %s (%s)\n", verb, res.Project.Name, res.Project.Key)
	if res.WidgetInstalled {
		fmt.Fprintln(w.out, "✓ Widget installed.")
	} else if res.WidgetNote != "" {
		fmt.Fprintf(w.out, "  Widget skipped: %s\n", res.WidgetNote)
	}

	w.offerConnectors(p)

	fmt.Fprintln(w.out, "\nSetup complete! Next steps:")
	for _, line := range NextSteps() {
		fmt.Fprintln(w.out, "  "+line)
	}
	return nil
}

// NextSteps lists the commands shown when setup finishes.
func NextSteps() []string {
	return []string{
		"termino board          open the board",
		"n                      new task (on the board)",
		"?                      help (on the board)",
		`termino add "Fix bug"  add a task from the shell`,
	}
}

// chooseMode asks what to do when Termino is already configured.
func (w *SetupWizard) chooseMode(p *prompter, st SetupState) (Mode, error) {
	current := "no current project"
	if st.Current != nil {
		current = fmt.Sprintf("project %s (%s)", st.Current.Name, st.Current.Key)
	}
	fmt.Fprintf(w.out, "Termino is already set up (user %s, %s).\n", st.UserName, current)
	for attempt := 0; attempt < maxAttempts; attempt++ {
		answer, _ := p.ask("[e]dit current setup, [a]dd another project, or [c]ancel? [c]: ")
		switch strings.ToLower(answer) {
		case "", "c", "cancel", "n", "no":
			return "", errCancelled
		case "e", "edit":
			if st.Current == nil {
				fmt.Fprintln(w.out, "  No current project to edit — adding a new one.")
				return ModeCreate, nil
			}
			return ModeEdit, nil
		case "a", "add":
			return ModeCreate, nil
		}
		fmt.Fprintln(w.out, "  Please answer e, a or c.")
	}
	return "", errCancelled
}

// collect prompts for each field, re-prompting on invalid answers.
func (w *SetupWizard) collect(ctx context.Context, p *prompter, st SetupState, in *SetupInput) error {
	var err error
	nameDefault := in.UserName
	if in.UserName, err = p.askValid("Your name", nameDefault, func(v string) error {
		if v == "" {
			return errors.New("name is required")
		}
		return nil
	}); err != nil {
		return err
	}

	if in.ProjectName, err = p.askValid("Project name", in.ProjectName, func(v string) error {
		if v == "" {
			return errors.New("project name is required")
		}
		return nil
	}); err != nil {
		return err
	}

	if in.Mode == ModeEdit {
		fmt.Fprintf(w.out, "Project key: %s (cannot be changed — task IDs use it)\n", in.ProjectKey)
	} else {
		if in.ProjectKey == "" || in.ProjectKey == SuggestKey(DefaultProjectName(st.Cwd)) {
			in.ProjectKey = SuggestKey(in.ProjectName)
		}
		if in.ProjectKey, err = p.askValid("Project key (2-5 uppercase letters)", in.ProjectKey, func(v string) error {
			return KeyProblem(v, st.ExistingKeys)
		}); err != nil {
			return err
		}
		in.ProjectKey = strings.ToUpper(in.ProjectKey)
	}

	method, err := p.askValid("Methodology (kanban/scrum)", string(in.Methodology), func(v string) error {
		if v = strings.ToLower(v); v != "kanban" && v != "scrum" {
			return errors.New("answer kanban or scrum")
		}
		return nil
	})
	if err != nil {
		return err
	}
	in.Methodology = projectdomain.MethodologyKind(strings.ToLower(method))

	if st.Cwd != "" {
		label := fmt.Sprintf("Associate this project with %s", st.Cwd)
		if in.Mode == ModeEdit && st.Current != nil && st.Current.AssociatedPath != "" && st.Current.AssociatedPath != st.Cwd {
			label += fmt.Sprintf(" (now %s; n keeps it)", st.Current.AssociatedPath)
		}
		if in.Associate, err = p.askYesNo(label, in.Associate); err != nil {
			return err
		}
	}

	themes := w.svc.Themes()
	if in.Theme, err = p.askValid(fmt.Sprintf("Theme (%s)", strings.Join(themes, "/")), in.Theme, func(v string) error {
		if !contains(themes, v) {
			return fmt.Errorf("unknown theme %q", v)
		}
		return nil
	}); err != nil {
		return err
	}

	if !st.Shell.Supported {
		fmt.Fprintf(w.out, "Shell widget: skipped — %s\n", st.Shell.Note)
		in.InstallWidget = false
	} else if in.InstallWidget, err = p.askYesNo(fmt.Sprintf("Install shell widget in %s (status on new shells)", st.Shell.Name), in.InstallWidget); err != nil {
		return err
	}
	_ = ctx
	return nil
}

// KeyProblem returns why key cannot be used for a new project, or nil.
// Lowercase input is accepted (it is upper-cased).
func KeyProblem(key string, existing map[string]string) error {
	key = strings.ToUpper(strings.TrimSpace(key))
	if err := ValidateKey(key); err != nil {
		return err
	}
	if name, taken := existing[key]; taken {
		return fmt.Errorf("key %s is already used by project %q", key, name)
	}
	return nil
}

// offerConnectors asks to connect each detected agent.
func (w *SetupWizard) offerConnectors(p *prompter) {
	for _, conn := range w.connectors {
		detected, err := conn.Detect()
		if err != nil || !detected {
			continue
		}
		yes, err := p.askYesNo(fmt.Sprintf("Found %s — connect", conn.Agent()), false)
		if err != nil || !yes {
			continue
		}
		items, err := conn.Connect(connectordomain.ScopeUser)
		if err != nil {
			fmt.Fprintf(w.out, "  ✗ Connect failed: %v\n", err)
			continue
		}
		for _, item := range items {
			if err := w.registry.Record(setupdomain.InstalledItem{
				Kind:       string(item.Kind),
				Path:       item.Path,
				Marker:     item.Marker,
				BackupPath: item.BackupPath,
			}); err != nil {
				fmt.Fprintf(w.out, "  ✗ Registry record failed: %v\n", err)
			}
		}
		fmt.Fprintf(w.out, "✓ %s connected.\n", conn.Agent())
	}
}

// prompter reads answers line by line.
type prompter struct {
	scanner *bufio.Scanner
	out     io.Writer
}

// ask prints prompt and returns the trimmed answer; ok is false at EOF.
func (p *prompter) ask(prompt string) (string, bool) {
	fmt.Fprint(p.out, prompt)
	if p.scanner.Scan() {
		return strings.TrimSpace(p.scanner.Text()), true
	}
	fmt.Fprintln(p.out)
	return "", false
}

// askValid prompts "label [def]: " until valid answers; blank keeps def.
func (p *prompter) askValid(label, def string, valid func(string) error) (string, error) {
	prompt := label + ": "
	if def != "" {
		prompt = fmt.Sprintf("%s [%s]: ", label, def)
	}
	for attempt := 0; attempt < maxAttempts; attempt++ {
		answer, ok := p.ask(prompt)
		if answer == "" {
			answer = def
		}
		err := valid(answer)
		if err == nil {
			return answer, nil
		}
		if !ok {
			return "", fmt.Errorf("%s: %w", strings.ToLower(label), err)
		}
		fmt.Fprintf(p.out, "  ✗ %v\n", err)
	}
	return "", fmt.Errorf("%s: too many invalid answers", strings.ToLower(label))
}

// askYesNo prompts "label? [Y/n]: " and returns the answer.
func (p *prompter) askYesNo(label string, def bool) (bool, error) {
	hint := "[y/N]"
	if def {
		hint = "[Y/n]"
	}
	for attempt := 0; attempt < maxAttempts; attempt++ {
		answer, ok := p.ask(fmt.Sprintf("%s? %s: ", label, hint))
		switch strings.ToLower(answer) {
		case "":
			return def, nil
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		}
		if !ok {
			return def, nil
		}
		fmt.Fprintln(p.out, "  ✗ Please answer y or n.")
	}
	return false, fmt.Errorf("%s: too many invalid answers", strings.ToLower(label))
}

// contains reports whether names holds name.
func contains(names []string, name string) bool {
	for _, n := range names {
		if n == name {
			return true
		}
	}
	return false
}

// widgetMarker identifies the widget block in shell rc files; it matches the
// marker used by `termino widget install` so either path is idempotent.
const widgetMarker = "termino-widget-start"

// installWidgetBlock adds the shell widget block to rcPath (once) and records
// it in the registry.
func installWidgetBlock(registry *setupdomain.InstallationRegistry, rcPath string) error {
	const block = "\n# --- termino-widget-start ---\n" +
		"if command -v termino &>/dev/null; then termino widget; fi\n" +
		"# --- termino-widget-end ---\n"

	existing, err := os.ReadFile(rcPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if !strings.Contains(string(existing), widgetMarker) {
		if err == nil {
			if err := os.WriteFile(rcPath+".termino-bak", existing, 0600); err != nil {
				return fmt.Errorf("backup %s: %w", rcPath, err)
			}
		}
		f, err := os.OpenFile(rcPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return err
		}
		_, writeErr := f.WriteString(block)
		if cerr := f.Close(); writeErr == nil {
			writeErr = cerr
		}
		if writeErr != nil {
			return writeErr
		}
	}
	if registry == nil {
		return nil
	}
	return registry.Record(setupdomain.InstalledItem{Kind: "block", Path: rcPath, Marker: widgetMarker})
}
