package application_test

import (
	"context"
	"errors"
	"testing"

	sharedomain "github.com/lesquel/termino/internal/domain"
	memberapp "github.com/lesquel/termino/internal/features/member/application"
	memberdomain "github.com/lesquel/termino/internal/features/member/domain"
)

var errNotFound = errors.New("not found")

type memRepo struct{ members []memberdomain.Member }

func (r *memRepo) Save(_ context.Context, m memberdomain.Member) error {
	r.members = append(r.members, m)
	return nil
}
func (r *memRepo) FindByID(_ context.Context, id string) (memberdomain.Member, error) {
	for _, m := range r.members {
		if m.ID == id {
			return m, nil
		}
	}
	return memberdomain.Member{}, errNotFound
}
func (r *memRepo) FindByName(_ context.Context, name string) (memberdomain.Member, error) {
	for _, m := range r.members {
		if m.Name == name {
			return m, nil
		}
	}
	return memberdomain.Member{}, errNotFound
}
func (r *memRepo) List(context.Context) ([]memberdomain.Member, error) { return r.members, nil }
func (r *memRepo) Delete(context.Context, string) error                { return nil }

func TestLocalActorUsesSetupUserMember(t *testing.T) {
	repo := &memRepo{members: []memberdomain.Member{
		{ID: "cli", Name: "CLI", Kind: memberdomain.KindHuman},
		{ID: "Leo", Name: "Leo", Kind: memberdomain.KindHuman},
	}}
	uc := memberapp.NewMemberUseCases(repo)
	ctx := context.Background()

	want := sharedomain.Actor{ID: "Leo", Name: "Leo", Kind: sharedomain.ActorKindHuman}
	if got := uc.LocalActor(ctx, "Leo"); got != want {
		t.Errorf("LocalActor(Leo) = %+v, want %+v", got, want)
	}
	cli := sharedomain.Actor{ID: "cli", Name: "cli", Kind: sharedomain.ActorKindHuman}
	for _, name := range []string{"", "Unregistered"} {
		if got := uc.LocalActor(ctx, name); got != cli {
			t.Errorf("LocalActor(%q) = %+v, want the cli fallback", name, got)
		}
	}
	var nilUC *memberapp.MemberUseCases
	if got := nilUC.LocalActor(ctx, "Leo"); got != cli {
		t.Errorf("nil use cases should fall back to cli, got %+v", got)
	}
}
