package cmd

import (
	"fmt"

	"shield-poc/internal/atvremote"

	"github.com/spf13/cobra"
)

var keyOptions remoteActionOptions

var keyCmd = &cobra.Command{
	Use:   "key <action>",
	Short: "Send a key action to the Android TV Remote v2 endpoint",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return keyOptions.run(cmd, args[0])
	},
}

func init() {
	rootCmd.AddCommand(keyCmd)
	keyOptions.addFlags(keyCmd, atvremote.DefaultRemotePort)
	keyCmd.Example = fmt.Sprintf("shield-poc key home\nshield-poc key power\nshield-poc key enter\n\nSupported actions: %s", atvremote.AvailableKeyActions())
}
