package device

import (
	"context"
	"errors"
	"fmt"
)

var (
	ErrMultipleDevices = errors.New("multiple devices found")
	ErrNoDevices       = errors.New("no devices found")
	ErrNoSavedTarget   = errors.New("no saved target")
	ErrNoUsableAddress = errors.New("device has no usable address")
)

type Device struct {
	ID       string
	Service  string
	Instance string
	HostName string
	Port     int
	IPv4     []string
	IPv6     []string
	Text     []string
}

type Target struct {
	Host     string
	Port     int
	DeviceID string
	Label    string
}

func (t Target) IsZero() bool {
	return t.Host == ""
}

type Discoverer interface {
	Discover(ctx context.Context) ([]Device, error)
}

type TargetStore interface {
	Load(ctx context.Context) (Target, error)
	Save(ctx context.Context, target Target) error
	Clear(ctx context.Context) error
}

type Service struct {
	discoverer  Discoverer
	targetStore TargetStore
	defaultPort int
}

func NewService(discoverer Discoverer, targetStore TargetStore, defaultPort int) *Service {
	return &Service{
		discoverer:  discoverer,
		targetStore: targetStore,
		defaultPort: defaultPort,
	}
}

func (s *Service) Discover(ctx context.Context) ([]Device, error) {
	if s.discoverer == nil {
		return nil, ErrNoDevices
	}
	return s.discoverer.Discover(ctx)
}

func (s *Service) LoadDefault(ctx context.Context) (Target, error) {
	if s.targetStore == nil {
		return Target{}, ErrNoSavedTarget
	}
	return s.targetStore.Load(ctx)
}

func (s *Service) SaveDefault(ctx context.Context, target Target) error {
	if s.targetStore == nil {
		return nil
	}
	return s.targetStore.Save(ctx, normalizeTarget(target, s.defaultPort))
}

func (s *Service) ClearDefault(ctx context.Context) error {
	if s.targetStore == nil {
		return nil
	}
	return s.targetStore.Clear(ctx)
}

func (s *Service) Resolve(ctx context.Context, explicit *Target) (Target, error) {
	if explicit != nil && !explicit.IsZero() {
		return normalizeTarget(*explicit, s.defaultPort), nil
	}

	if s.targetStore != nil {
		target, err := s.targetStore.Load(ctx)
		switch {
		case err == nil:
			return normalizeTarget(target, s.defaultPort), nil
		case errors.Is(err, ErrNoSavedTarget):
		default:
			return Target{}, err
		}
	}

	devs, err := s.Discover(ctx)
	if err != nil {
		return Target{}, err
	}
	return ResolveTarget(nil, nil, devs, s.defaultPort)
}

func ResolveTarget(explicit *Target, saved *Target, discovered []Device, defaultPort int) (Target, error) {
	if explicit != nil && !explicit.IsZero() {
		return normalizeTarget(*explicit, defaultPort), nil
	}
	if saved != nil && !saved.IsZero() {
		return normalizeTarget(*saved, defaultPort), nil
	}
	if len(discovered) == 0 {
		return Target{}, ErrNoDevices
	}
	if len(discovered) > 1 {
		return Target{}, fmt.Errorf("%w (%d)", ErrMultipleDevices, len(discovered))
	}
	return TargetFromDevice(discovered[0], defaultPort)
}

func TargetFromDevice(dev Device, defaultPort int) (Target, error) {
	var host string
	if len(dev.IPv4) > 0 {
		host = dev.IPv4[0]
	} else if len(dev.IPv6) > 0 {
		host = dev.IPv6[0]
	} else if dev.HostName != "" {
		host = dev.HostName
	}
	if host == "" {
		return Target{}, ErrNoUsableAddress
	}
	port := defaultPort
	if port == 0 {
		port = dev.Port
	}
	label := dev.Instance
	if label == "" {
		label = host
	}
	return Target{Host: host, Port: port, DeviceID: dev.ID, Label: label}, nil
}

func normalizeTarget(target Target, defaultPort int) Target {
	if target.Port == 0 {
		target.Port = defaultPort
	}
	if target.Label == "" {
		target.Label = target.Host
	}
	return target
}
