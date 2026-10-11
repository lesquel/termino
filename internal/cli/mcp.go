package cli

import (
	"github.com/spf13/cobra"
	appmcp "github.com/lesquel/termino/internal/app/mcp"
	"github.com/lesquel/termino/internal/platform/logging"
)

// newMCPCmd returns `termino mcp` — starts the MCP server on stdio.
func newMCPCmd() *cobra.Command {
	var readOnly bool

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

			deps := appmcp.Deps{
				ProjectUC: projectUC,
				BoardUC:   boardUC,
				TaskUC:    taskUC,
				AgentUC:   agentUC,
				Logger:    logging.Logger,
				ReadOnly:  readOnly,
			}

			s := appmcp.NewServer(deps)
			return appmcp.Serve(s, logging.Logger)
		},
	}

	cmd.Flags().BoolVar(&readOnly, "read-only", false, "Start in read-only mode (disables write tools)")
	return cmd
}
