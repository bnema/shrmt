package remote

import (
	"context"
	"errors"

	"shrmt/core/action"
	"shrmt/core/device"
	"shrmt/core/pairing"
)

var ErrRemoteNotReady = errors.New("remote not ready")

type SendInput struct {
	Target *device.Target
	Action action.Action
}

type State struct {
	Target      device.Target
	HasTarget   bool
	SavedTarget bool
	Pairing     pairing.State
}

type SendResult struct {
	Action            action.Action
	SupportedFeatures int32
	ActiveFeatures    int32
	Powered           *bool
}

type Sender interface {
	Send(ctx context.Context, target device.Target, creds pairing.Credentials, act action.Action) (SendResult, error)
}

type Warmupper interface {
	Warmup(ctx context.Context, target device.Target, creds pairing.Credentials) error
}

type Service struct {
	devices *device.Service
	pairing *pairing.Service
	sender  Sender
}

func NewService(devices *device.Service, pairingSvc *pairing.Service, sender Sender) *Service {
	return &Service{devices: devices, pairing: pairingSvc, sender: sender}
}

func (s *Service) Load(ctx context.Context, explicit *device.Target) (State, error) {
	state := State{}
	if s.pairing != nil {
		pairingState, err := s.pairing.State(ctx)
		if err != nil {
			return State{}, err
		}
		state.Pairing = pairingState
	}
	if s.devices == nil {
		return state, nil
	}
	if explicit == nil {
		if _, err := s.devices.LoadDefault(ctx); err == nil {
			state.SavedTarget = true
		} else if !errors.Is(err, device.ErrNoSavedTarget) {
			return State{}, err
		}
	}
	target, err := s.devices.Resolve(ctx, explicit)
	if err != nil {
		return state, err
	}
	state.Target = target
	state.HasTarget = !target.IsZero()
	if state.HasTarget && state.Pairing.Available {
		if warmupper, ok := s.sender.(Warmupper); ok {
			_ = warmupper.Warmup(ctx, target, state.Pairing.Credentials)
		}
	}
	return state, nil
}

func (s *Service) Send(ctx context.Context, input SendInput) (SendResult, error) {
	if s.sender == nil || s.devices == nil || s.pairing == nil {
		return SendResult{}, ErrRemoteNotReady
	}
	target, err := s.devices.Resolve(ctx, input.Target)
	if err != nil {
		return SendResult{}, err
	}
	creds, err := s.pairing.Credentials(ctx)
	if err != nil {
		if errors.Is(err, pairing.ErrCredentialsNotFound) {
			return SendResult{}, pairing.ErrPairingRequired
		}
		return SendResult{}, err
	}
	return s.sender.Send(ctx, target, creds, input.Action)
}
