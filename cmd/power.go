package cmd

import (
	"shield-poc/internal/atvremote"

	"github.com/spf13/cobra"
)

var powerOptions remoteActionOptions

var powerCmd = &cobra.Command{
	Use:   "power",
	Short: "Send a power key action to the Android TV Remote v2 endpoint",
	RunE: func(cmd *cobra.Command, args []string) error {
		return powerOptions.run(cmd, "power")
	},
}

func init() {
	rootCmd.AddCommand(powerCmd)
	powerOptions.addFlags(powerCmd, atvremote.DefaultRemotePort)
}
