package cli

import (
	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start a component (agent)",
}

func init() {
	rootCmd.AddCommand(startCmd)
}
