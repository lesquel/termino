package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	kiroconnector "github.com/lesquel/termino/internal/features/connector/infrastructure/kiro"
	"github.com/lesquel/termino/internal/platform/config"
	"github.com/lesquel/termino/internal/platform/sqlite"
	"github.com/spf13/cobra"
)

// newDoctorCmd returns `termino doctor`.
func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose your Termino installation",
		RunE:  runDoctor,
	}
}

func runDoctor(cmd *cobra.Command, args []string) error {
	pass := 0
	fail := 0

	check := func(label string, ok bool, fix string) {
		if ok {
			fmt.Printf("  ✓ %s\n", label)
			pass++
		} else {
			fmt.Printf("  ✗ %s\n    Fix: %s\n", label, fix)
			fail++
		}
	}

	fmt.Println("Termino Doctor")
	fmt.Println()

	// (a) Binary in PATH.
	_, err := exec.LookPath("termino")
	check("termino binary is in PATH",
		err == nil,
		"Add the directory containing `termino` to your $PATH",
	)

	// (b) Config file exists and is readable.
	cfgPath := filepath.Join(config.ConfigDir(), "config.toml")
	_, cfgErr := os.Stat(cfgPath)
	check("Config file exists",
		cfgErr == nil,
		"Run `termino setup` to create the config file",
	)

	// (c) DB file exists and migrations are up to date.
	dbPath := filepath.Join(config.DataDir(), "termino.db")
	_, dbErr := os.Stat(dbPath)
	dbOk := dbErr == nil
	if dbOk {
		// Verify migrations by opening the DB.
		db, err := sqlite.Open(dbPath)
		if err == nil {
			var count int
			_ = db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count)
			db.Close()
			dbOk = count > 0
		} else {
			dbOk = false
		}
	}
	check("Database exists and migrations are applied",
		dbOk,
		"Run `termino setup` to initialise the database",
	)

	// (d) For each connector in registry: verify MCP entry is present.
	connectors := []interface {
		Agent() string
		Detect() (bool, error)
	}{
		kiroconnector.New(),
	}

	reg, _ := loadRegistry()
	if reg != nil {
		items, _ := reg.List()
		for _, conn := range connectors {
			// Check if any registry item looks like this connector's MCP entry.
			found := false
			for _, item := range items {
				if strings.Contains(item.Path, conn.Agent()) || item.Marker == "termino" {
					found = true
					break
				}
			}
			if found {
				detected, _ := conn.Detect()
				check(
					fmt.Sprintf("%s connector MCP entry present", conn.Agent()),
					detected,
					fmt.Sprintf("Run `termino connect %s` to reinstall", conn.Agent()),
				)
			}
		}
	}

	// (e) DB file permissions are 0600.
	if dbErr == nil {
		info, err := os.Stat(dbPath)
		if err == nil {
			check("DB file permissions are 0600",
				info.Mode().Perm() == 0600,
				fmt.Sprintf("Run: chmod 0600 %s", dbPath),
			)
		}
	}

	// (f) Config file permissions are 0600.
	if cfgErr == nil {
		info, err := os.Stat(cfgPath)
		if err == nil {
			check("Config file permissions are 0600",
				info.Mode().Perm() == 0600,
				fmt.Sprintf("Run: chmod 0600 %s", cfgPath),
			)
		}
	}

	// Summary.
	fmt.Printf("\n%d check(s) passed, %d failed\n", pass, fail)
	if fail > 0 {
		return fmt.Errorf("%d check(s) failed", fail)
	}
	return nil
}
