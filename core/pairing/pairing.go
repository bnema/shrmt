package pairing

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"shrmt/core/device"
)

var (
	ErrCredentialsNotFound = errors.New("credentials not found")
	ErrInvalidCode         = errors.New("invalid pairing code")
	ErrPairingRequired     = errors.New("pairing required")
)

type Code string

type Credentials struct {
	CertPath string
	KeyPath  string
	Source   string
}

type State struct {
	Available   bool
	Credentials Credentials
}

type CodeProvider func() (Code, error)

type PairRequest struct {
	Target       device.Target
	Code         Code
	CodeProvider CodeProvider
	Credentials  Credentials
	ClientName   string
	ServiceName  string
}

type Pairer interface {
	Pair(ctx context.Context, req PairRequest) (Credentials, error)
}

type CredentialStore interface {
	Default(ctx context.Context) (Credentials, error)
	Load(ctx context.Context) (Credentials, error)
	Exists(ctx context.Context, creds Credentials) (bool, error)
}

type Service struct {
	pairer      Pairer
	store       CredentialStore
	clientName  string
	serviceName string
}

func NewService(pairer Pairer, store CredentialStore, clientName, serviceName string) *Service {
	return &Service{pairer: pairer, store: store, clientName: clientName, serviceName: serviceName}
}

func ParseCode(raw string) (Code, error) {
	code := Code(strings.ToUpper(strings.TrimSpace(raw)))
	if len(code) != 6 {
		return "", fmt.Errorf("%w: expected 6 hex characters", ErrInvalidCode)
	}
	for _, r := range code {
		if !(r >= '0' && r <= '9') && !(r >= 'A' && r <= 'F') {
			return "", fmt.Errorf("%w: %q", ErrInvalidCode, raw)
		}
	}
	return code, nil
}

func (c Code) String() string {
	return string(c)
}

func (c Credentials) IsZero() bool {
	return c.CertPath == "" || c.KeyPath == ""
}

func (s *Service) State(ctx context.Context) (State, error) {
	creds, err := s.Credentials(ctx)
	if err != nil {
		if errors.Is(err, ErrCredentialsNotFound) {
			return State{}, nil
		}
		return State{}, err
	}
	return State{Available: true, Credentials: creds}, nil
}

func (s *Service) Credentials(ctx context.Context) (Credentials, error) {
	if s.store == nil {
		return Credentials{}, ErrCredentialsNotFound
	}
	creds, err := s.store.Load(ctx)
	if err != nil {
		return Credentials{}, err
	}
	if creds.IsZero() {
		return Credentials{}, ErrCredentialsNotFound
	}
	exists, err := s.store.Exists(ctx, creds)
	if err != nil {
		return Credentials{}, err
	}
	if !exists {
		return Credentials{}, ErrCredentialsNotFound
	}
	return creds, nil
}

func (s *Service) Pair(ctx context.Context, target device.Target, code Code, provider CodeProvider) (State, error) {
	if target.IsZero() {
		return State{}, device.ErrNoDevices
	}
	if code == "" && provider == nil {
		return State{}, ErrInvalidCode
	}
	if code != "" {
		validated, err := ParseCode(code.String())
		if err != nil {
			return State{}, err
		}
		code = validated
	}
	if s.store == nil || s.pairer == nil {
		return State{}, errors.New("pairing not configured")
	}
	creds, err := s.store.Default(ctx)
	if err != nil {
		return State{}, err
	}
	wrappedProvider := provider
	if provider != nil {
		wrappedProvider = func() (Code, error) {
			provided, err := provider()
			if err != nil {
				return "", err
			}
			return ParseCode(provided.String())
		}
	}
	creds, err = s.pairer.Pair(ctx, PairRequest{
		Target:       target,
		Code:         code,
		CodeProvider: wrappedProvider,
		Credentials:  creds,
		ClientName:   s.clientName,
		ServiceName:  s.serviceName,
	})
	if err != nil {
		return State{}, err
	}
	return State{Available: true, Credentials: creds}, nil
}
