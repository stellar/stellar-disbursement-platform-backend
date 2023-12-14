package utils

import "github.com/spf13/cobra"

var DefaultPersistentPreRun = func(cmd *cobra.Command, args []string) {
	if cmd.Parent().PersistentPreRun != nil {
		cmd.Parent().PersistentPreRun(cmd.Parent(), args)
	}
}
