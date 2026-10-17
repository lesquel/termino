package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	sprintapp "github.com/lesquel/termino/internal/features/sprint/application"
	taskdomain "github.com/lesquel/termino/internal/features/task/domain"
	"github.com/lesquel/termino/internal/platform/sanitize"
	"github.com/spf13/cobra"
)

const (
	dateLayout          = "2006-01-02"
	defaultSprintLength = 14 * 24 * time.Hour
)

// newSprintCmd returns `termino sprint new|list|start|close|add` for the current
// project. Sprints are given by name (ignoring case) or ID.
func newSprintCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sprint",
		Short: "Plan and run sprints in the current project",
	}
	cmd.AddCommand(newSprintNewCmd(), newSprintListCmd(), newSprintStartCmd(), newSprintCloseCmd(), newSprintAddCmd())
	return cmd
}

func newSprintNewCmd() *cobra.Command {
	var start, end string
	cmd := &cobra.Command{
		Use:   "new <name>",
		Short: "Create a draft sprint",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := cleanName("sprint name", args[0])
			if err != nil {
				return err
			}
			startsAt, endsAt, err := sprintDates(start, end, time.Now())
			if err != nil {
				return err
			}
			projectID, err := currentProjectID()
			if err != nil {
				return err
			}
			uc, err := globalContainer.SprintUseCases()
			if err != nil {
				return err
			}
			s, err := uc.CreateSprint(context.Background(), cliActor(), projectID, name, startsAt, endsAt)
			if err != nil {
				return err
			}
			fmt.Printf("✓ Created sprint %s (%s → %s)\n", s.Name, s.StartsAt.Format(dateLayout), s.EndsAt.Format(dateLayout))
			return nil
		},
	}
	cmd.Flags().StringVar(&start, "start", "", "Start date YYYY-MM-DD (default today)")
	cmd.Flags().StringVar(&end, "end", "", "End date YYYY-MM-DD (default start + 14 days)")
	return cmd
}

// sprintDates parses the --start/--end flags, applying the defaults (today,
// start + 14 days) and rejecting an end that is not after the start.
func sprintDates(start, end string, now time.Time) (time.Time, time.Time, error) {
	startsAt := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	if start != "" {
		t, err := time.Parse(dateLayout, start)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid --start %q (use YYYY-MM-DD)", sanitize.Line(start))
		}
		startsAt = t
	}
	endsAt := startsAt.Add(defaultSprintLength)
	if end != "" {
		t, err := time.Parse(dateLayout, end)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid --end %q (use YYYY-MM-DD)", sanitize.Line(end))
		}
		endsAt = t
	}
	if !endsAt.After(startsAt) {
		return time.Time{}, time.Time{}, fmt.Errorf("sprint end %s must be after its start %s", endsAt.Format(dateLayout), startsAt.Format(dateLayout))
	}
	return startsAt, endsAt, nil
}

func newSprintListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the project's sprints",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectID, err := currentProjectID()
			if err != nil {
				return err
			}
			uc, err := globalContainer.SprintUseCases()
			if err != nil {
				return err
			}
			sprints, err := uc.ListSprints(context.Background(), projectID)
			if err != nil {
				return err
			}
			if len(sprints) == 0 {
				fmt.Println("No sprints yet. Run `termino sprint new <name>`.")
				return nil
			}
			taskUC, err := globalContainer.TaskUseCases()
			if err != nil {
				return err
			}
			for _, s := range sprints {
				tasks, err := taskUC.ListTasks(context.Background(), taskdomain.TaskFilter{ProjectID: projectID, SprintID: s.ID})
				if err != nil {
					return fmt.Errorf("list sprint tasks: %w", err)
				}
				fmt.Printf("  %s\t%s\t%s → %s\t%d task(s)\n", sanitize.Line(s.Name), s.State,
					s.StartsAt.Format(dateLayout), s.EndsAt.Format(dateLayout), len(tasks))
			}
			return nil
		},
	}
}

func newSprintStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start <sprint>",
		Short: "Start a sprint (one active sprint per project)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			projectID, err := currentProjectID()
			if err != nil {
				return err
			}
			s, err := resolveSprint(ctx, projectID, args[0])
			if err != nil {
				return err
			}
			uc, err := globalContainer.SprintUseCases()
			if err != nil {
				return err
			}
			if err := uc.StartSprint(ctx, cliActor(), s.ID); err != nil {
				return err
			}
			fmt.Printf("✓ Started sprint %s\n", sanitize.Line(s.Name))
			return nil
		},
	}
}

func newSprintCloseCmd() *cobra.Command {
	var unfinished string
	cmd := &cobra.Command{
		Use:   "close <sprint>",
		Short: "Close a sprint; unfinished tasks leave it",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			strategy := sprintapp.UnfinishedStrategy(strings.ToLower(strings.TrimSpace(unfinished)))
			if strategy != sprintapp.UnfinishedToBacklog && strategy != sprintapp.UnfinishedToNextSprint {
				return fmt.Errorf("invalid --unfinished %q (use backlog or next_sprint)", sanitize.Line(unfinished))
			}
			projectID, err := currentProjectID()
			if err != nil {
				return err
			}
			s, err := resolveSprint(ctx, projectID, args[0])
			if err != nil {
				return err
			}
			uc, err := globalContainer.SprintUseCases()
			if err != nil {
				return err
			}
			if err := uc.CloseSprint(ctx, cliActor(), s.ID, strategy); err != nil {
				return err
			}
			fmt.Printf("✓ Closed sprint %s\n", sanitize.Line(s.Name))
			return nil
		},
	}
	cmd.Flags().StringVar(&unfinished, "unfinished", string(sprintapp.UnfinishedToBacklog), "Where unfinished tasks go: backlog or next_sprint")
	return cmd
}

func newSprintAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <sprint> <task>...",
		Short: "Put tasks in a sprint",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, ref := range args[1:] {
				if err := putTaskInSprint(context.Background(), ref, args[0]); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

// putTaskInSprint assigns the referenced task to a sprint of its own project.
func putTaskInSprint(ctx context.Context, taskRefArg, sprintRef string) error {
	t, err := resolveTask(ctx, taskRefArg)
	if err != nil {
		return err
	}
	s, err := resolveSprint(ctx, t.ProjectID, sprintRef)
	if err != nil {
		return err
	}
	uc, err := globalContainer.SprintUseCases()
	if err != nil {
		return err
	}
	if err := uc.AddTaskToSprint(ctx, cliActor(), t.ID, s.ID); err != nil {
		return err
	}
	keys, _ := projectKeys(ctx)
	fmt.Printf("✓ %s added to sprint %s\n", taskRef(keys, t), sanitize.Line(s.Name))
	return nil
}
