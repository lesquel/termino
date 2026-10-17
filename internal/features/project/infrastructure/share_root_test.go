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
