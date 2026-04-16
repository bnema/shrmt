package xdg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"shrmt/core/device"
	"shrmt/core/pairing"
)

const (
	appDirName = "shrmt"
	targetFile = "target.json"
	certFile   = "androidtv-client-cert.pem"
	keyFile    = "androidtv-client-key.pem"
)

type CredentialStore struct{}

type TargetStore struct{}

func NewCredentialStore() *CredentialStore { return &CredentialStore{} }
func NewTargetStore() *TargetStore         { return &TargetStore{} }

func (s *CredentialStore) Default(context.Context) (pairing.Credentials, error) {
	base, err := configPath(appDirName)
	if err != nil {
		return pairing.Credentials{}, err
	}
	return pairing.Credentials{
		CertPath: filepath.Join(base, certFile),
		KeyPath:  filepath.Join(base, keyFile),
		Source:   appDirName,
	}, nil
}

func (s *CredentialStore) Load(ctx context.Context) (pairing.Credentials, error) {
	primary, err := s.Default(ctx)
	if err != nil {
		return pairing.Credentials{}, err
	}
	if ok, err := s.Exists(ctx, primary); err != nil {
		return pairing.Credentials{}, err
	} else if ok {
		return primary, nil
	}

	return pairing.Credentials{}, pairing.ErrCredentialsNotFound
}

func (s *CredentialStore) Exists(_ context.Context, creds pairing.Credentials) (bool, error) {
	certOK, err := fileExists(creds.CertPath)
	if err != nil {
		return false, err
	}
	keyOK, err := fileExists(creds.KeyPath)
	if err != nil {
		return false, err
	}
	if certOK != keyOK {
		return false, fmt.Errorf("credential files out of sync: cert=%s key=%s", creds.CertPath, creds.KeyPath)
	}
	return certOK && keyOK, nil
}

func (s *TargetStore) Load(context.Context) (device.Target, error) {
	path, err := targetPath()
	if err != nil {
		return device.Target{}, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return device.Target{}, device.ErrNoSavedTarget
		}
		return device.Target{}, fmt.Errorf("read target: %w", err)
	}
	var target device.Target
	if err := json.Unmarshal(raw, &target); err != nil {
		return device.Target{}, fmt.Errorf("decode target: %w", err)
	}
	if target.IsZero() {
		return device.Target{}, device.ErrNoSavedTarget
	}
	return target, nil
}

func (s *TargetStore) Save(_ context.Context, target device.Target) error {
	path, err := targetPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	raw, err := json.MarshalIndent(target, "", "  ")
	if err != nil {
		return fmt.Errorf("encode target: %w", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return fmt.Errorf("write target: %w", err)
	}
	return nil
}

func (s *TargetStore) Clear(context.Context) error {
	path, err := targetPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("clear target: %w", err)
	}
	return nil
}

func targetPath() (string, error) {
	base, err := configPath(appDirName)
	if err != nil {
		return "", err
	}
	return filepath.Join(base, targetFile), nil
}

func configPath(appName string) (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return filepath.Join(configDir, appName), nil
}

func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}
