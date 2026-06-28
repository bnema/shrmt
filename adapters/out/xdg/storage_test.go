package xdg

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"shrmt/core/device"
)

func TestLoadMigratesLegacyShremoteConfig(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	legacyDir := filepath.Join(configHome, "shremote")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatalf("MkdirAll legacyDir: %v", err)
	}
	writeTestFile(t, filepath.Join(legacyDir, certFile), []byte("cert"), 0o644)
	writeTestFile(t, filepath.Join(legacyDir, keyFile), []byte("key"), 0o600)
	writeTestFile(t, filepath.Join(legacyDir, targetFile), []byte(`{"Host":"192.168.1.16","Port":6466,"Label":"SHIELD"}`), 0o600)

	ctx := context.Background()

	creds, err := NewCredentialStore().Load(ctx)
	if err != nil {
		t.Fatalf("CredentialStore.Load() error = %v", err)
	}
	if creds.Source != appDirName {
		t.Fatalf("CredentialStore.Load() source = %q, want %q", creds.Source, appDirName)
	}
	if _, err := os.Stat(filepath.Join(configHome, appDirName, certFile)); err != nil {
		t.Fatalf("migrated cert missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(configHome, appDirName, keyFile)); err != nil {
		t.Fatalf("migrated key missing: %v", err)
	}

	target, err := NewTargetStore().Load(ctx)
	if err != nil {
		t.Fatalf("TargetStore.Load() error = %v", err)
	}
	want := device.Target{Host: "192.168.1.16", Port: 6466, Label: "SHIELD"}
	if target != want {
		t.Fatalf("TargetStore.Load() = %+v, want %+v", target, want)
	}
	if _, err := os.Stat(filepath.Join(configHome, appDirName, targetFile)); err != nil {
		t.Fatalf("migrated target missing: %v", err)
	}
}

func TestPrimaryConfigWinsOverLegacy(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	primaryDir := filepath.Join(configHome, appDirName)
	legacyDir := filepath.Join(configHome, "shremote")
	if err := os.MkdirAll(primaryDir, 0o755); err != nil {
		t.Fatalf("MkdirAll primaryDir: %v", err)
	}
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatalf("MkdirAll legacyDir: %v", err)
	}

	writeTestFile(t, filepath.Join(primaryDir, certFile), []byte("primary-cert"), 0o644)
	writeTestFile(t, filepath.Join(primaryDir, keyFile), []byte("primary-key"), 0o600)
	writeTestFile(t, filepath.Join(primaryDir, targetFile), []byte(`{"Host":"10.0.0.1","Port":6466,"Label":"primary"}`), 0o600)
	writeTestFile(t, filepath.Join(legacyDir, certFile), []byte("legacy-cert"), 0o644)
	writeTestFile(t, filepath.Join(legacyDir, keyFile), []byte("legacy-key"), 0o600)
	writeTestFile(t, filepath.Join(legacyDir, targetFile), []byte(`{"Host":"10.0.0.2","Port":6466,"Label":"legacy"}`), 0o600)

	ctx := context.Background()

	creds, err := NewCredentialStore().Load(ctx)
	if err != nil {
		t.Fatalf("CredentialStore.Load() error = %v", err)
	}
	if got := filepath.Dir(creds.CertPath); got != primaryDir {
		t.Fatalf("CredentialStore.Load() dir = %q, want %q", got, primaryDir)
	}

	target, err := NewTargetStore().Load(ctx)
	if err != nil {
		t.Fatalf("TargetStore.Load() error = %v", err)
	}
	if target.Host != "10.0.0.1" {
		t.Fatalf("TargetStore.Load() host = %q, want %q", target.Host, "10.0.0.1")
	}
}

func writeTestFile(t *testing.T, path string, data []byte, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", path, err)
	}
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}
