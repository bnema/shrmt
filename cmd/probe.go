package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"shield-poc/internal/discovery"
	"shield-poc/internal/probe"

	"github.com/spf13/cobra"
)

var (
	probeTimeout         time.Duration
	probeDomain          string
	probeServices        []string
	probeHost            string
	probePorts           []int
	probeAllAddresses    bool
	probeKnownCompanions bool
	probeJSON            bool
)

var probeCmd = &cobra.Command{
	Use:   "probe",
	Short: "Probe discovered or explicit SHIELD / Android TV endpoints",
	RunE: func(cmd *cobra.Command, args []string) error {
		discoveryCtx, cancel := context.WithTimeout(cmd.Context(), probeTimeout)
		defer cancel()

		targets, err := probeTargets(discoveryCtx)
		if err != nil {
			return err
		}

		results := make([]probe.Result, 0, len(targets))
		for _, target := range targets {
			probeCtx, probeCancel := context.WithTimeout(cmd.Context(), probeTimeout)
			results = append(results, probe.Run(probeCtx, target))
			probeCancel()
		}

		if probeJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(results)
		}

		if len(results) == 0 {
			fmt.Println("No probe targets found.")
			return nil
		}

		for _, result := range results {
			target := result.Target
			fmt.Printf("target=%s:%d", target.Address, target.Port)
			if target.Service != "" {
				fmt.Printf(" service=%s", target.Service)
			}
			if target.Instance != "" {
				fmt.Printf(" instance=%q", target.Instance)
			}
			if target.HostName != "" {
				fmt.Printf(" host=%s", target.HostName)
			}
			fmt.Println()
			fmt.Printf("  tcp: %t\n", result.TCPReachable)
			if result.TLS != nil {
				fmt.Printf("  tls: %t", result.TLS.Enabled)
				if result.TLS.Protocol != "" {
					fmt.Printf(" protocol=%s", result.TLS.Protocol)
				}
				if result.TLS.CipherSuite != "" {
					fmt.Printf(" cipher=%s", result.TLS.CipherSuite)
				}
				fmt.Println()
				if result.TLS.CommonName != "" {
					fmt.Printf("  cert_common_name: %s\n", result.TLS.CommonName)
				}
				if result.TLS.IssuerCommonName != "" {
					fmt.Printf("  cert_issuer_common_name: %s\n", result.TLS.IssuerCommonName)
				}
				fmt.Printf("  cert_self_signed: %t\n", result.TLS.SelfSigned)
			}
			if result.Error != "" {
				fmt.Printf("  error: %s\n", result.Error)
			}
			if len(target.Text) > 0 {
				fmt.Printf("  txt: %s\n", strings.Join(target.Text, ", "))
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(probeCmd)

	probeCmd.Flags().DurationVar(&probeTimeout, "timeout", 5*time.Second, "probe timeout")
	probeCmd.Flags().StringVar(&probeDomain, "domain", "local", "mDNS domain")
	probeCmd.Flags().StringSliceVar(&probeServices, "service", discovery.DefaultServices(), "service(s) to browse when host is not specified")
	probeCmd.Flags().StringVar(&probeHost, "host", "", "explicit host or IP to probe")
	probeCmd.Flags().IntSliceVar(&probePorts, "port", nil, "explicit port(s) to probe with --host")
	probeCmd.Flags().BoolVar(&probeAllAddresses, "all-addresses", false, "probe all discovered IP addresses instead of the preferred one")
	probeCmd.Flags().BoolVar(&probeKnownCompanions, "known-companions", true, "probe known companion ports for discovered services")
	probeCmd.Flags().BoolVar(&probeJSON, "json", false, "emit JSON")
}

func probeTargets(ctx context.Context) ([]probe.Target, error) {
	if strings.TrimSpace(probeHost) != "" {
		ports := probePorts
		if len(ports) == 0 {
			ports = []int{6466, 6467, 8987}
		}
		ports = uniquePorts(ports)

		targets := make([]probe.Target, 0, len(ports))
		for _, port := range ports {
			targets = append(targets, probe.Target{
				HostName: probeHost,
				Address:  probeHost,
				Port:     port,
			})
		}
		return targets, nil
	}

	devices, err := discovery.Scan(ctx, probeServices, probeDomain)
	if err != nil {
		return nil, err
	}

	targets := make([]probe.Target, 0, len(devices))
	seen := make(map[string]struct{})
	for _, device := range devices {
		addresses := preferredAddresses(device, probeAllAddresses)
		for _, address := range addresses {
			targets = appendUniqueTarget(targets, seen, probe.Target{
				Service:  device.Service,
				Instance: device.Instance,
				HostName: device.HostName,
				Address:  address,
				Port:     device.Port,
				Text:     append([]string(nil), device.Text...),
			})

			if probeKnownCompanions && device.Service == "_androidtvremote2._tcp" && device.Port == 6466 {
				targets = appendUniqueTarget(targets, seen, probe.Target{
					Service:  device.Service,
					Instance: device.Instance,
					HostName: device.HostName,
					Address:  address,
					Port:     6467,
					Text:     append([]string(nil), device.Text...),
				})
			}
		}
	}

	return targets, nil
}

func appendUniqueTarget(targets []probe.Target, seen map[string]struct{}, target probe.Target) []probe.Target {
	key := fmt.Sprintf("%s|%s|%s|%d", target.Service, target.Instance, target.Address, target.Port)
	if _, ok := seen[key]; ok {
		return targets
	}
	seen[key] = struct{}{}
	return append(targets, target)
}

func preferredAddresses(device discovery.Device, all bool) []string {
	addresses := make([]string, 0, len(device.IPv4)+len(device.IPv6)+1)
	addresses = append(addresses, device.IPv4...)
	addresses = append(addresses, device.IPv6...)
	if len(addresses) == 0 && device.HostName != "" {
		addresses = append(addresses, device.HostName)
	}
	if !all && len(addresses) > 1 {
		return addresses[:1]
	}
	return addresses
}

func uniquePorts(ports []int) []int {
	seen := make(map[int]struct{}, len(ports))
	out := make([]int, 0, len(ports))
	for _, port := range ports {
		if port <= 0 {
			continue
		}
		if _, ok := seen[port]; ok {
			continue
		}
		seen[port] = struct{}{}
		out = append(out, port)
	}
	sort.Ints(out)
	return out
}
