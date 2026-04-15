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
	"shield-poc/internal/discovery"

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
	if strings.TrimSpace(pairHost) != "" {
		return strings.TrimSpace(pairHost), pairPort, nil
	}
	if !pairDiscover {
		return "", 0, errors.New("host is required when discovery is disabled")
	}

	ctx, cancel := context.WithTimeout(parent, pairDiscoveryWait)
	defer cancel()

	devices, err := discovery.Scan(ctx, []string{"_androidtvremote2._tcp"}, "local")
	if err != nil {
		return "", 0, err
	}
	if len(devices) == 0 {
		return "", 0, errors.New("no _androidtvremote2._tcp devices found")
	}
	if len(devices) > 1 {
		return "", 0, fmt.Errorf("multiple Android TV Remote v2 devices found (%d); pass --host explicitly", len(devices))
	}

	device := devices[0]
	addresses := append([]string(nil), device.IPv4...)
	addresses = append(addresses, device.IPv6...)
	if len(addresses) == 0 {
		if device.HostName == "" {
			return "", 0, errors.New("discovered device has no usable address")
		}
		addresses = append(addresses, device.HostName)
	}

	fmt.Printf("Using discovered Android TV target %q at %s:%d\n", device.Instance, addresses[0], pairPort)
	return addresses[0], pairPort, nil
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
