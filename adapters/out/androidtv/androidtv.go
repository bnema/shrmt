package androidtv

import (
	"context"
	"fmt"
	"sync"
	"time"

	"shrmt/core/action"
	"shrmt/core/device"
	"shrmt/core/pairing"
	"shrmt/core/remote"
	intatv "shrmt/internal/atvremote"
)

const (
	DefaultRemotePort  = intatv.DefaultRemotePort
	DefaultPairingPort = intatv.DefaultPairingPort
	DefaultServiceName = intatv.DefaultServiceName
	wakeKeyDelay       = 750 * time.Millisecond
	wakeLaunchPackage  = "com.google.android.youtube.tv"
)

type Sender struct {
	mu      sync.Mutex
	session *intatv.Session
	config  sessionConfig
}

type sessionConfig struct {
	host     string
	port     int
	certPath string
	keyPath  string
}

type Pairer struct {
	PairingPort int
}

type sendStep struct {
	key       string
	appLink   string
	postDelay time.Duration
}

func NewSender() *Sender {
	return &Sender{}
}

func NewPairer() *Pairer {
	return &Pairer{PairingPort: DefaultPairingPort}
}

func (s *Sender) Send(ctx context.Context, target device.Target, creds pairing.Credentials, act action.Action) (remote.SendResult, error) {
	result, err := s.sendWithPersistentSession(ctx, target, creds, act)
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

func (s *Sender) Warmup(ctx context.Context, target device.Target, creds pairing.Credentials) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.ensureSessionLocked(ctx, target, creds)
	return err
}

func (s *Sender) Launch(ctx context.Context, target device.Target, creds pairing.Credentials, link string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, err := s.ensureSessionLocked(ctx, target, creds)
	if err != nil {
		return err
	}

	err = session.LaunchAppLink(ctx, link)
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return err
	}

	s.closeSessionLocked()
	session, reconnectErr := s.ensureSessionLocked(ctx, target, creds)
	if reconnectErr != nil {
		return reconnectErr
	}
	return session.LaunchAppLink(ctx, link)
}

func (p *Pairer) Pair(ctx context.Context, req pairing.PairRequest) (pairing.Credentials, error) {
	port := p.PairingPort
	if port == 0 {
		port = DefaultPairingPort
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

func (s *Sender) sendWithPersistentSession(ctx context.Context, target device.Target, creds pairing.Credentials, act action.Action) (*intatv.SendKeyResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, err := s.ensureSessionLocked(ctx, target, creds)
	if err != nil {
		return nil, err
	}

	result, err := sendAction(ctx, session, act)
	if err == nil {
		return result, nil
	}
	if ctx.Err() != nil {
		return nil, err
	}

	s.closeSessionLocked()
	session, reconnectErr := s.ensureSessionLocked(ctx, target, creds)
	if reconnectErr != nil {
		return nil, reconnectErr
	}
	return sendAction(ctx, session, act)
}

func sendAction(ctx context.Context, session *intatv.Session, act action.Action) (*intatv.SendKeyResult, error) {
	powered, hasPower := session.PowerState()
	steps, err := planSendSteps(act, powered, hasPower)
	if err != nil {
		return nil, err
	}

	var result *intatv.SendKeyResult
	for _, step := range steps {
		if step.appLink != "" {
			if err := session.LaunchAppLink(ctx, step.appLink); err != nil {
				return nil, err
			}
			if step.postDelay > 0 {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(step.postDelay):
				}
			}
			result = session.Snapshot("launch:" + step.appLink)
			continue
		}
		result, err = session.SendKeyWithDelay(ctx, step.key, step.postDelay)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func planSendSteps(act action.Action, powered bool, hasPower bool) ([]sendStep, error) {
	mapped, err := actionToAndroidTV(act)
	if err != nil {
		return nil, err
	}
	if !hasPower {
		return []sendStep{{key: mapped}}, nil
	}
	if !powered {
		switch act {
		case action.Home, action.Power:
			return []sendStep{{appLink: wakeLaunchPackage, postDelay: wakeKeyDelay}, {key: action.Home.String()}}, nil
		}
	}
	return []sendStep{{key: mapped}}, nil
}

func (s *Sender) ensureSessionLocked(ctx context.Context, target device.Target, creds pairing.Credentials) (*intatv.Session, error) {
	cfg := sessionConfig{
		host:     target.Host,
		port:     target.Port,
		certPath: creds.CertPath,
		keyPath:  creds.KeyPath,
	}
	if s.session != nil && s.config == cfg {
		return s.session, nil
	}

	s.closeSessionLocked()
	session, err := intatv.DialSession(ctx, intatv.SendKeyParams{
		Host:     target.Host,
		Port:     target.Port,
		CertPath: creds.CertPath,
		KeyPath:  creds.KeyPath,
	})
	if err != nil {
		return nil, err
	}
	s.session = session
	s.config = cfg
	return s.session, nil
}

func (s *Sender) closeSessionLocked() {
	if s.session != nil {
		s.session.Close()
		s.session = nil
	}
	s.config = sessionConfig{}
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
		action.VolumeUp,
		action.Wakeup:
		return act.String(), nil
	default:
		return "", fmt.Errorf("unsupported android tv action %q", act)
	}
}
