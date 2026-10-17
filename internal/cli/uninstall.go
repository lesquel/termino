package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

	// Process items in reverse order.
	for i := len(items) - 1; i >= 0; i-- {
		item := items[i]
		if !confirmAction(scanner, fmt.Sprintf("Remove %s (%s)?", item.Kind, item.Path)) {
			continue
		}

		switch item.Kind {
		case "block":
			marker := item.Marker
			endMarker := marker[:strings.LastIndex(marker, "-start")] + "-end"
			if endMarker == marker {
				endMarker = marker + "-end"
			}
			endMarker = strings.Replace(marker, "-start", "-end", 1)
			if err := removeBlockFromFile(item.Path, marker, endMarker); err != nil {
				fmt.Fprintf(os.Stderr, "warning: remove block from %s: %v\n", item.Path, err)
			} else {
				fmt.Printf("  ✓ Removed block from %s\n", item.Path)
			}
		case "file":
			if err := os.Remove(item.Path); err != nil && !os.IsNotExist(err) {
				fmt.Fprintf(os.Stderr, "warning: remove %s: %v\n", item.Path, err)
			} else {
				fmt.Printf("  ✓ Removed file %s\n", item.Path)
			}
		case "dir":
			if err := os.RemoveAll(item.Path); err != nil {
				fmt.Fprintf(os.Stderr, "warning: remove dir %s: %v\n", item.Path, err)
			} else {
				fmt.Printf("  ✓ Removed directory %s\n", item.Path)
			}
		}
	}

	// Clear the registry (write empty, don't delete).
	emptyReg := setupdomain.NewInstallationRegistry(
		filepath.Join(config.StateDir(), "installed.json"),
	)
	emptyReg.Items = nil
	if err := emptyReg.Save(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: clear registry: %v\n", err)
	}

	if purge {
		// Prompt for each irreversible item.
		dbPath := filepath.Join(config.DataDir(), "termino.db")
		cfgPath := filepath.Join(config.ConfigDir(), "config.toml")
		stateDir := config.StateDir()

		if confirmAction(scanner, fmt.Sprintf("Delete database %s?", dbPath)) {
			if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
				fmt.Fprintf(os.Stderr, "warning: %v\n", err)
			} else {
				fmt.Printf("  ✓ Deleted %s\n", dbPath)
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
	fmt.Println("Run: rm ~/.local/bin/termino  to remove the binary.")
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
