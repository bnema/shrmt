package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"shield-poc/internal/atvremote"

	"github.com/spf13/cobra"
)

var (
	pairTimeout       time.Duration
	pairHost          string
	pairPort          int
	pairCode          string
	pairName          string
	pairCertPath      string
	pairKeyPath       string
	pairDiscover      bool
	pairDiscoveryWait time.Duration
)

var pairCmd = &cobra.Command{
	Use:   "pair",
	Short: "Pair with an Android TV Remote v2 endpoint",
	RunE: func(cmd *cobra.Command, args []string) error {
		host, port, err := resolvePairTarget(cmd.Context())
		if err != nil {
			return err
		}

		fmt.Printf("Starting pairing with %s:%d\n", host, port)
		fmt.Println("Check the TV for the pairing code prompt.")

		ctx, cancel := context.WithTimeout(cmd.Context(), pairTimeout)
		defer cancel()

		params := atvremote.PairParams{
			Host:        host,
			Port:        port,
			ClientName:  pairName,
			ServiceName: atvremote.DefaultServiceName,
			PairingCode: pairCode,
			CertPath:    pairCertPath,
			KeyPath:     pairKeyPath,
		}
		if strings.TrimSpace(pairCode) == "" {
			params.CodeProvider = func() (string, error) {
				return promptPairCode(cmd.InOrStdin())
			}
		}

		result, err := atvremote.Pair(ctx, params)
		if err != nil {
			return err
		}

		fmt.Printf("Pairing completed for %s:%d\n", result.Host, result.Port)
		if result.ServerName != "" {
			fmt.Printf("Server name: %s\n", result.ServerName)
		}
		fmt.Printf("Certificates saved:\n")
		fmt.Printf("  cert: %s\n", result.CertPath)
		fmt.Printf("  key:  %s\n", result.KeyPath)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(pairCmd)

	pairCmd.Flags().DurationVar(&pairTimeout, "timeout", 20*time.Second, "pairing timeout")
	pairCmd.Flags().StringVar(&pairHost, "host", "", "explicit host or IP to pair with")
	pairCmd.Flags().IntVar(&pairPort, "port", atvremote.DefaultPairingPort, "pairing port")
	pairCmd.Flags().StringVar(&pairCode, "code", "", "6-character pairing code shown on the TV")
	pairCmd.Flags().StringVar(&pairName, "name", "shield-poc", "client name shown during pairing")
	pairCmd.Flags().StringVar(&pairCertPath, "cert", defaultCredentialPath("androidtv-client-cert.pem"), "path to the client certificate PEM file")
	pairCmd.Flags().StringVar(&pairKeyPath, "key", defaultCredentialPath("androidtv-client-key.pem"), "path to the client private key PEM file")
	pairCmd.Flags().BoolVar(&pairDiscover, "discover", true, "discover the target automatically when --host is not provided")
	pairCmd.Flags().DurationVar(&pairDiscoveryWait, "discover-timeout", 5*time.Second, "discovery timeout used when --host is not provided")
}

func resolvePairTarget(parent context.Context) (string, int, error) {
	target, err := resolveAndroidTVRemoteTarget(parent, pairHost, pairPort, pairDiscover, pairDiscoveryWait)
	if err != nil {
		return "", 0, err
	}
	if target.Device != nil {
		fmt.Printf("Using discovered Android TV target %q at %s:%d\n", target.Device.Instance, target.Host, target.Port)
	}
	return target.Host, target.Port, nil
}

func promptPairCode(r io.Reader) (string, error) {
	reader := bufio.NewReader(r)
	fmt.Print("Enter 6-character pairing code shown on the TV: ")
	code, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return atvremote.NormalizePairingCode(code)
}

func defaultCredentialPath(filename string) string {
	if configDir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(configDir, "shield-poc", filename)
	}
	return filepath.Join("certs", filename)
}
