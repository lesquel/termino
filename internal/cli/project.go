package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/charmbracelet/x/term"
	projectapp "github.com/lesquel/termino/internal/features/project/application"
	projectdomain "github.com/lesquel/termino/internal/features/project/domain"
	"github.com/lesquel/termino/internal/platform/config"
	"github.com/lesquel/termino/internal/platform/sanitize"
	"github.com/spf13/cobra"
)

var keyRegexp = regexp.MustCompile(`^[A-Z]{2,5}$`)

// newProjectCmd returns the `termino project` command and its subcommands.
func newProjectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage projects",
	}
	cmd.AddCommand(newProjectNewCmd())
	cmd.AddCommand(newProjectListCmd())
	cmd.AddCommand(newProjectUseCmd())
	return cmd
}

// newProjectNewCmd returns `termino project new`.
func newProjectNewCmd() *cobra.Command {
	var (
		name         string
		key          string
		methodology  string
		path         string
		associatePwd bool
	)

	cmd := &cobra.Command{
		Use:   "new [name]",
		Short: "Create a new project",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" && len(args) == 1 {
				name = args[0]
			}
			// Prompt for missing values; answers are whole lines, so a
			// name may contain spaces.
			in := bufio.NewReader(cmd.InOrStdin())
			if name == "" {
				name = promptLine(in, cmd.OutOrStdout(), "Project name: ")
			}
			if key == "" {
				key = promptLine(in, cmd.OutOrStdout(), "Project key (2-5 letters): ")
			}
			name = sanitize.Line(name)
			if name == "" {
				return fmt.Errorf("project name is required")
			}
			key = strings.ToUpper(strings.TrimSpace(key))
			if !keyRegexp.MatchString(key) {
				return fmt.Errorf("key must be 2-5 uppercase letters, got %q", key)
			}

			mKind := projectdomain.MethodologyKanban
			if methodology != "" {
				switch strings.ToLower(methodology) {
				case "scrum":
					mKind = projectdomain.MethodologyScrum
				case "kanban":
					mKind = projectdomain.MethodologyKanban
				default:
					return fmt.Errorf("unknown methodology %q (use kanban or scrum)", methodology)
				}
			}

			// Resolve the associated path: an explicit --path wins, otherwise
			// --associate-pwd uses the current working directory.
			associatedPath := strings.TrimSpace(path)
			if associatePwd && associatedPath == "" {
				pwd, err := os.Getwd()
				if err != nil {
					return fmt.Errorf("get working directory: %w", err)
				}
				associatedPath = pwd
			}
			if associatedPath != "" {
				associatedPath = filepath.Clean(associatedPath)
			}

			uc, err := globalContainer.ProjectUseCases()
			if err != nil {
				return err
			}

			actor := cliActor()
			p, err := uc.CreateProject(context.Background(), actor, projectapp.CreateProjectInput{
				Key:            key,
				Name:           name,
				Methodology:    mKind,
				AssociatedPath: associatedPath,
			})
			if err != nil {
				return fmt.Errorf("create project: %w", err)
			}

			// Persist the new project as the current one.
			cfg, err := globalContainer.Config()
			if err != nil {
				return err
			}
			cfg.CurrentProjectID = p.ID
			if err := config.Save(cfg); err != nil {
				// Non-fatal: project was created; just warn.
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not save config: %v\n", err)
			}

			fmt.Printf("Created project %s (%s)\n", p.Key, p.Name)
			if p.AssociatedPath != "" {
				fmt.Printf("  Associated path: %s (widget will auto-detect it here)\n", p.AssociatedPath)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Project name")
	cmd.Flags().StringVar(&key, "key", "", "Project key (2-5 uppercase letters)")
	cmd.Flags().StringVar(&methodology, "methodology", "kanban", "Methodology: kanban or scrum")
	cmd.Flags().StringVar(&path, "path", "", "Associate this project with a filesystem path (for widget CWD detection)")
	cmd.Flags().BoolVar(&associatePwd, "associate-pwd", false, "Associate this project with the current working directory ($PWD)")
	return cmd
}

// promptLine prints prompt and reads one line. When stdin is not a terminal
// the answer is not echoed, so a newline keeps the next prompt on its own line.
func promptLine(in *bufio.Reader, out io.Writer, prompt string) string {
	fmt.Fprint(out, prompt)
	line, _ := in.ReadString('\n')
	if !term.IsTerminal(os.Stdin.Fd()) {
		fmt.Fprintln(out)
	}
	return strings.TrimSpace(line)
}

// newProjectListCmd returns `termino project list`; the current project is
// marked with "*".
func newProjectListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all projects (current marked *)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			uc, err := globalContainer.ProjectUseCases()
			if err != nil {
				return err
			}
			projects, err := uc.ListProjects(context.Background())
			if err != nil {
				return fmt.Errorf("list projects: %w", err)
			}
			if len(projects) == 0 {
				fmt.Println("No projects yet. Run `termino setup` to create one.")
				return nil
			}
			current, _ := currentProjectID()
			for _, p := range projects {
				mark := " "
				if p.ID == current {
					mark = "*"
				}
				fmt.Printf("%s %s\t%s\t(%s)\n", mark, p.Key, sanitize.Line(p.Name), p.Methodology)
			}
			return nil
		},
	}
}

// newProjectUseCmd returns `termino project use <key>`: make a project the
// current one outside directories associated with another project.
func newProjectUseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "use <key>",
		Short: "Make a project the current one",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := projectByKey(context.Background(), strings.TrimSpace(args[0]))
			if err != nil {
				return err
			}
			cfg, err := globalContainer.Config()
			if err != nil {
				return err
			}
			cfg.CurrentProjectID = p.ID
			if err := config.Save(cfg); err != nil {
				return fmt.Errorf("save config: %w", err)
			}
			fmt.Printf("✓ Now using %s (%s)\n", p.Key, sanitize.Line(p.Name))
			if current, err := currentProjectID(); err == nil && current != p.ID {
				fmt.Println("  Note: this directory is associated with another project, which still wins here.")
			}
			return nil
		},
	}
}
