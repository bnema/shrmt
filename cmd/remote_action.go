package cmd

import (
	"context"
	"fmt"
	"time"

	"shield-poc/internal/atvremote"

	"github.com/spf13/cobra"
)

type remoteActionOptions struct {
	Timeout       time.Duration
	Host          string
	Port          int
	CertPath      string
	KeyPath       string
	Discover      bool
	DiscoverAfter time.Duration
}

func (o *remoteActionOptions) addFlags(cmd *cobra.Command, defaultPort int) {
	cmd.Flags().DurationVar(&o.Timeout, "timeout", 10*time.Second, "command timeout")
	cmd.Flags().StringVar(&o.Host, "host", "", "explicit host or IP to connect to")
	cmd.Flags().IntVar(&o.Port, "port", defaultPort, "remote command port")
	cmd.Flags().StringVar(&o.CertPath, "cert", defaultCredentialPath("androidtv-client-cert.pem"), "path to the client certificate PEM file")
	cmd.Flags().StringVar(&o.KeyPath, "key", defaultCredentialPath("androidtv-client-key.pem"), "path to the client private key PEM file")
	cmd.Flags().BoolVar(&o.Discover, "discover", true, "discover the target automatically when --host is not provided")
	cmd.Flags().DurationVar(&o.DiscoverAfter, "discover-timeout", 5*time.Second, "discovery timeout used when --host is not provided")
}

func (o *remoteActionOptions) run(cmd *cobra.Command, action string) error {
	target, err := resolveAndroidTVRemoteTarget(cmd.Context(), o.Host, o.Port, o.Discover, o.DiscoverAfter)
	if err != nil {
		return err
	}
	if target.Device != nil {
		fmt.Printf("Using discovered Android TV target %q at %s:%d\n", target.Device.Instance, target.Host, target.Port)
	}

	fmt.Printf("Sending action %q to %s:%d\n", action, target.Host, target.Port)

	ctx, cancel := context.WithTimeout(cmd.Context(), o.Timeout)
	defer cancel()

	result, err := atvremote.SendKey(ctx, atvremote.SendKeyParams{
		Host:      target.Host,
		Port:      target.Port,
		CertPath:  o.CertPath,
		KeyPath:   o.KeyPath,
		PostDelay: 350 * time.Millisecond,
	}, action)
	if err != nil {
		return err
	}

	fmt.Printf("Action %q sent successfully\n", result.Action)
	fmt.Printf("Features: supported=0x%X active=0x%X\n", result.SupportedFeatures, result.ActiveFeatures)
	if result.HasPowerState {
		fmt.Printf("Power state reported by device: %t\n", result.Powered)
	}
	return nil
}
