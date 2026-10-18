package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	memberdomain "github.com/lesquel/termino/internal/features/member/domain"
	shareapp "github.com/lesquel/termino/internal/features/share/application"
	sharedomain "github.com/lesquel/termino/internal/features/share/domain"
	shareinfra "github.com/lesquel/termino/internal/features/share/infrastructure"
	"github.com/lesquel/termino/internal/platform/sanitize"
)

// newShareCmd returns `termino share`.
func newShareCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "share",
		Short: "Share the board with your team through the git repository",
		Long: "Keeps plain-text files under <repo>/.termino/ in sync with the board.\n" +
			"Commit and push .termino/ with your normal git flow; teammates who pull\n" +
			"and run termino see the same board. Termino never commits or pushes.",
	}
	cmd.AddCommand(newShareInitCmd(), newShareStatusCmd(), newShareOffCmd())
	return cmd
}

func newShareInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Share the current project from the git repository of this directory",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := context.Background()
			projectID, err := currentProjectID()
			if err != nil {
				return err
			}
			root, err := enableSharing(ctx, projectID)
			if err != nil {
				return err
			}
			key, err := projectKey(ctx, projectID)
			if err != nil {
				return err
			}
			n := countSharedFiles(root)
			files := "files"
			if n == 1 {
				files = "file"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Sharing %s through %s (%d %s).\n", key, filepath.Join(root, sharedomain.Dir), n, files)
			fmt.Fprintln(cmd.OutOrStdout(), "Next: commit .termino/ and push; teammates who pull and run termino get the board.")
			return nil
		},
	}
}

// enableSharing shares projectID from the git root of the working directory
// and makes sure the configured user is an exported member.
func enableSharing(ctx context.Context, projectID string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	root, err := shareinfra.FindGitRoot(cwd)
	if errors.Is(err, shareinfra.ErrNotGitRepository) {
		return "", fmt.Errorf("sharing needs a git repository: no .git found in %s or its parents — run `git init` (or cd into your repository) first", cwd)
	}
	if err != nil {
		return "", err
	}
	svc, err := globalContainer.ShareService()
	if err != nil {
		return "", err
	}
	if err := svc.InitFor(ctx, projectID, root); err != nil {
		return "", err
	}
	ensureSharedUser(ctx, projectID)
	return root, nil
}

func newShareStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show where the current project is shared from and what the last import skipped",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := context.Background()
			projectID, err := currentProjectID()
			if err != nil {
				return err
			}
			projectUC, err := globalContainer.ProjectUseCases()
			if err != nil {
				return err
			}
			p, err := projectUC.GetProject(ctx, projectID)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if p.ShareRoot == "" {
				fmt.Fprintf(out, "%s is not shared — run `termino share init` inside its git repository.\n", p.Key)
				return nil
			}
			svc, err := globalContainer.ShareService()
			if err != nil {
				return err
			}
			st, err := svc.Status(ctx, projectID)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "Project:     %s (%s)\n", sanitize.Line(p.Name), p.Key)
			fmt.Fprintf(out, "Shared from: %s\n", p.ShareRoot)
			fmt.Fprintf(out, "Files:       %d in %s\n", countSharedFiles(p.ShareRoot), filepath.Join(p.ShareRoot, sharedomain.Dir))
			last := "never"
			if !st.At.IsZero() {
				last = st.At.Local().Format("2006-01-02 15:04:05")
			}
			fmt.Fprintf(out, "Last import: %s\n", last)
			if len(st.Skipped) == 0 {
				fmt.Fprintln(out, "Skipped:     none")
				return nil
			}
			fmt.Fprintf(out, "Skipped:     %d (fix them and run any termino command to import again)\n", len(st.Skipped))
			for _, s := range st.Skipped {
				fmt.Fprintf(out, "  %s: %s\n", s.Path, sanitize.Line(s.Reason))
			}
			return nil
		},
	}
}

func newShareOffCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "off",
		Short: "Stop sharing the current project (the files stay in the repository)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := context.Background()
			projectID, err := currentProjectID()
			if err != nil {
				return err
			}
			projectUC, err := globalContainer.ProjectUseCases()
			if err != nil {
				return err
			}
			p, err := projectUC.GetProject(ctx, projectID)
			if err != nil {
				return err
			}
			if p.ShareRoot == "" {
				fmt.Fprintf(cmd.OutOrStdout(), "%s is not shared.\n", p.Key)
				return nil
			}
			svc, err := globalContainer.ShareService()
			if err != nil {
				return err
			}
			if err := svc.Off(ctx, projectID); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Stopped sharing %s. The files in %s were left in place (remove them with `git rm -r .termino`).\n",
				p.Key, filepath.Join(p.ShareRoot, sharedomain.Dir))
			return nil
		},
	}
}

// countSharedFiles counts the shared files under root/.termino.
func countSharedFiles(root string) int {
	dir := filepath.Join(root, sharedomain.Dir)
	store := shareinfra.FileStore{}
	top, _ := store.List(dir)
	tasks, _ := store.List(filepath.Join(dir, sharedomain.TasksDir))
	return len(top) + len(tasks)
}

// adoptSharedRepo connects the board shared in the git repository of the
// working directory, if any (see ShareService.Adopt). A project it creates
// or links also gets the configured user as an exported member.
func adoptSharedRepo(ctx context.Context) error {
	cwd, err := os.Getwd()
	if err != nil {
		return nil
	}
	root, err := shareinfra.FindGitRoot(cwd)
	if err != nil {
		return nil
	}
	if _, err := os.Stat(filepath.Join(root, sharedomain.Dir, sharedomain.ProjectFileName)); err != nil {
		return nil
	}
	svc, err := globalContainer.ShareService()
	if err != nil {
		return err
	}
	res, err := svc.Adopt(ctx, root)
	if errors.Is(err, shareapp.ErrSharedElsewhere) {
		return err
	}
	if err != nil {
		fileLogger().Warn("share: adopting the repository's board failed", "root", root, "err", err)
		return nil
	}
	if res.Action == shareapp.AdoptCreated || res.Action == shareapp.AdoptLinked {
		fileLogger().Info("share: adopted the repository's board", "root", root, "project", res.ProjectID)
		ensureSharedUser(ctx, res.ProjectID)
	}
	return nil
}

// ensureSharedUser registers the configured user as a human member (when
// missing) and makes sure project.toml lists it.
func ensureSharedUser(ctx context.Context, projectID string) {
	cfg, err := globalContainer.Config()
	if err != nil || cfg.UserName == "" {
		return
	}
	memberUC, err := globalContainer.MemberUseCases()
	if err != nil {
		return
	}
	if _, err := memberUC.EnsureMember(ctx, cfg.UserName, memberdomain.KindHuman); err != nil {
		fileLogger().Warn("share: register user failed", "user", cfg.UserName, "err", err)
		return
	}
	svc, err := globalContainer.ShareService()
	if err != nil {
		return
	}
	if err := svc.ExportProject(ctx, projectID); err != nil {
		fileLogger().Warn("share: export project failed", "project", projectID, "err", err)
	}
}

// shareToggler backs the settings screen's share row with the same calls as
// `termino share init/off`.
type shareToggler struct{}

func (shareToggler) ShareRoot(projectID string) (string, error) {
	projectUC, err := globalContainer.ProjectUseCases()
	if err != nil {
		return "", err
	}
	p, err := projectUC.GetProject(context.Background(), projectID)
	return p.ShareRoot, err
}

func (shareToggler) EnableShare(projectID string) (string, error) {
	return enableSharing(context.Background(), projectID)
}

func (shareToggler) DisableShare(projectID string) error {
	svc, err := globalContainer.ShareService()
	if err != nil {
		return err
	}
	return svc.Off(context.Background(), projectID)
}
