package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/stellar/stellar-disbursement-platform-backend/tools/sdp-setup/internal/workflow"
)

// Execute runs the setup command
func Execute() error {
	root := newRootCommand()
	if err := root.Execute(); err != nil {
		return fmt.Errorf("executing command: %w", err)
	}
	return nil
}

func newRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sdp-setup",
		Short: "SDP Setup wizard",
		Long:  "SDP Setup wizard manages your run configurations for the Stellar Disbursement Platform",
		RunE: func(cmd *cobra.Command, args []string) error {
			return workflow.Execute(cmd.Context())
		},
	}

	return cmd
}
