package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	connectordomain "github.com/lesquel/termino/internal/features/connector/domain"
	setupdomain "github.com/lesquel/termino/internal/features/setup/domain"
	"github.com/lesquel/termino/internal/platform/config"
	"github.com/spf13/cobra"
)

// newUninstallCmd returns `termino uninstall [--purge]`.
func newUninstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove all Termino integrations and registered files",
		RunE:  runUninstall,
	}
	cmd.Flags().Bool("purge", false, "Also delete the database, config file, and state directory")
	return cmd
}

func runUninstall(cmd *cobra.Command, args []string) error {
	purge, _ := cmd.Flags().GetBool("purge")

	reg, err := loadRegistry()
	if err != nil {
		return err
	}

	items, err := reg.List()
	if err != nil {
		return err
	}

	scanner := bufio.NewScanner(os.Stdin)

	// Process items in reverse order. Only items actually removed leave the
	// registry; declined or failed ones stay recorded for a later run.
	for i := len(items) - 1; i >= 0; i-- {
		item := items[i]
		if !confirmAction(scanner, fmt.Sprintf("Remove %s (%s)?", item.Kind, item.Path)) {
			continue
		}
		if err := removeInstalledItem(item); err != nil {
			fmt.Fprintf(os.Stderr, "warning: remove %s: %v\n", item.Path, err)
			continue
		}
		if err := reg.Remove(item); err != nil {
			fmt.Fprintf(os.Stderr, "warning: update registry: %v\n", err)
		}
	}

	if purge {
		// Prompt for each irreversible item.
		dbPath := filepath.Join(config.DataDir(), "termino.db")
		cfgPath := filepath.Join(config.ConfigDir(), "config.toml")
		stateDir := config.StateDir()

		if confirmAction(scanner, fmt.Sprintf("Delete database %s?", dbPath)) {
			// The WAL sidecars belong to the database and go with it.
			for _, p := range []string{dbPath, dbPath + "-shm", dbPath + "-wal"} {
				if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
					fmt.Fprintf(os.Stderr, "warning: %v\n", err)
				} else if err == nil {
					fmt.Printf("  ✓ Deleted %s\n", p)
				}
			}
		}
		if confirmAction(scanner, fmt.Sprintf("Delete config %s?", cfgPath)) {
			if err := os.Remove(cfgPath); err != nil && !os.IsNotExist(err) {
				fmt.Fprintf(os.Stderr, "warning: %v\n", err)
			} else {
				fmt.Printf("  ✓ Deleted %s\n", cfgPath)
			}
		}
		if confirmAction(scanner, fmt.Sprintf("Delete state directory %s?", stateDir)) {
			if err := os.RemoveAll(stateDir); err != nil {
				fmt.Fprintf(os.Stderr, "warning: %v\n", err)
			} else {
				fmt.Printf("  ✓ Deleted %s\n", stateDir)
			}
		}
	}

	fmt.Println("\nUninstall complete.")
	fmt.Printf("Run: rm %s  to remove the binary.\n", binaryLocation())
	return nil
}

// binaryLocation returns the running executable's path, falling back to the
// command name when it cannot be resolved.
func binaryLocation() string {
	exe, err := os.Executable()
	if err != nil {
		return "termino"
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		return resolved
	}
	return exe
}

// removeInstalledItem undoes one recorded install. Items owned by a
// connector are removed through it, so shared config files (such as Kiro's
// mcp.json) only lose the termino entry and are never deleted.
func removeInstalledItem(item setupdomain.InstalledItem) error {
	ci := connectordomain.InstalledItem{
		Kind:       connectordomain.ItemKind(item.Kind),
		Path:       item.Path,
		Marker:     item.Marker,
		BackupPath: item.BackupPath,
	}
	for _, agent := range supportedAgents {
		if conn := resolveConnector(agent); conn != nil && conn.Owns(ci) {
			if err := conn.Disconnect([]connectordomain.InstalledItem{ci}); err != nil {
				return err
			}
			fmt.Printf("  ✓ Removed %s integration from %s\n", agent, item.Path)
			return nil
		}
	}

	switch item.Kind {
	case "block":
		endMarker := strings.Replace(item.Marker, "-start", "-end", 1)
		if endMarker == item.Marker {
			endMarker = item.Marker + "-end"
		}
		if err := removeBlockFromFile(item.Path, item.Marker, endMarker); err != nil {
			return err
		}
		fmt.Printf("  ✓ Removed block from %s\n", item.Path)
	case "file":
		if err := os.Remove(item.Path); err != nil && !os.IsNotExist(err) {
			return err
		}
		fmt.Printf("  ✓ Removed file %s\n", item.Path)
	case "dir":
		if err := os.RemoveAll(item.Path); err != nil {
			return err
		}
		fmt.Printf("  ✓ Removed directory %s\n", item.Path)
	default:
		return fmt.Errorf("unknown item kind %q", item.Kind)
	}
	return nil
}

// confirmAction prints prompt and reads y/N from the scanner. Returns true for "y".
func confirmAction(scanner *bufio.Scanner, prompt string) bool {
	fmt.Printf("%s [y/N]: ", prompt)
	if scanner.Scan() {
		return strings.ToLower(strings.TrimSpace(scanner.Text())) == "y"
	}
	return false
}
