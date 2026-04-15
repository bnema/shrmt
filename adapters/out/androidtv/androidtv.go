package androidtv

import (
	"context"
	"fmt"
	"time"

	"shrmt/core/action"
	"shrmt/core/device"
	"shrmt/core/pairing"
	"shrmt/core/remote"
	intatv "shrmt/internal/atvremote"
)

type Sender struct{}

type Pairer struct {
	PairingPort int
}

func NewSender() *Sender {
	return &Sender{}
}

func NewPairer() *Pairer {
	return &Pairer{PairingPort: intatv.DefaultPairingPort}
}

func (s *Sender) Send(ctx context.Context, target device.Target, creds pairing.Credentials, act action.Action) (remote.SendResult, error) {
	mapped, err := actionToAndroidTV(act)
	if err != nil {
		return remote.SendResult{}, err
	}
	result, err := intatv.SendKey(ctx, intatv.SendKeyParams{
		Host:      target.Host,
		Port:      target.Port,
		CertPath:  creds.CertPath,
		KeyPath:   creds.KeyPath,
		PostDelay: 350 * time.Millisecond,
	}, mapped)
	if err != nil {
		return remote.SendResult{}, err
	}
	var powered *bool
	if result.HasPowerState {
		v := result.Powered
		powered = &v
	}
	return remote.SendResult{
		Action:            act,
		SupportedFeatures: result.SupportedFeatures,
		ActiveFeatures:    result.ActiveFeatures,
		Powered:           powered,
	}, nil
}

func (p *Pairer) Pair(ctx context.Context, req pairing.PairRequest) (pairing.Credentials, error) {
	port := p.PairingPort
	if port == 0 {
		port = intatv.DefaultPairingPort
	}
	params := intatv.PairParams{
		Host:        req.Target.Host,
		Port:        port,
		ClientName:  req.ClientName,
		ServiceName: req.ServiceName,
		PairingCode: req.Code.String(),
		CertPath:    req.Credentials.CertPath,
		KeyPath:     req.Credentials.KeyPath,
	}
	if req.CodeProvider != nil {
		params.CodeProvider = func() (string, error) {
			code, err := req.CodeProvider()
			if err != nil {
				return "", err
			}
			return code.String(), nil
		}
	}
	result, err := intatv.Pair(ctx, params)
	if err != nil {
		return pairing.Credentials{}, err
	}
	return pairing.Credentials{
		CertPath: result.CertPath,
		KeyPath:  result.KeyPath,
		Source:   "shrmt",
	}, nil
}

func actionToAndroidTV(act action.Action) (string, error) {
	switch act {
	case action.Back,
		action.Down,
		action.Enter,
		action.Home,
		action.Left,
		action.Mute,
		action.PlayPause,
		action.Power,
		action.Right,
		action.Sleep,
		action.Up,
		action.VolumeDown,
		action.VolumeUp:
		return act.String(), nil
	default:
		return "", fmt.Errorf("unsupported android tv action %q", act)
	}
}
