package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"shield-poc/internal/discovery"
)

type resolvedTarget struct {
	Host   string
	Port   int
	Device *discovery.Device
}

func resolveAndroidTVRemoteTarget(parent context.Context, explicitHost string, port int, discoverEnabled bool, discoverTimeout time.Duration) (*resolvedTarget, error) {
	host := strings.TrimSpace(explicitHost)
	if host != "" {
		return &resolvedTarget{Host: host, Port: port}, nil
	}
	if !discoverEnabled {
		return nil, errors.New("host is required when discovery is disabled")
	}

	ctx, cancel := context.WithTimeout(parent, discoverTimeout)
	defer cancel()

	devices, err := discovery.Scan(ctx, []string{"_androidtvremote2._tcp"}, "local")
	if err != nil {
		return nil, err
	}
	if len(devices) == 0 {
		return nil, errors.New("no _androidtvremote2._tcp devices found")
	}
	if len(devices) > 1 {
		return nil, fmt.Errorf("multiple Android TV Remote v2 devices found (%d); pass --host explicitly", len(devices))
	}

	device := devices[0]
	address, err := preferredDiscoveryAddress(device)
	if err != nil {
		return nil, err
	}

	return &resolvedTarget{Host: address, Port: port, Device: &device}, nil
}

func preferredDiscoveryAddress(device discovery.Device) (string, error) {
	addresses := append([]string(nil), device.IPv4...)
	addresses = append(addresses, device.IPv6...)
	if len(addresses) > 0 {
		return addresses[0], nil
	}
	if device.HostName != "" {
		return device.HostName, nil
	}
	return "", errors.New("discovered device has no usable address")
}
