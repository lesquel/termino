package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	xterm "github.com/charmbracelet/x/term"
	setupapp "github.com/lesquel/termino/internal/features/setup/application"
	setupdomain "github.com/lesquel/termino/internal/features/setup/domain"
	widgetapp "github.com/lesquel/termino/internal/features/widget/application"
	widgetdomain "github.com/lesquel/termino/internal/features/widget/domain"
	widgetinfra "github.com/lesquel/termino/internal/features/widget/infrastructure"
	"github.com/lesquel/termino/internal/platform/config"
	"github.com/lesquel/termino/internal/platform/logging"
	"github.com/spf13/cobra"
)

// newWidgetCmd returns `termino widget` and its subcommands.
func newWidgetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "widget",
		Short: "Display the status-bar widget summary",
		RunE:  runWidget,
	}
	cmd.AddCommand(newWidgetInstallCmd(), newWidgetUninstallCmd())
	return cmd
}

func runWidget(cmd *cobra.Command, args []string) error {
	// Ensure logging goes to file, never stdout/stderr (would corrupt widget output).
	logPath := filepath.Join(config.StateDir(), "termino.log")
	_ = logging.Init(logPath)

	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := renderWidget(); err != nil {
			slog.Error("widget render error", "err", err)
		}
	}()

	select {
	case <-done:
		// completed in time
	case <-time.After(100 * time.Millisecond):
		slog.Warn("widget timed out after 100ms")
		os.Exit(0)
	}
	return nil
}

func renderWidget() error {
	cfg, err := globalContainer.Config()
	if err != nil {
		return err
	}
	uc, err := globalContainer.WidgetUseCases()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Millisecond)
	defer cancel()

	// The project associated with the working directory wins; the configured
	// current project is the fallback.
	// "My tasks" are those assigned to the configured user (member ID or
	// name), falling back to the built-in CLI member.
	actor := strings.TrimSpace(cfg.UserName)
	if actor == "" {
		actor = "cli"
	}
	cwd, _ := os.Getwd()
	summary, err := uc.GetSummary(ctx, cfg.CurrentProjectID, actor, cwd)
	if errors.Is(err, widgetapp.ErrNoProject) {
		return fmt.Errorf("%w — run `termino project new --associate-pwd` first", err)
	}
	if err != nil {
		return fmt.Errorf("get summary: %w", err)
	}

	width, _, err := xterm.GetSize(uintptr(os.Stdout.Fd()))
	if err != nil || width <= 0 {
		width = 80
	}

	var renderer widgetdomain.Renderer

	// Select renderer based on terminal width.
	if width < 60 {
		renderer = widgetinfra.NewCompactRenderer()
	} else if width <= 120 {
		renderer = widgetinfra.NewNormalRenderer()
	} else {
		renderer = widgetinfra.NewWideRenderer()
	}

	out, err := renderer.Render(summary, width)
	if err != nil {
		return fmt.Errorf("render: %w", err)
	}
	fmt.Println(out)
	return nil
}

func newWidgetInstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install the widget block into ~/.bashrc or ~/.zshrc",
		RunE:  runWidgetInstall,
	}
	cmd.Flags().String("shell", "", "Shell whose rc file receives the widget: bash or zsh (default: from $SHELL)")
	return cmd
}

func runWidgetInstall(cmd *cobra.Command, args []string) error {
	shell, _ := cmd.Flags().GetString("shell")
	rcPath, err := shellRC(shell)
	if err != nil {
		return err
	}
	reg, err := loadRegistry()
	if err != nil {
		return err
	}

	present, err := setupapp.InstallWidget(reg, rcPath)
	if err != nil {
		return err
	}

	if present {
		fmt.Printf("Widget block already present in %s\n", rcPath)
		return nil
	}
	fmt.Printf("✓ Widget block added to %s\n", rcPath)
	fmt.Println("  Restart your shell or run: source", rcPath)
	return nil
}

func newWidgetUninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Remove the widget block from the shell rc file",
		RunE:  runWidgetUninstall,
	}
}

func runWidgetUninstall(cmd *cobra.Command, args []string) error {
	reg, err := loadRegistry()
	if err != nil {
		return err
	}
	path, found, err := setupapp.UninstallWidget(reg)
	if err != nil {
		return err
	}
	if !found {
		fmt.Println("No widget block found in registry.")
		return nil
	}
	fmt.Printf("✓ Widget block removed from %s\n", path)
	return nil
}

// shellRC returns the rc file for an explicitly named shell, or the one
// detected from $SHELL when shell is empty. Only bash and zsh are supported.
func shellRC(shell string) (string, error) {
	shell = strings.ToLower(strings.TrimSpace(shell))
	if shell == "" {
		return detectShellRC()
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	switch shell {
	case "bash":
		return filepath.Join(home, ".bashrc"), nil
	case "zsh":
		return filepath.Join(home, ".zshrc"), nil
	case "fish":
		return "", fmt.Errorf("fish is not supported yet — the widget supports bash and zsh")
	default:
		return "", fmt.Errorf("unsupported shell %q — use bash or zsh", shell)
	}
}

// detectShellRC returns the path to the shell rc file based on $SHELL.
func detectShellRC() (string, error) {
	shell := os.Getenv("SHELL")
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	switch {
	case strings.Contains(shell, "zsh"):
		return filepath.Join(home, ".zshrc"), nil
	case strings.Contains(shell, "bash"):
		return filepath.Join(home, ".bashrc"), nil
	default:
		// Default to bash.
		return filepath.Join(home, ".bashrc"), nil
	}
}

// removeBlockFromFile removes lines between the start and end markers (inclusive).
func removeBlockFromFile(path, startMarker, endMarker string) error {
	return setupapp.RemoveMarkedBlock(path, startMarker, endMarker)
}

// loadRegistry returns the loaded InstallationRegistry from the state dir.
func loadRegistry() (*setupdomain.InstallationRegistry, error) {
	registryPath := filepath.Join(config.StateDir(), "installed.json")
	reg := setupdomain.NewInstallationRegistry(registryPath)
	if err := reg.Load(); err != nil {
		return nil, fmt.Errorf("load registry: %w", err)
	}
	return reg, nil
}
