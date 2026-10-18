package application

import (
	"context"
	"errors"
	"testing"
)

type fakeSharer struct {
	root   string // "" means not in a git repository
	shared []string
}

func (s *fakeSharer) GitRoot(dir string) (string, error) {
	if s.root == "" {
		return "", errors.New("not inside a git repository")
	}
	return s.root, nil
}

func (s *fakeSharer) Share(_ context.Context, projectID, root string) error {
	s.shared = append(s.shared, projectID+"@"+root)
	return nil
}

func TestSetupShareStep(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t, "/bin/bash")
	sharer := &fakeSharer{root: "/work"}
	f.svc.WithSharer(sharer)

	st, err := f.svc.State(ctx)
	if err != nil || st.ShareRoot != "/work" {
		t.Fatalf("State.ShareRoot = %q, %v", st.ShareRoot, err)
	}
	in := st.Defaults(ModeCreate)
	if in.Share {
		t.Error("sharing must default to no")
	}
	in.Share = true
	res, err := f.svc.Apply(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if res.ShareRoot != "/work" || len(sharer.shared) != 1 || sharer.shared[0] != res.Project.ID+"@/work" {
		t.Errorf("result %+v, shared %v", res, sharer.shared)
	}
}

func TestSetupShareOutsideGitIsNoted(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t, "/bin/bash")
	sharer := &fakeSharer{}
	f.svc.WithSharer(sharer)
	st, _ := f.svc.State(ctx)
	if st.ShareRoot != "" {
		t.Errorf("ShareRoot = %q outside git", st.ShareRoot)
	}
	in := st.Defaults(ModeCreate)
	in.Share = true
	res, err := f.svc.Apply(ctx, in)
	if err != nil || res.ShareRoot != "" || res.ShareNote == "" || len(sharer.shared) != 0 {
		t.Errorf("Apply = %+v, %v (shared %v)", res, err, sharer.shared)
	}
}
