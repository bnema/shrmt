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

var legacyConfigDirNames = []string{"shremote", "shield-poc"}

type CredentialStore struct{}

type TargetStore struct{}

func NewCredentialStore() *CredentialStore { return &CredentialStore{} }
func NewTargetStore() *TargetStore         { return &TargetStore{} }

func (s *CredentialStore) Default(context.Context) (pairing.Credentials, error) {
	return credentialsForApp(appDirName)
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

	for _, legacyDirName := range legacyConfigDirNames {
		legacy, err := credentialsForApp(legacyDirName)
		if err != nil {
			return pairing.Credentials{}, err
		}
		if ok, err := s.Exists(ctx, legacy); err != nil {
			return pairing.Credentials{}, err
		} else if ok {
			if err := migrateCredentials(legacy, primary); err == nil {
				return primary, nil
			}
			return legacy, nil
		}
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

func (s *TargetStore) Load(ctx context.Context) (device.Target, error) {
	paths, err := targetPaths()
	if err != nil {
		return device.Target{}, err
	}
	for idx, path := range paths {
		target, err := loadTarget(path)
		if err != nil {
			if errors.Is(err, device.ErrNoSavedTarget) {
				continue
			}
			return device.Target{}, err
		}
		if idx > 0 {
			_ = s.Save(ctx, target)
		}
		return target, nil
	}
	return device.Target{}, device.ErrNoSavedTarget
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

func credentialsForApp(appName string) (pairing.Credentials, error) {
	base, err := configPath(appName)
	if err != nil {
		return pairing.Credentials{}, err
	}
	return pairing.Credentials{
		CertPath: filepath.Join(base, certFile),
		KeyPath:  filepath.Join(base, keyFile),
		Source:   appName,
	}, nil
}

func migrateCredentials(src, dst pairing.Credentials) error {
	if err := os.MkdirAll(filepath.Dir(dst.CertPath), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	if err := copyFile(src.CertPath, dst.CertPath); err != nil {
		return fmt.Errorf("copy cert: %w", err)
	}
	if err := copyFile(src.KeyPath, dst.KeyPath); err != nil {
		return fmt.Errorf("copy key: %w", err)
	}
	return nil
}

func loadTarget(path string) (device.Target, error) {
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

func targetPath() (string, error) {
	base, err := configPath(appDirName)
	if err != nil {
		return "", err
	}
	return filepath.Join(base, targetFile), nil
}

func targetPaths() ([]string, error) {
	paths := make([]string, 0, 1+len(legacyConfigDirNames))
	primary, err := targetPath()
	if err != nil {
		return nil, err
	}
	paths = append(paths, primary)
	for _, legacyDirName := range legacyConfigDirNames {
		base, err := configPath(legacyDirName)
		if err != nil {
			return nil, err
		}
		paths = append(paths, filepath.Join(base, targetFile))
	}
	return paths, nil
}

func configPath(appName string) (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return filepath.Join(configDir, appName), nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, info.Mode().Perm())
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
