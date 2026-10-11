// Package application contains the member feature use cases.
// It imports only domain packages and the Go standard library.
package application

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	sharedomain "github.com/lesquel/termino/internal/domain"
	memberdomain "github.com/lesquel/termino/internal/features/member/domain"
)

// Actor is a type alias so callers can use the application package directly.
type Actor = sharedomain.Actor

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
