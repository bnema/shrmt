package zeroconf

import (
	"context"
	"fmt"
	"strings"

	"shrmt/core/device"
	intdiscovery "shrmt/internal/discovery"
)

const androidTVRemoteV2Service = "_androidtvremote2._tcp"

type Discoverer struct {
	Domain string
}

func New(domain string) *Discoverer {
	if strings.TrimSpace(domain) == "" {
		domain = "local"
	}
	return &Discoverer{Domain: domain}
}

func (d *Discoverer) Discover(ctx context.Context) ([]device.Device, error) {
	results, err := intdiscovery.Scan(ctx, []string{androidTVRemoteV2Service}, d.Domain)
	if err != nil {
		return nil, err
	}
	out := make([]device.Device, 0, len(results))
	for _, result := range results {
		out = append(out, device.Device{
			ID:       idFor(result.Service, result.Instance, result.HostName, result.Port),
			Service:  result.Service,
			Instance: result.Instance,
			HostName: result.HostName,
			Port:     result.Port,
			IPv4:     append([]string(nil), result.IPv4...),
			IPv6:     append([]string(nil), result.IPv6...),
			Text:     append([]string(nil), result.Text...),
		})
	}
	return out, nil
}

func idFor(service, instance, host string, port int) string {
	return fmt.Sprintf("%s|%s|%s|%d", service, instance, host, port)
}
