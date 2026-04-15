package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"shield-poc/internal/discovery"

	"github.com/spf13/cobra"
)

var (
	discoverTimeout  time.Duration
	discoverDomain   string
	discoverServices []string
	discoverJSON     bool
)

var discoverCmd = &cobra.Command{
	Use:   "discover",
	Short: "Discover SHIELD / Android TV services on the local network",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(cmd.Context(), discoverTimeout)
		defer cancel()

		results, err := discovery.Scan(ctx, discoverServices, discoverDomain)
		if err != nil {
			return err
		}

		if discoverJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(results)
		}

		if len(results) == 0 {
			fmt.Println("No devices found.")
			fmt.Println("Scanned services:", strings.Join(discoverServices, ", "))
			return nil
		}

		for _, device := range results {
			fmt.Printf("service=%s instance=%q host=%s port=%d\n", device.Service, device.Instance, device.HostName, device.Port)
			if len(device.IPv4) > 0 {
				fmt.Printf("  ipv4: %s\n", strings.Join(device.IPv4, ", "))
			}
			if len(device.IPv6) > 0 {
				fmt.Printf("  ipv6: %s\n", strings.Join(device.IPv6, ", "))
			}
			if len(device.Text) > 0 {
				fmt.Printf("  txt:  %s\n", strings.Join(device.Text, ", "))
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(discoverCmd)

	discoverCmd.Flags().DurationVar(&discoverTimeout, "timeout", 5*time.Second, "discovery timeout")
	discoverCmd.Flags().StringVar(&discoverDomain, "domain", "local", "mDNS domain")
	discoverCmd.Flags().StringSliceVar(&discoverServices, "service", discovery.DefaultServices(), "service(s) to browse")
	discoverCmd.Flags().BoolVar(&discoverJSON, "json", false, "emit JSON")
}
