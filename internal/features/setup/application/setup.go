// Package application provides the SetupWizard that guides users through
// the first-run configuration of Termino.
package application

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	connectordomain "github.com/lesquel/termino/internal/features/connector/domain"
	projectapp "github.com/lesquel/termino/internal/features/project/application"
	projectdomain "github.com/lesquel/termino/internal/features/project/domain"
	setupdomain "github.com/lesquel/termino/internal/features/setup/domain"
	sharedomain "github.com/lesquel/termino/internal/domain"
	"github.com/lesquel/termino/internal/platform/config"
)

// ConnectorLike is the subset of connectordomain.Connector used by SetupWizard.
// Having it here avoids the CLI from needing to import connectordomain.
type ConnectorLike interface {
	Agent() string
	Detect() (bool, error)
	Connect(scope connectordomain.Scope) ([]connectordomain.InstalledItem, error)
}

// SetupWizard orchestrates the first-run setup experience.
type SetupWizard struct {
	projectUC  *projectapp.ProjectUseCases
	connectors []ConnectorLike
	registry   *setupdomain.InstallationRegistry
	in         io.Reader
	out        io.Writer
}

// NewSetupWizard constructs a SetupWizard with no connectors.
func NewSetupWizard(
	projectUC *projectapp.ProjectUseCases,
	registry *setupdomain.InstallationRegistry,
) *SetupWizard {
	return &SetupWizard{
		projectUC: projectUC,
		registry:  registry,
		in:        os.Stdin,
		out:       os.Stdout,
	}
}

// NewSetupWizardWithConnectors constructs a SetupWizard with connectors.
func NewSetupWizardWithConnectors(
	projectUC *projectapp.ProjectUseCases,
	connectors []ConnectorLike,
	registry *setupdomain.InstallationRegistry,
) *SetupWizard {
	return &SetupWizard{
		projectUC:  projectUC,
		connectors: connectors,
		registry:   registry,
		in:         os.Stdin,
		out:        os.Stdout,
	}
}

// Run executes the setup wizard interactively.
func (w *SetupWizard) Run(ctx context.Context) error {
	scanner := bufio.NewScanner(w.in)

	fmt.Fprintln(w.out, "Welcome to Termino! Let's set things up.")

	// Step 1: user name.
	fmt.Fprint(w.out, "Your name (used for activity attribution): ")
	name := readLine(scanner)
	if name == "" {
		name = "human"
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	cfg.UserName = name
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	fmt.Fprintf(w.out, "✓ Name saved: %s\n\n", name)

	// Step 2: create initial project.
	fmt.Fprint(w.out, "Project name (e.g. My Project): ")
	projectName := readLine(scanner)
	if projectName == "" {
		projectName = "My Project"
	}
	fmt.Fprint(w.out, "Project key (2-5 uppercase letters, e.g. PROJ): ")
	projectKey := strings.ToUpper(readLine(scanner))
	if projectKey == "" {
		projectKey = "PROJ"
	}

	actor := sharedomain.Actor{ID: "setup", Name: name, Kind: "human"}
	project, err := w.projectUC.CreateProject(ctx, actor, projectapp.CreateProjectInput{
		Name:        projectName,
		Key:         projectKey,
		Methodology: projectdomain.MethodologyKanban,
	})
	if err != nil {
		return fmt.Errorf("create project: %w", err)
	}
	cfg.CurrentProjectID = project.ID
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	fmt.Fprintf(w.out, "✓ Project created: %s (%s)\n\n", project.Name, project.Key)

	// Step 3: offer connector setup.
	for _, conn := range w.connectors {
		detected, err := conn.Detect()
		if err != nil {
			continue
		}
		if !detected {
			continue
		}
		fmt.Fprintf(w.out, "Found %s — connect? [y/N]: ", conn.Agent())
		answer := readLine(scanner)
		if strings.ToLower(strings.TrimSpace(answer)) != "y" {
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
		fmt.Fprintf(w.out, "✓ %s connected.\n\n", conn.Agent())
	}

	// Step 4: offer widget install.
	fmt.Fprint(w.out, "Install shell widget (runs termino widget on new shells)? [y/N]: ")
	if strings.ToLower(strings.TrimSpace(readLine(scanner))) == "y" {
		if err := installWidgetBlock(w.registry); err != nil {
			fmt.Fprintf(w.out, "  ✗ Widget install failed: %v\n", err)
		} else {
			fmt.Fprintln(w.out, "✓ Widget installed.")
		}
	}

	// Step 5: offer theme selection.
	fmt.Fprint(w.out, "Choose theme [default/spooky] (default): ")
	theme := strings.TrimSpace(readLine(scanner))
	if theme == "" {
		theme = "default"
	}
	cfg.Theme = theme
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	fmt.Fprintf(w.out, "✓ Theme set: %s\n\n", theme)

	fmt.Fprintln(w.out, "Setup complete! Run `termino` to open the board.")
	return nil
}

// readLine reads a trimmed line from the scanner.
func readLine(s *bufio.Scanner) string {
	if s.Scan() {
		return strings.TrimSpace(s.Text())
	}
	return ""
}

// installWidgetBlock adds the shell widget block and records it in the registry.
func installWidgetBlock(registry *setupdomain.InstallationRegistry) error {
	shell := os.Getenv("SHELL")
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	var rcPath string
	switch {
	case strings.Contains(shell, "zsh"):
		rcPath = home + "/.zshrc"
	case strings.Contains(shell, "bash"):
		rcPath = home + "/.bashrc"
	default:
		rcPath = home + "/.bashrc"
	}

	const marker = "termino-widget-start"
	const block = "\n# --- termino-widget-start ---\n" +
		"if command -v termino &>/dev/null; then termino widget; fi\n" +
		"# --- termino-widget-end ---\n"

	// Backup.
	if data, err := os.ReadFile(rcPath); err == nil {
		_ = os.WriteFile(rcPath+".termino-bak", data, 0600)
	}

	// Check not already present.
	existing, _ := os.ReadFile(rcPath)
	if strings.Contains(string(existing), marker) {
		return nil
	}

	f, err := os.OpenFile(rcPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	_, writeErr := f.WriteString(block)
	f.Close()
	if writeErr != nil {
		return writeErr
	}

	return registry.Record(setupdomain.InstalledItem{
		Kind:   "block",
		Path:   rcPath,
		Marker: marker,
	})
}
