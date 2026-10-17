package cli

import (
	"fmt"
	"os"

	connectordomain "github.com/lesquel/termino/internal/features/connector/domain"
	kiroconnector "github.com/lesquel/termino/internal/features/connector/infrastructure/kiro"
	setupdomain "github.com/lesquel/termino/internal/features/setup/domain"
	"github.com/spf13/cobra"
)

// newConnectCmd returns `termino connect <agent>` and `termino disconnect <agent>`.
func newConnectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "connect <agent>",
		Short: "Connect an AI agent integration (e.g. kiro)",
		Args:  cobra.ExactArgs(1),
		RunE:  runConnect,
	}
	cmd.AddCommand(newDisconnectCmd())
	return cmd
}

func runConnect(cmd *cobra.Command, args []string) error {
	agent := args[0]
	conn := resolveConnector(agent)
	if conn == nil {
		return fmt.Errorf("unknown agent %q — supported: kiro", agent)
	}

	detected, err := conn.Detect()
	if err != nil {
		return fmt.Errorf("detect %s: %w", agent, err)
	}
	if !detected {
		return fmt.Errorf("%s not found in your environment", agent)
	}

	items, err := conn.Connect(connectordomain.ScopeUser)
	if err != nil {
		return fmt.Errorf("connect %s: %w", agent, err)
	}

	reg, err := loadRegistry()
	if err != nil {
		return err
	}
	for _, item := range items {
		if err := reg.Record(setupdomain.InstalledItem{
			Kind:       string(item.Kind),
			Path:       item.Path,
			Marker:     item.Marker,
			BackupPath: item.BackupPath,
		}); err != nil {
			return fmt.Errorf("record install item: %w", err)
		}
	}

	fmt.Printf("✓ Connected to %s (%d item(s) installed)\n", agent, len(items))
	return nil
}

func newDisconnectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disconnect <agent>",
		Short: "Disconnect an AI agent integration",
		Args:  cobra.ExactArgs(1),
		RunE:  runDisconnect,
	}
}

func runDisconnect(cmd *cobra.Command, args []string) error {
	agent := args[0]
	conn := resolveConnector(agent)
	if conn == nil {
		return fmt.Errorf("unknown agent %q — supported: kiro", agent)
	}

	reg, err := loadRegistry()
	if err != nil {
		return err
	}
	items, err := reg.List()
	if err != nil {
		return err
	}

	// Only this connector's own items: other integrations (shell widget,
	// other agents) stay installed and recorded.
	var connItems []connectordomain.InstalledItem
	for _, item := range items {
		ci := connectordomain.InstalledItem{
			Kind:       connectordomain.ItemKind(item.Kind),
			Path:       item.Path,
			Marker:     item.Marker,
			BackupPath: item.BackupPath,
		}
		if conn.Owns(ci) {
			connItems = append(connItems, ci)
		}
	}

	if err := conn.Disconnect(connItems); err != nil {
		return fmt.Errorf("disconnect %s: %w", agent, err)
	}

	// Remove connector items from registry.
	for _, item := range connItems {
		_ = reg.Remove(setupdomain.InstalledItem{
			Kind:   string(item.Kind),
			Path:   item.Path,
			Marker: item.Marker,
		})
	}

	fmt.Printf("✓ Disconnected from %s\n", agent)
	return nil
}

// resolveConnector returns the Connector for the named agent. The working
// directory is the project that receives project-level files (Kiro steering).
func resolveConnector(agent string) connectordomain.Connector {
	switch agent {
	case "kiro":
		conn := kiroconnector.New()
		if cwd, err := os.Getwd(); err == nil {
			conn.WithProjectRoot(cwd)
		}
		return conn
	default:
		return nil
	}
}
