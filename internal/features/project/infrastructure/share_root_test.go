package infrastructure_test

import (
	"context"
	"testing"
	"time"

	projectdomain "github.com/lesquel/termino/internal/features/project/domain"
	projectinfra "github.com/lesquel/termino/internal/features/project/infrastructure"
)

func TestProjectRepository_ShareRootRoundTrip(t *testing.T) {
	db := openTestDB(t)
	repo := projectinfra.NewProjectRepository(db)
	ctx := context.Background()

	p := projectdomain.Project{ID: "proj-share", Key: "SHR", Name: "Shared",
		Methodology: projectdomain.MethodologyKanban, CreatedAt: time.Now().UTC().Truncate(time.Second),
		ShareRoot: "/repo"}
	if err := repo.Save(ctx, p); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := repo.FindByKey(ctx, "SHR")
	if err != nil {
		t.Fatalf("FindByKey: %v", err)
	}
	if got.ShareRoot != "/repo" {
		t.Errorf("ShareRoot = %q, want /repo", got.ShareRoot)
	}
	list, err := repo.List(ctx)
	if err != nil || len(list) != 1 || list[0].ShareRoot != "/repo" {
		t.Errorf("List ShareRoot: %+v %v", list, err)
	}
	p.ShareRoot = ""
	if err := repo.Save(ctx, p); err != nil {
		t.Fatalf("Save clear: %v", err)
	}
	if got, _ := repo.FindByID(ctx, "proj-share"); got.ShareRoot != "" {
		t.Errorf("ShareRoot not cleared: %q", got.ShareRoot)
	}
}

// A shared project is picked for any directory inside its share root, even
// when its associated path points elsewhere.
func TestPathDetector_MatchesShareRoot(t *testing.T) {
	db := openTestDB(t)
	repo := projectinfra.NewProjectRepository(db)
	detector := projectinfra.NewPathDetector(db)
	ctx := context.Background()
	for _, p := range []projectdomain.Project{
		{ID: "other", Key: "OTH", Name: "Other", Methodology: projectdomain.MethodologyKanban, AssociatedPath: "/work"},
		{ID: "tie", Key: "TIE", Name: "Tie", Methodology: projectdomain.MethodologyKanban, AssociatedPath: "/work/repo"},
		{ID: "shared", Key: "SHR", Name: "Shared", Methodology: projectdomain.MethodologyKanban, AssociatedPath: "/elsewhere", ShareRoot: "/work/repo"},
	} {
		p.CreatedAt = time.Now().UTC().Truncate(time.Second)
		if err := repo.Save(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	for path, want := range map[string]string{"/work/repo/src": "shared", "/work/repo": "shared", "/work/other": "other", "/elsewhere/x": "shared"} {
		got, err := detector.DetectForPath(path)
		if err != nil || got != want {
			t.Errorf("DetectForPath(%s) = %q, %v; want %q", path, got, err, want)
		}
	}
}
