package atvremote

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizePairingCode(t *testing.T) {
	t.Parallel()

	code, err := NormalizePairingCode(" a1b2c3\n")
	if err != nil {
		t.Fatalf("NormalizePairingCode returned error: %v", err)
	}
	if got, want := code, "A1B2C3"; got != want {
		t.Fatalf("NormalizePairingCode = %q, want %q", got, want)
	}
}

func TestEnsureClientCertificate(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	certPath := filepath.Join(dir, "client-cert.pem")
	keyPath := filepath.Join(dir, "client-key.pem")

	if err := EnsureClientCertificate(certPath, keyPath, "shrmt-test"); err != nil {
		t.Fatalf("EnsureClientCertificate returned error: %v", err)
	}

	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatalf("ReadFile(cert) returned error: %v", err)
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("ReadFile(key) returned error: %v", err)
	}

	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		t.Fatal("failed to decode certificate PEM")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		t.Fatalf("ParseCertificate returned error: %v", err)
	}
	if cert.Subject.CommonName != "shrmt-test" {
		t.Fatalf("certificate common name = %q, want %q", cert.Subject.CommonName, "shrmt-test")
	}

	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		t.Fatal("failed to decode key PEM")
	}
	key, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	if err != nil {
		t.Fatalf("ParsePKCS1PrivateKey returned error: %v", err)
	}
	if _, ok := any(key).(*rsa.PrivateKey); !ok {
		t.Fatal("expected RSA private key")
	}
}
