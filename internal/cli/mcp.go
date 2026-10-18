package cli

import (
	"context"
	"strings"

	appmcp "github.com/lesquel/termino/internal/app/mcp"
	"github.com/lesquel/termino/internal/platform/logging"
	"github.com/spf13/cobra"
)

// newMCPCmd returns `termino mcp` — starts the MCP server on stdio.
func newMCPCmd() *cobra.Command {
	var (
		readOnly bool
		actor    string
	)

	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Start the MCP server (stdio transport)",
		Long:  "Starts the Termino MCP server. Communicates over stdio; never prints to stdout.",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Never print to stdout from here — it would break the MCP protocol.
			projectUC, err := globalContainer.ProjectUseCases()
			if err != nil {
				return err
			}
			boardUC, err := globalContainer.BoardUseCases()
			if err != nil {
				return err
			}
			taskUC, err := globalContainer.TaskUseCases()
			if err != nil {
				return err
			}
			agentUC, err := globalContainer.AgentUseCases()
			if err != nil {
				return err
			}
			memberUC, err := globalContainer.MemberUseCases()
			if err != nil {
				return err
			}

			// A task moved to Done by an agent also checks its source box.
			syncSvc, err := newSyncService()
			if err != nil {
				return err
			}
			sources := taskSources()

			deps := appmcp.Deps{
				ProjectUC:     projectUC,
				BoardUC:       boardUC,
				TaskUC:        taskUC,
				AgentUC:       agentUC,
				MemberUC:      memberUC,
				Logger:        logging.Logger,
				ActorOverride: strings.TrimSpace(actor),
				ReadOnly:      readOnly,
				Version:       Version,
				OnTaskDone: func(ctx context.Context, taskID string) error {
					return syncSvc.ExportTask(ctx, taskID, sources)
				},
			}

			// Keep importing teammates' shared files while the server runs.
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			projectID, _ := currentProjectID()
			_, watchDone := watchShared(ctx, projectID)

			s := appmcp.NewServer(deps)
			err = appmcp.Serve(s, logging.Logger)
			cancel()
			<-watchDone
			return err
		},
	}

	cmd.Flags().BoolVar(&readOnly, "read-only", false, "Start in read-only mode (disables write tools)")
	cmd.Flags().StringVar(&actor, "actor", "", "Agent identity for activity and presence (default: MCP client name)")
	return cmd
}
