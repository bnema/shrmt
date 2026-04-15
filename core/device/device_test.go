package device_test

import (
	"errors"
	"testing"

	"shrmt/core/device"
)

func TestResolveTargetExplicitWins(t *testing.T) {
	explicit := &device.Target{Host: "10.0.0.10", Port: 6466}
	saved := &device.Target{Host: "10.0.0.20", Port: 6466}
	got, err := device.ResolveTarget(explicit, saved, []device.Device{{HostName: "ignored"}}, 6466)
	if err != nil {
		t.Fatalf("ResolveTarget returned error: %v", err)
	}
	if got.Host != explicit.Host {
		t.Fatalf("expected explicit host %q, got %q", explicit.Host, got.Host)
	}
}

func TestResolveTargetSavedWinsWhenNoExplicit(t *testing.T) {
	saved := &device.Target{Host: "10.0.0.20", Port: 6466}
	got, err := device.ResolveTarget(nil, saved, nil, 6466)
	if err != nil {
		t.Fatalf("ResolveTarget returned error: %v", err)
	}
	if got.Host != saved.Host {
		t.Fatalf("expected saved host %q, got %q", saved.Host, got.Host)
	}
}

func TestResolveTargetSingleDiscovery(t *testing.T) {
	got, err := device.ResolveTarget(nil, nil, []device.Device{{Instance: "Shield", IPv4: []string{"10.0.0.5"}}}, 6466)
	if err != nil {
		t.Fatalf("ResolveTarget returned error: %v", err)
	}
	if got.Host != "10.0.0.5" {
		t.Fatalf("expected discovered host, got %#v", got)
	}
}

func TestResolveTargetMultipleDevices(t *testing.T) {
	_, err := device.ResolveTarget(nil, nil, []device.Device{{IPv4: []string{"10.0.0.5"}}, {IPv4: []string{"10.0.0.6"}}}, 6466)
	if !errors.Is(err, device.ErrMultipleDevices) {
		t.Fatalf("expected ErrMultipleDevices, got %v", err)
	}
}

func TestTargetFromDeviceNoUsableAddress(t *testing.T) {
	_, err := device.TargetFromDevice(device.Device{}, 6466)
	if !errors.Is(err, device.ErrNoUsableAddress) {
		t.Fatalf("expected ErrNoUsableAddress, got %v", err)
	}
}
