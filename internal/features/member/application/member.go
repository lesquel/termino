// Package application contains the member feature use cases.
// It imports only domain packages and the Go standard library.
package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	sharedomain "github.com/lesquel/termino/internal/domain"
	memberdomain "github.com/lesquel/termino/internal/features/member/domain"
)

// Actor is a type alias so callers can use the application package directly.
type Actor = sharedomain.Actor

// ErrMemberExists is returned when a member name is already taken (names are
// compared ignoring case).
var ErrMemberExists = errors.New("member already exists")

// MemberUseCases groups all member-related use cases.
type MemberUseCases struct {
	members memberdomain.MemberRepository
}

// NewMemberUseCases constructs a MemberUseCases instance.
func NewMemberUseCases(members memberdomain.MemberRepository) *MemberUseCases {
	return &MemberUseCases{members: members}
}

// CreateMember creates a new workspace member.
func (uc *MemberUseCases) CreateMember(
	ctx context.Context,
	actor Actor,
	name string,
	kind memberdomain.Kind,
) (memberdomain.Member, error) {
	existing, err := uc.members.List(ctx)
	if err != nil {
		return memberdomain.Member{}, fmt.Errorf("list members: %w", err)
	}
	for _, e := range existing {
		if strings.EqualFold(strings.TrimSpace(e.Name), strings.TrimSpace(name)) {
			return memberdomain.Member{}, fmt.Errorf("%w: a member named %q already exists", ErrMemberExists, e.Name)
		}
	}
	m := memberdomain.Member{
		ID:        uuid.NewString(),
		Name:      name,
		Kind:      kind,
		CreatedAt: time.Now().UTC(),
	}
	if err := uc.members.Save(ctx, m); err != nil {
		return memberdomain.Member{}, fmt.Errorf("save member: %w", err)
	}
	return m, nil
}

// ListMembers returns all workspace members.
func (uc *MemberUseCases) ListMembers(ctx context.Context) ([]memberdomain.Member, error) {
	return uc.members.List(ctx)
}

// GetMember retrieves a member by ID.
func (uc *MemberUseCases) GetMember(ctx context.Context, id string) (memberdomain.Member, error) {
	m, err := uc.members.FindByID(ctx, id)
	if err != nil {
		return memberdomain.Member{}, fmt.Errorf("find member: %w", err)
	}
	return m, nil
}

// DeleteMember permanently removes a workspace member.
func (uc *MemberUseCases) DeleteMember(
	ctx context.Context,
	actor Actor,
	id string,
) error {
	return uc.members.Delete(ctx, id)
}

// EnsureMember returns the member identified by name, registering it with the
// given kind when it does not exist yet. An existing member whose ID or name
// equals name is reused, so the call is idempotent and safe on every write.
// The new member's ID is name itself, keeping agent identities readable in
// activity and assignee fields.
func (uc *MemberUseCases) EnsureMember(
	ctx context.Context,
	name string,
	kind memberdomain.Kind,
) (memberdomain.Member, error) {
	if m, err := uc.members.FindByID(ctx, name); err == nil {
		return m, nil
	}
	if m, err := uc.members.FindByName(ctx, name); err == nil {
		return m, nil
	}
	m := memberdomain.Member{
		ID:        name,
		Name:      name,
		Kind:      kind,
		CreatedAt: time.Now().UTC(),
	}
	if err := uc.members.Save(ctx, m); err != nil {
		return memberdomain.Member{}, fmt.Errorf("register member %q: %w", name, err)
	}
	return m, nil
}
