package cmd

import "github.com/spf13/cobra"

var rootCmd = &cobra.Command{
	Use:           "shield-poc",
	Short:         "NVIDIA SHIELD TV proof-of-concept CLI",
	SilenceErrors: true,
	SilenceUsage:  true,
}

func Execute() error {
	return rootCmd.Execute()
}
