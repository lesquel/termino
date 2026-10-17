package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

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
			// Prompt for missing flags when running interactively.
			if name == "" {
				fmt.Print("Project name: ")
				fmt.Scanln(&name) //nolint:errcheck
			}
			if key == "" {
				fmt.Print("Project key (2-5 uppercase letters): ")
				fmt.Scanln(&key) //nolint:errcheck
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

// newProjectListCmd returns `termino project list`.
func newProjectListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all projects",
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
				fmt.Println("No projects found. Run `termino project new` to create one.")
				return nil
			}
			for _, p := range projects {
				fmt.Printf("  %s\t%s\t(%s)\n", p.Key, sanitize.Line(p.Name), p.Methodology)
			}
			return nil
		},
	}
}
