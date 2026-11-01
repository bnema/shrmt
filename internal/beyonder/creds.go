package beyonder

import (
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	certFile     = "beyonder-client-cert.pem"
	keyFile      = "beyonder-client-key.pem"
	deviceIDFile = "beyonder-device-id"
)

// configDir mirrors adapters/out/xdg: os.UserConfigDir()/shrmt.
func configDir() (string, error) {
	d, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return filepath.Join(d, "shrmt"), nil
}

func writePrivate(dir, name string, data []byte) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, data, 0o600); err != nil {
		return err
	}
	return os.Chmod(p, 0o600)
}

// LoadCredentials returns the stored client certificate, or (nil, nil) if absent.
func LoadCredentials() (*tls.Certificate, error) {
	dir, err := configDir()
	if err != nil {
		return nil, err
	}
	certPEM, err := os.ReadFile(filepath.Join(dir, certFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	keyPEM, err := os.ReadFile(filepath.Join(dir, keyFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("beyonder: load stored credentials: %w", err)
	}
	return &c, nil
}

// SaveCredentials stores the certificate and key (each DER or PEM; normalized
// to PEM) with mode 0600.
func SaveCredentials(cert, key []byte) error {
	c, err := ParseCredentials(cert, key)
	if err != nil {
		return err
	}
	return SaveCertificate(c)
}

// SaveCertificate stores an issued tls.Certificate as PEM files.
func SaveCertificate(c tls.Certificate) error {
	if len(c.Certificate) == 0 || c.PrivateKey == nil {
		return errors.New("beyonder: empty certificate")
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(c.PrivateKey)
	if err != nil {
		return fmt.Errorf("beyonder: marshal key: %w", err)
	}
	dir, err := configDir()
	if err != nil {
		return err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: c.Certificate[0]})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	if err := writePrivate(dir, keyFile, keyPEM); err != nil {
		return err
	}
	return writePrivate(dir, certFile, certPEM)
}

// LoadOrCreateDeviceID returns the persistent random device id (32 hex chars).
func LoadOrCreateDeviceID() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	p := filepath.Join(dir, deviceIDFile)
	if b, err := os.ReadFile(p); err == nil {
		if id := strings.TrimSpace(string(b)); id != "" {
			return id, nil
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	id := hex.EncodeToString(raw)
	if err := writePrivate(dir, deviceIDFile, []byte(id+"\n")); err != nil {
		return "", err
	}
	return id, nil
}
