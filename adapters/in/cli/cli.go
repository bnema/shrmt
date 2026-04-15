package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"shrmt/core/action"
	"shrmt/core/device"
	"shrmt/core/pairing"
	"shrmt/ports"

	"github.com/spf13/cobra"
)

func NewRoot(ctrl ports.Controller) *cobra.Command {
	root := &cobra.Command{
		Use:           "shrmt",
		Short:         "NVIDIA Shield remote",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.AddCommand(newDiscoverCommand(ctrl))
	root.AddCommand(newPairCommand(ctrl))
	root.AddCommand(newKeyCommand(ctrl))
	root.AddCommand(newPowerCommand(ctrl))
	return root
}

func newDiscoverCommand(ctrl ports.Controller) *cobra.Command {
	var timeout time.Duration
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "discover",
		Short: "Discover Android TV Remote v2 devices",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()
			devices, err := ctrl.Discover(ctx)
			if err != nil {
				return err
			}
			if asJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(devices)
			}
			if len(devices) == 0 {
				fmt.Println("No devices found.")
				return nil
			}
			for _, dev := range devices {
				fmt.Printf("instance=%q host=%s port=%d\n", dev.Instance, dev.HostName, dev.Port)
				if len(dev.IPv4) > 0 {
					fmt.Printf("  ipv4: %s\n", strings.Join(dev.IPv4, ", "))
				}
				if len(dev.IPv6) > 0 {
					fmt.Printf("  ipv6: %s\n", strings.Join(dev.IPv6, ", "))
				}
				if len(dev.Text) > 0 {
					fmt.Printf("  txt:  %s\n", strings.Join(dev.Text, ", "))
				}
			}
			return nil
		},
	}
	cmd.Flags().DurationVar(&timeout, "timeout", 5*time.Second, "discovery timeout")
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit JSON")
	return cmd
}

func newPairCommand(ctrl ports.Controller) *cobra.Command {
	var timeout time.Duration
	var host string
	var port int
	var code string
	cmd := &cobra.Command{
		Use:   "pair",
		Short: "Pair with the target",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()

			fmt.Println("Starting pairing. Check the TV for the pairing code prompt.")

			request := ports.PairRequest{Target: explicitTarget(host, port)}
			if strings.TrimSpace(code) != "" {
				parsed, err := readCode(cmd.InOrStdin(), code)
				if err != nil {
					return err
				}
				request.Code = parsed
			} else {
				request.CodeProvider = func() (pairing.Code, error) {
					return readCode(cmd.InOrStdin(), "")
				}
			}

			state, err := ctrl.Pair(ctx, request)
			if err != nil {
				return err
			}
			fmt.Printf("Pairing complete. Credentials: cert=%s key=%s source=%s\n", state.Credentials.CertPath, state.Credentials.KeyPath, state.Credentials.Source)
			return nil
		},
	}
	cmd.Flags().DurationVar(&timeout, "timeout", 20*time.Second, "pairing timeout")
	cmd.Flags().StringVar(&host, "host", "", "explicit host or IP")
	cmd.Flags().IntVar(&port, "port", 0, "remote command port")
	cmd.Flags().StringVar(&code, "code", "", "6-character hex pairing code")
	return cmd
}

func newKeyCommand(ctrl ports.Controller) *cobra.Command {
	var timeout time.Duration
	var host string
	var port int
	cmd := &cobra.Command{
		Use:   "key <action>",
		Short: "Send a remote action",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()
			act, err := action.Parse(args[0])
			if err != nil {
				return err
			}
			result, err := ctrl.Send(ctx, ports.SendRequest{Target: explicitTarget(host, port), Action: act})
			if err != nil {
				return err
			}
			fmt.Printf("Sent %q successfully\n", result.Action)
			fmt.Printf("Features: supported=0x%X active=0x%X\n", result.SupportedFeatures, result.ActiveFeatures)
			if result.Powered != nil {
				fmt.Printf("Power state: %t\n", *result.Powered)
			}
			return nil
		},
	}
	cmd.Flags().DurationVar(&timeout, "timeout", 10*time.Second, "command timeout")
	cmd.Flags().StringVar(&host, "host", "", "explicit host or IP")
	cmd.Flags().IntVar(&port, "port", 0, "remote command port")
	return cmd
}

func newPowerCommand(ctrl ports.Controller) *cobra.Command {
	var timeout time.Duration
	var host string
	var port int
	cmd := &cobra.Command{
		Use:   "power",
		Short: "Send the power action",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()
			result, err := ctrl.Send(ctx, ports.SendRequest{Target: explicitTarget(host, port), Action: action.Power})
			if err != nil {
				return err
			}
			fmt.Printf("Sent %q successfully\n", result.Action)
			fmt.Printf("Features: supported=0x%X active=0x%X\n", result.SupportedFeatures, result.ActiveFeatures)
			if result.Powered != nil {
				fmt.Printf("Power state: %t\n", *result.Powered)
			}
			return nil
		},
	}
	cmd.Flags().DurationVar(&timeout, "timeout", 10*time.Second, "command timeout")
	cmd.Flags().StringVar(&host, "host", "", "explicit host or IP")
	cmd.Flags().IntVar(&port, "port", 0, "remote command port")
	return cmd
}

func explicitTarget(host string, port int) *device.Target {
	host = strings.TrimSpace(host)
	if host == "" {
		return nil
	}
	return &device.Target{Host: host, Port: port, Label: host}
}

func readCode(r io.Reader, raw string) (pairing.Code, error) {
	if strings.TrimSpace(raw) != "" {
		return pairing.ParseCode(raw)
	}
	reader := bufio.NewReader(r)
	fmt.Print("Enter 6-character pairing code shown on the TV: ")
	text, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return pairing.ParseCode(text)
}
