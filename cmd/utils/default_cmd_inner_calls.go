package utils

import (
	"fmt"

	"github.com/spf13/cobra"
)

var PropagatePersistentPreRun = func(cmd *cobra.Command, args []string) {
	if cmd.Parent().PersistentPreRun != nil {
		cmd.Parent().PersistentPreRun(cmd.Parent(), args)
	}
}

var CallHelpCommand = func(cmd *cobra.Command, args []string) error {
	if err := cmd.Help(); err != nil {
		return fmt.Errorf("calling help command: %w", err)
	}
	return nil
}
