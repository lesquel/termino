package cli

import (
	"context"
	"fmt"
	"strings"

	memberdomain "github.com/lesquel/termino/internal/features/member/domain"
	"github.com/lesquel/termino/internal/platform/sanitize"
	"github.com/spf13/cobra"
)

// newMemberCmd returns `termino member add|list|remove`.
func newMemberCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "member",
		Short: "Manage the people and agents tasks can be assigned to",
	}
	cmd.AddCommand(newMemberAddCmd(), newMemberListCmd(), newMemberRemoveCmd())
	return cmd
}

func newMemberAddCmd() *cobra.Command {
	var kind string
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Add a member (a person or an agent)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := cleanName("member name", args[0])
			if err != nil {
				return err
			}
			k := memberdomain.Kind(strings.ToLower(strings.TrimSpace(kind)))
			if k != memberdomain.KindHuman && k != memberdomain.KindAgent {
				return fmt.Errorf("invalid kind %q (use human or agent)", sanitize.Line(kind))
			}
			uc, err := globalContainer.MemberUseCases()
			if err != nil {
				return err
			}
			m, err := uc.CreateMember(context.Background(), cliActor(), name, k)
			if err != nil {
				return err
			}
			fmt.Printf("✓ Added %s %s (%s)\n", m.Kind, m.Name, m.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&kind, "kind", string(memberdomain.KindHuman), "Member kind: human or agent")
	return cmd
}

func newMemberListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List members",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			uc, err := globalContainer.MemberUseCases()
			if err != nil {
				return err
			}
			members, err := uc.ListMembers(context.Background())
			if err != nil {
				return err
			}
			if len(members) == 0 {
				fmt.Println("No members yet. Run `termino member add <name>`.")
				return nil
			}
			for _, m := range members {
				fmt.Printf("  %s\t%s\t%s\n", sanitize.Line(m.Name), m.Kind, m.ID)
			}
			return nil
		},
	}
}

func newMemberRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name|id>",
		Short: "Remove a member",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			m, err := resolveMember(ctx, args[0])
			if err != nil {
				return err
			}
			uc, err := globalContainer.MemberUseCases()
			if err != nil {
				return err
			}
			if err := uc.DeleteMember(ctx, cliActor(), m.ID); err != nil {
				return fmt.Errorf("remove member %s (still assigned to tasks or recorded in activity?): %w", sanitize.Line(m.Name), err)
			}
			fmt.Printf("✓ Removed %s\n", sanitize.Line(m.Name))
			return nil
		},
	}
}
