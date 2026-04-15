package discovery

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"

	"github.com/grandcat/zeroconf"
)

type Device struct {
	Service  string   `json:"service"`
	Instance string   `json:"instance"`
	HostName string   `json:"host_name"`
	Port     int      `json:"port"`
	IPv4     []string `json:"ipv4,omitempty"`
	IPv6     []string `json:"ipv6,omitempty"`
	Text     []string `json:"text,omitempty"`
}

func DefaultServices() []string {
	return []string{
		"_nv_shield_remote._tcp",
		"_androidtvremote._tcp",
		"_androidtvremote2._tcp",
	}
}

func Scan(ctx context.Context, services []string, domain string) ([]Device, error) {
	domain = normalizeDomain(domain)

	var (
		mu      sync.Mutex
		wg      sync.WaitGroup
		records = map[string]Device{}
		errs    []error
	)

	for _, raw := range services {
		service := normalizeService(raw)
		if service == "" {
			continue
		}

		wg.Add(1)
		go func(service string) {
			defer wg.Done()

			resolver, err := zeroconf.NewResolver()
			if err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("create zeroconf resolver for %s: %w", service, err))
				mu.Unlock()
				return
			}

			entries := make(chan *zeroconf.ServiceEntry)
			go func(entries <-chan *zeroconf.ServiceEntry) {
				for {
					select {
					case <-ctx.Done():
						return
					case entry, ok := <-entries:
						if !ok {
							return
						}
						if entry == nil {
							continue
						}

						device := fromEntry(service, entry)
						mu.Lock()
						records[recordKey(device)] = device
						mu.Unlock()
					}
				}
			}(entries)

			if err := resolver.Browse(ctx, service, domain, entries); err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("browse %s: %w", service, err))
				mu.Unlock()
			}

			<-ctx.Done()
		}(service)
	}

	<-ctx.Done()
	wg.Wait()

	results := make([]Device, 0, len(records))
	for _, device := range records {
		results = append(results, device)
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Service != results[j].Service {
			return results[i].Service < results[j].Service
		}
		if results[i].Instance != results[j].Instance {
			return results[i].Instance < results[j].Instance
		}
		if results[i].HostName != results[j].HostName {
			return results[i].HostName < results[j].HostName
		}
		return results[i].Port < results[j].Port
	})

	return results, joinErrors(errs)
}

func fromEntry(service string, entry *zeroconf.ServiceEntry) Device {
	return Device{
		Service:  service,
		Instance: entry.Instance,
		HostName: strings.TrimSuffix(entry.HostName, "."),
		Port:     entry.Port,
		IPv4:     ipStrings(entry.AddrIPv4),
		IPv6:     ipStrings(entry.AddrIPv6),
		Text:     sortedStrings(entry.Text),
	}
}

func recordKey(device Device) string {
	parts := []string{
		device.Service,
		device.Instance,
		device.HostName,
		fmt.Sprintf("%d", device.Port),
		strings.Join(device.IPv4, ","),
		strings.Join(device.IPv6, ","),
	}
	return strings.Join(parts, "|")
}

func ipStrings(ips []net.IP) []string {
	if len(ips) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(ips))
	out := make([]string, 0, len(ips))
	for _, ip := range ips {
		if ip == nil {
			continue
		}
		s := ip.String()
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}

	sort.Strings(out)
	return out
}

func sortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}

func normalizeDomain(domain string) string {
	domain = strings.TrimSpace(domain)
	domain = strings.TrimSuffix(domain, ".")
	if domain == "" {
		return "local"
	}
	return domain
}

func normalizeService(service string) string {
	service = strings.TrimSpace(service)
	service = strings.TrimSuffix(service, ".")
	service = strings.TrimSuffix(service, ".local")
	service = strings.TrimSuffix(service, ".local.")
	return service
}

func joinErrors(errs []error) error {
	filtered := errs[:0]
	for _, err := range errs {
		if err != nil {
			filtered = append(filtered, err)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	if len(filtered) == 1 {
		return filtered[0]
	}
	parts := make([]string, 0, len(filtered))
	for _, err := range filtered {
		parts = append(parts, err.Error())
	}
	return fmt.Errorf(strings.Join(parts, "; "))
}
