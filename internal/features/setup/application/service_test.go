package application

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	boardinfra "github.com/lesquel/termino/internal/features/board/infrastructure"
	memberapp "github.com/lesquel/termino/internal/features/member/application"
	memberinfra "github.com/lesquel/termino/internal/features/member/infrastructure"
	projectapp "github.com/lesquel/termino/internal/features/project/application"
	projectdomain "github.com/lesquel/termino/internal/features/project/domain"
	projectinfra "github.com/lesquel/termino/internal/features/project/infrastructure"
	setupdomain "github.com/lesquel/termino/internal/features/setup/domain"
	"github.com/lesquel/termino/internal/platform/config"
	"github.com/lesquel/termino/internal/platform/events"
	"github.com/lesquel/termino/internal/platform/sqlite"
)

type fixture struct {
	svc      *Service
	projects *projectapp.ProjectUseCases
	members  *memberapp.MemberUseCases
	home     string
}

// newFixture builds a Service over a fresh database and isolated XDG/HOME.
func newFixture(t *testing.T, shell string) fixture {
	t.Helper()
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))

	db, err := sqlite.Open(filepath.Join(dir, "termino.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	uc := projectapp.NewProjectUseCases(projectinfra.NewProjectRepository(db), boardinfra.NewBoardRepository(db), events.NewBus())
	members := memberapp.NewMemberUseCases(memberinfra.NewMemberRepository(db))
	reg := setupdomain.NewInstallationRegistry(filepath.Join(dir, "state", "installed.json"))
	svc := NewService(uc, reg).WithMembers(members).WithThemes([]string{"default", "spooky"})
	svc.getwd = func() (string, error) { return "/work/web-app", nil }
	svc.getenv = func(k string) string {
		switch k {
		case "USER":
			return "leo"
		case "SHELL":
			return shell
		}
		return ""
	}
	return fixture{svc: svc, projects: uc, members: members, home: home}
}

func TestSuggestKey(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"initials of two words", "Web App", "WA"},
		{"initials capped at five", "a b c d e f g", "ABCDE"},
		{"single word takes three letters", "termino", "TER"},
		{"separators split words", "web-app_v2", "WAV"},
		{"single word shorter than three", "Go", "GO"},
		{"digits and accents are ignored", "Café 2024", "CAF"},
		{"nothing usable falls back", "42", "PROJ"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SuggestKey(tt.in); got != tt.want {
				t.Fatalf("SuggestKey(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestValidateKey(t *testing.T) {
	for _, ok := range []string{"WA", "WEB", "ABCDE"} {
		if err := ValidateKey(ok); err != nil {
			t.Errorf("ValidateKey(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"", "W", "ABCDEF", "W1", "wa", "W A"} {
		if err := ValidateKey(bad); err == nil {
			t.Errorf("ValidateKey(%q) = nil, want error", bad)
		}
	}
}

func TestDetectShell(t *testing.T) {
	tests := []struct {
		env       string
		name      string
		supported bool
	}{
		{"/bin/bash", "bash", true},
		{"/usr/bin/zsh", "zsh", true},
		{"/usr/bin/fish", "fish", false},
		{"", "bash", true},
		{"/bin/tcsh", "tcsh", false},
	}
	for _, tt := range tests {
		got := DetectShell(tt.env)
		if got.Name != tt.name || got.Supported != tt.supported {
			t.Errorf("DetectShell(%q) = %+v", tt.env, got)
		}
		if !got.Supported && got.Note == "" {
			t.Errorf("DetectShell(%q) unsupported without a note", tt.env)
		}
	}
}

func TestStateFirstRunDefaults(t *testing.T) {
	f := newFixture(t, "/bin/bash")
	st, err := f.svc.State(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.Configured {
		t.Fatal("fresh install reported as configured")
	}
	in := st.Defaults(ModeCreate)
	if in.UserName != "leo" || in.ProjectName != "web-app" || in.ProjectKey != "WA" ||
		in.Methodology != projectdomain.MethodologyKanban || !in.Associate || in.Theme != "default" {
		t.Fatalf("defaults = %+v", in)
	}
}

func TestApplyCreateRegistersMemberAndProject(t *testing.T) {
	f := newFixture(t, "/bin/bash")
	ctx := context.Background()
	res, err := f.svc.Apply(ctx, SetupInput{
		Mode: ModeCreate, UserName: "Ana", ProjectName: "Web App", ProjectKey: "WA",
		Methodology: projectdomain.MethodologyScrum, Associate: true, Theme: "spooky", InstallWidget: true,
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !res.Created || res.Project.Key != "WA" || res.Project.AssociatedPath != "/work/web-app" {
		t.Fatalf("result = %+v", res)
	}
	cfg, _ := config.Load()
	if cfg.UserName != "Ana" || cfg.Theme != "spooky" || cfg.CurrentProjectID != res.Project.ID {
		t.Fatalf("config = %+v", cfg)
	}
	if names := userMembers(t, f); len(names) != 1 || names[0] != "Ana" {
		t.Fatalf("user members = %v, want only Ana", names)
	}
	rc, _ := os.ReadFile(filepath.Join(f.home, ".bashrc"))
	if !res.WidgetInstalled || !strings.Contains(string(rc), "termino-widget-start") {
		t.Fatalf("widget not installed: %+v / %q", res, rc)
	}
}

func TestApplyEmptyNameUsesUserNotPlaceholder(t *testing.T) {
	f := newFixture(t, "/bin/bash")
	ctx := context.Background()
	if _, err := f.svc.Apply(ctx, SetupInput{Mode: ModeCreate, ProjectName: "Web", ProjectKey: "WEB"}); err != nil {
		t.Fatal(err)
	}
	if names := userMembers(t, f); len(names) != 1 || names[0] != "leo" {
		t.Fatalf("user members = %v, want only $USER leo (no \"human\" placeholder)", names)
	}
}

func TestApplyValidatesBeforeAnyWrite(t *testing.T) {
	f := newFixture(t, "/bin/bash")
	ctx := context.Background()
	if _, err := f.svc.Apply(ctx, SetupInput{Mode: ModeCreate, ProjectName: "Web", ProjectKey: "WEB"}); err != nil {
		t.Fatal(err)
	}
	cases := map[string]SetupInput{
		"bad key":         {Mode: ModeCreate, UserName: "Bob", ProjectName: "X", ProjectKey: "x1"},
		"duplicate key":   {Mode: ModeCreate, UserName: "Bob", ProjectName: "X", ProjectKey: "WEB"},
		"empty project":   {Mode: ModeCreate, UserName: "Bob", ProjectKey: "XY"},
		"unknown theme":   {Mode: ModeCreate, UserName: "Bob", ProjectName: "X", ProjectKey: "XY", Theme: "nope"},
		"bad methodology": {Mode: ModeCreate, UserName: "Bob", ProjectName: "X", ProjectKey: "XY", Methodology: "waterfall"},
		"missing edit id": {Mode: ModeEdit, UserName: "Bob", ProjectName: "X", ProjectID: "nope"},
	}
	for name, in := range cases {
		if _, err := f.svc.Apply(ctx, in); err == nil {
			t.Errorf("%s: Apply succeeded, want validation error", name)
		}
	}
	cfg, _ := config.Load()
	members := userMembers(t, f)
	projects, _ := f.projects.ListProjects(ctx)
	if cfg.UserName != "leo" || len(members) != 1 || len(projects) != 1 {
		t.Fatalf("a rejected setup wrote state: cfg=%+v members=%v projects=%d", cfg, members, len(projects))
	}
}

func TestApplyEditUpdatesInPlace(t *testing.T) {
	f := newFixture(t, "/bin/bash")
	ctx := context.Background()
	first, err := f.svc.Apply(ctx, SetupInput{Mode: ModeCreate, UserName: "Ana", ProjectName: "Web", ProjectKey: "WEB", Associate: true})
	if err != nil {
		t.Fatal(err)
	}
	st, _ := f.svc.State(ctx)
	if !st.Configured || st.Current == nil || st.Current.ID != first.Project.ID {
		t.Fatalf("state after setup = %+v", st)
	}
	in := st.Defaults(ModeEdit)
	if in.ProjectID != first.Project.ID || in.ProjectName != "Web" || in.ProjectKey != "WEB" || in.UserName != "Ana" {
		t.Fatalf("edit defaults = %+v", in)
	}
	in.ProjectName = "Web App"
	in.ProjectKey = "ZZZ" // ignored: the key is read-only when editing
	in.Methodology = projectdomain.MethodologyScrum
	in.Associate = false // keeps the existing association
	res, err := f.svc.Apply(ctx, in)
	if err != nil {
		t.Fatalf("Apply edit: %v", err)
	}
	projects, _ := f.projects.ListProjects(ctx)
	if res.Created || len(projects) != 1 {
		t.Fatalf("edit created a project: created=%v projects=%d", res.Created, len(projects))
	}
	p := projects[0]
	if p.Name != "Web App" || p.Key != "WEB" || p.Methodology != projectdomain.MethodologyScrum || p.AssociatedPath != "/work/web-app" {
		t.Fatalf("edited project = %+v", p)
	}
}

func TestApplyWidgetOnUnsupportedShellIsSkipped(t *testing.T) {
	f := newFixture(t, "/usr/bin/fish")
	res, err := f.svc.Apply(context.Background(), SetupInput{Mode: ModeCreate, ProjectName: "Web", ProjectKey: "WEB", InstallWidget: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.WidgetInstalled || !strings.Contains(res.WidgetNote, "fish") {
		t.Fatalf("result = %+v, want skipped with a fish note", res)
	}
}

// userMembers returns member names other than the built-in cli/system ones.
func userMembers(t *testing.T, f fixture) []string {
	t.Helper()
	members, err := f.members.ListMembers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, m := range members {
		if m.ID != "cli" && m.ID != "system" {
			names = append(names, m.Name)
		}
	}
	return names
}

// A directory association wins over the configured current project, as it
// does for the board and the CLI, so re-running setup edits the project the
// user is standing in.
func TestStateCurrentPrefersDirectoryAssociation(t *testing.T) {
	f := newFixture(t, "/bin/bash")
	ctx := context.Background()
	web, err := f.svc.Apply(ctx, SetupInput{Mode: ModeCreate, UserName: "Ana", ProjectName: "Web", ProjectKey: "WEB", Associate: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Apply(ctx, SetupInput{Mode: ModeCreate, UserName: "Ana", ProjectName: "Mobile", ProjectKey: "MOB"}); err != nil {
		t.Fatal(err)
	}
	st, err := f.svc.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st.Current == nil || st.Current.ID != web.Project.ID {
		t.Fatalf("current = %+v, want the project associated with the cwd (WEB)", st.Current)
	}

	f.svc.getwd = func() (string, error) { return "/elsewhere", nil }
	st, _ = f.svc.State(ctx)
	if st.Current == nil || st.Current.Key != "MOB" {
		t.Fatalf("outside any association current = %+v, want the configured MOB", st.Current)
	}
}

func TestAssociatedProjectLongestMatch(t *testing.T) {
	ps := []projectdomain.Project{
		{ID: "a", AssociatedPath: "/work"},
		{ID: "b", AssociatedPath: "/work/web/"},
		{ID: "c", AssociatedPath: "/work/webapp"},
		{ID: "d"},
	}
	tests := []struct{ cwd, want string }{
		{"/work/web", "b"},
		{"/work/web/src", "b"},
		{"/work/webapp", "c"},
		{"/work/other", "a"},
		{"/home", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := ""
		if p := associatedProject(ps, tt.cwd); p != nil {
			got = p.ID
		}
		if got != tt.want {
			t.Errorf("associatedProject(%q) = %q, want %q", tt.cwd, got, tt.want)
		}
	}
}
