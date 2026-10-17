package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	kiroconnector "github.com/lesquel/termino/internal/features/connector/infrastructure/kiro"
	setupapp "github.com/lesquel/termino/internal/features/setup/application"
	"github.com/spf13/cobra"
)

// newSetupCmd returns `termino setup`.
func newSetupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Set up Termino: user name, first project, and integrations",
		RunE:  runSetup,
	}
}

func runSetup(cmd *cobra.Command, args []string) error {
	cfg, err := globalContainer.Config()
	if err != nil {
		return err
	}

	// Guard: ask if already configured.
	if cfg.UserName != "" {
		fmt.Print("Already configured. Re-run setup? [y/N]: ")
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			answer := strings.TrimSpace(scanner.Text())
			if strings.ToLower(answer) != "y" {
				fmt.Println("Setup aborted.")
				return nil
			}
		}
	}

	projectUC, err := globalContainer.ProjectUseCases()
	if err != nil {
		return err
	}

	reg, err := loadRegistry()
	if err != nil {
		return err
	}

	connectors := []setupapp.ConnectorLike{
		kiroconnector.New(),
	}
	w := setupapp.NewSetupWizardWithConnectors(projectUC, connectors, reg)
	return w.Run(context.Background())
}
