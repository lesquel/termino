package application

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	setupdomain "github.com/lesquel/termino/internal/features/setup/domain"
)

const (
	// WidgetMarker identifies the widget block in shell rc files and in the
	// installation registry.
	WidgetMarker = "termino-widget-start"
	// WidgetMarkerEnd closes the widget block.
	WidgetMarkerEnd = "termino-widget-end"
	// widgetBlock is appended (after a blank line) to the rc file.
	widgetBlock = "# --- termino-widget-start ---\n" +
		"if command -v termino &>/dev/null; then termino widget; fi\n" +
		"# --- termino-widget-end ---\n"
)

// widgetMarker keeps the older unexported name used inside this package.
const widgetMarker = WidgetMarker

// WidgetRCPath returns the rc file for shell under home. Only bash and zsh
// are supported.
func WidgetRCPath(home, shell string) (string, error) {
	switch shell {
	case "bash", "zsh":
		return filepath.Join(home, "."+shell+"rc"), nil
	}
	if info := DetectShell(shell); info.Note != "" {
		return "", fmt.Errorf("%s", info.Note)
	}
	return "", fmt.Errorf("unsupported shell %q — use bash or zsh", shell)
}

// InstallWidget appends the widget block to rcPath unless it is already
// there (backing up an existing file first) and records it in registry,
// which may be nil. present reports that the block was already in the file.
func InstallWidget(registry *setupdomain.InstallationRegistry, rcPath string) (present bool, err error) {
	existing, err := os.ReadFile(rcPath)
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("read %s: %w", rcPath, err)
	}
	present = strings.Contains(string(existing), WidgetMarker)
	if !present {
		if err == nil {
			if err := os.WriteFile(rcPath+".termino-bak", existing, 0o600); err != nil {
				return false, fmt.Errorf("backup %s: %w", rcPath, err)
			}
		}
		f, err := os.OpenFile(rcPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return false, fmt.Errorf("open %s: %w", rcPath, err)
		}
		_, writeErr := f.WriteString("\n" + widgetBlock)
		if cerr := f.Close(); writeErr == nil {
			writeErr = cerr
		}
		if writeErr != nil {
			return false, fmt.Errorf("write widget block: %w", writeErr)
		}
	}
	if registry == nil {
		return present, nil
	}
	if err := registry.Record(setupdomain.InstalledItem{Kind: "block", Path: rcPath, Marker: WidgetMarker}); err != nil {
		return present, fmt.Errorf("record widget install: %w", err)
	}
	return present, nil
}

// UninstallWidget removes the widget block recorded in registry from its rc
// file and drops the record. found is false when nothing was recorded.
func UninstallWidget(registry *setupdomain.InstallationRegistry) (path string, found bool, err error) {
	item, ok := installedWidget(registry)
	if !ok {
		return "", false, nil
	}
	if err := RemoveMarkedBlock(item.Path, WidgetMarker, WidgetMarkerEnd); err != nil {
		return item.Path, true, fmt.Errorf("remove block from %s: %w", item.Path, err)
	}
	if err := registry.Remove(item); err != nil {
		return item.Path, true, fmt.Errorf("update registry: %w", err)
	}
	return item.Path, true, nil
}

// installedWidget returns the registry entry of the widget block.
func installedWidget(registry *setupdomain.InstallationRegistry) (setupdomain.InstalledItem, bool) {
	if registry == nil {
		return setupdomain.InstalledItem{}, false
	}
	for _, item := range registry.Items {
		if item.Kind == "block" && item.Marker == WidgetMarker {
			return item, true
		}
	}
	return setupdomain.InstalledItem{}, false
}

// RemoveMarkedBlock removes the lines between the start and end markers
// (inclusive) from path, plus the trailing blank lines the block left. A
// missing file is not an error.
func RemoveMarkedBlock(path, startMarker, endMarker string) error {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	inside := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, startMarker) {
			inside = true
			continue
		}
		if strings.Contains(line, endMarker) {
			inside = false
			continue
		}
		if !inside {
			lines = append(lines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	f.Close()

	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

// WidgetStatus describes the shell widget for a settings screen.
type WidgetStatus struct {
	Shell     ShellInfo
	RCPath    string // rc file for the detected shell; "" when unsupported
	Installed bool
	// InstalledPath is where the block is recorded, which may differ from
	// RCPath when it was installed for another shell.
	InstalledPath string
}

// ShellWidget toggles the widget for the user's shell. It reloads the
// registry before each call so changes made by other termino processes
// are seen.
type ShellWidget struct {
	registry *setupdomain.InstallationRegistry
	shell    ShellInfo
	home     string
}

// NewShellWidget builds a ShellWidget for $SHELL value shellEnv and home.
func NewShellWidget(registry *setupdomain.InstallationRegistry, shellEnv, home string) *ShellWidget {
	return &ShellWidget{registry: registry, shell: DetectShell(shellEnv), home: home}
}

// Status reports the detected shell, its rc file and whether the widget is
// installed.
func (w *ShellWidget) Status() (WidgetStatus, error) {
	if err := w.registry.Load(); err != nil {
		return WidgetStatus{}, err
	}
	st := WidgetStatus{Shell: w.shell}
	if w.shell.Supported {
		st.RCPath, _ = WidgetRCPath(w.home, w.shell.Name)
	}
	if item, ok := installedWidget(w.registry); ok {
		st.Installed = true
		st.InstalledPath = item.Path
	}
	return st, nil
}

// Install adds the widget block to the detected shell's rc file and returns
// its path. Unsupported shells (fish) return an error explaining why.
func (w *ShellWidget) Install() (string, error) {
	if !w.shell.Supported {
		return "", fmt.Errorf("%s", w.shell.Note)
	}
	if err := w.registry.Load(); err != nil {
		return "", err
	}
	rcPath, err := WidgetRCPath(w.home, w.shell.Name)
	if err != nil {
		return "", err
	}
	if _, err := InstallWidget(w.registry, rcPath); err != nil {
		return "", err
	}
	return rcPath, nil
}

// Uninstall removes the recorded widget block and returns the file it was
// removed from ("" when none was installed).
func (w *ShellWidget) Uninstall() (string, error) {
	if err := w.registry.Load(); err != nil {
		return "", err
	}
	path, _, err := UninstallWidget(w.registry)
	return path, err
}
