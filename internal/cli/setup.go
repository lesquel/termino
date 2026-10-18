package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/term"
	kiroconnector "github.com/lesquel/termino/internal/features/connector/infrastructure/kiro"
	projectdomain "github.com/lesquel/termino/internal/features/project/domain"
	setupapp "github.com/lesquel/termino/internal/features/setup/application"
	setuptui "github.com/lesquel/termino/internal/features/setup/presentation/tui"
	sharedomain "github.com/lesquel/termino/internal/features/share/domain"
	shareinfra "github.com/lesquel/termino/internal/features/share/infrastructure"
	"github.com/lesquel/termino/internal/tuikit"
	"github.com/spf13/cobra"
)

// setupFlags holds `termino setup` flags.
type setupFlags struct {
	name, project, key, methodology, theme string
	associate, noAssociate, widget, yes    bool
	share                                  bool
	noTUI                                  bool
	openingBoard                           bool // not a flag: setup runs before the board opens
}

// newSetupCmd returns `termino setup`.
func newSetupCmd() *cobra.Command {
	f := &setupFlags{}
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Set up Termino: user name, project, theme and shell widget",
		Long: "Runs a full-screen wizard in a terminal. Re-running it lets you edit the\n" +
			"current setup or add another project; it never creates duplicates.\n\n" +
			"Without a terminal (or with --no-tui) it asks line by line. With --yes it\n" +
			"applies the flags plus defaults without asking; all values are validated\n" +
			"before anything is written.",
		Example: "  termino setup\n" +
			"  termino setup --yes --name Ana --project \"Web App\" --key WA --theme spooky",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error { return runSetup(cmd, f) },
	}
	fl := cmd.Flags()
	fl.StringVar(&f.name, "name", "", "Your name (default: $USER)")
	fl.StringVar(&f.project, "project", "", "Project name (default: current directory name)")
	fl.StringVar(&f.key, "key", "", "Project key, 2-5 uppercase letters (default: suggested from the name)")
	fl.StringVar(&f.methodology, "methodology", "", "kanban or scrum (default: kanban)")
	fl.StringVar(&f.theme, "theme", "", "Colour theme (see `termino theme`)")
	fl.BoolVar(&f.associate, "associate", false, "Associate the project with the current directory (default for a new project)")
	fl.BoolVar(&f.noAssociate, "no-associate", false, "Do not associate the current directory (when editing, keeps the existing association)")
	fl.BoolVar(&f.widget, "widget", false, "Install the shell widget (bash/zsh)")
	fl.BoolVar(&f.share, "share", false, "Share the board through the git repository of the current directory (see `termino share`)")
	fl.BoolVarP(&f.yes, "yes", "y", false, "Apply flags and defaults without asking")
	fl.BoolVar(&f.noTUI, "no-tui", false, "Ask line by line instead of the full-screen wizard")
	return cmd
}

func runSetup(cmd *cobra.Command, f *setupFlags) error {
	if f.associate && f.noAssociate {
		return errors.New("use either --associate or --no-associate, not both")
	}
	projectUC, err := globalContainer.ProjectUseCases()
	if err != nil {
		return err
	}
	memberUC, err := globalContainer.MemberUseCases()
	if err != nil {
		return err
	}
	reg, err := loadRegistry()
	if err != nil {
		return err
	}
	connectors := []setupapp.ConnectorLike{kiroconnector.New()}
	wizard := setupapp.NewSetupWizardWithConnectors(projectUC, connectors, reg).
		WithMembers(memberUC).
		WithThemes(availableThemes()).
		WithOverrides(f.apply)
	svc := wizard.Service().WithSharer(setupSharer{})
	ctx := context.Background()

	if f.yes {
		return runSetupNonInteractive(ctx, cmd, svc, f)
	}
	interactive := term.IsTerminal(os.Stdin.Fd()) && term.IsTerminal(os.Stdout.Fd())
	if !interactive || f.noTUI {
		return wizard.Run(ctx)
	}
	return runSetupTUI(ctx, cmd, svc, f.openingBoard)
}

// apply copies the flags that were given onto in.
func (f *setupFlags) apply(in *setupapp.SetupInput) {
	if f.name != "" {
		in.UserName = f.name
	}
	if f.project != "" {
		in.ProjectName = f.project
		if f.key == "" && in.Mode == setupapp.ModeCreate {
			in.ProjectKey = setupapp.SuggestKey(f.project)
		}
	}
	if f.key != "" && in.Mode == setupapp.ModeCreate {
		in.ProjectKey = f.key
	}
	if f.methodology != "" {
		in.Methodology = projectdomain.MethodologyKind(strings.ToLower(f.methodology))
	}
	if f.theme != "" {
		in.Theme = f.theme
	}
	if f.associate {
		in.Associate = true
	}
	if f.noAssociate {
		in.Associate = false
	}
	if f.widget {
		in.InstallWidget = true
	}
	if f.share {
		in.Share = true
	}
}

// runSetupNonInteractive applies flags plus defaults. On an existing setup it
// edits the current project, or the project named by --key; a --key no
// project uses yet adds a new project. It never creates a duplicate key.
func runSetupNonInteractive(ctx context.Context, cmd *cobra.Command, svc *setupapp.Service, f *setupFlags) error {
	st, err := svc.State(ctx)
	if err != nil {
		return err
	}
	mode := setupapp.ModeCreate
	if st.Configured && st.Current != nil {
		mode = setupapp.ModeEdit
		if key := strings.ToUpper(strings.TrimSpace(f.key)); key != "" && key != st.Current.Key {
			mode = setupapp.ModeCreate
			if _, taken := st.ExistingKeys[key]; taken {
				projectUC, err := globalContainer.ProjectUseCases()
				if err != nil {
					return err
				}
				p, err := projectUC.ProjectByKey(ctx, key)
				if err != nil {
					return err
				}
				st.Current = &p
				mode = setupapp.ModeEdit
			}
		}
	}
	in := st.Defaults(mode)
	if mode == setupapp.ModeEdit {
		in.Associate = false // keep the existing association unless --associate
	}
	f.apply(&in)
	if mode == setupapp.ModeCreate {
		in.ProjectKey = strings.ToUpper(in.ProjectKey)
	}
	res, err := svc.Apply(ctx, in)
	if err != nil {
		return fmt.Errorf("setup not applied (nothing was changed):\n%w", err)
	}
	printSetupResult(cmd, res)
	return nil
}

// runSetupTUI runs the full-screen wizard.
func runSetupTUI(ctx context.Context, cmd *cobra.Command, svc *setupapp.Service, openingBoard bool) error {
	st, err := svc.State(ctx)
	if err != nil {
		return err
	}
	loadTheme := func(name string) (*tuikit.Theme, error) { return tuikit.Load(name, userThemesDir()) }
	chrome, err := loadTheme(st.Theme)
	if err != nil {
		if chrome, err = tuikit.Load("default", ""); err != nil {
			return err
		}
	}
	apply := func(in setupapp.SetupInput) (setupapp.SetupResult, error) { return svc.Apply(ctx, in) }
	wiz := setuptui.New(st, apply, loadTheme, chrome)
	if openingBoard {
		wiz = wiz.OpeningBoard()
	}
	final, err := tea.NewProgram(wiz, tea.WithAltScreen()).Run()
	if err != nil {
		return fmt.Errorf("setup wizard: %w", err)
	}
	m := final.(setuptui.Model)
	res, done := m.Result()
	if !done {
		fmt.Fprintln(cmd.OutOrStdout(), "Setup cancelled. Nothing was changed.")
		return nil
	}
	if openingBoard {
		return nil // the board opens next; its header names the project
	}
	printSetupResult(cmd, res)
	return nil
}

func printSetupResult(cmd *cobra.Command, res setupapp.SetupResult) {
	out := cmd.OutOrStdout()
	verb := "created"
	if !res.Created {
		verb = "updated"
	}
	fmt.Fprintf(out, "✓ Project %s: %s (%s)\n", verb, res.Project.Name, res.Project.Key)
	if res.WidgetInstalled {
		fmt.Fprintln(out, "✓ Shell widget installed — open a new shell to see it.")
	} else if res.WidgetNote != "" {
		fmt.Fprintf(out, "  Widget skipped: %s\n", res.WidgetNote)
	}
	if res.ShareRoot != "" {
		fmt.Fprintf(out, "✓ Shared through %s — commit it to share the board.\n", filepath.Join(res.ShareRoot, sharedomain.Dir))
	} else if res.ShareNote != "" {
		fmt.Fprintf(out, "  Sharing skipped: %s\n", res.ShareNote)
	}
	fmt.Fprintln(out, "Next steps:")
	for _, s := range setupapp.NextSteps() {
		fmt.Fprintln(out, "  "+s)
	}
}

// setupSharer lets setup share the project like `termino share init`.
type setupSharer struct{}

func (setupSharer) GitRoot(dir string) (string, error) { return shareinfra.FindGitRoot(dir) }

func (setupSharer) Share(ctx context.Context, projectID, root string) error {
	svc, err := globalContainer.ShareService()
	if err != nil {
		return err
	}
	return svc.InitFor(ctx, projectID, root)
}
