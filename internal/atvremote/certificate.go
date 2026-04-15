package atvremote

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

func EnsureClientCertificate(certPath, keyPath, clientName string) error {
	certExists, err := fileExists(certPath)
	if err != nil {
		return fmt.Errorf("check cert path: %w", err)
	}
	keyExists, err := fileExists(keyPath)
	if err != nil {
		return fmt.Errorf("check key path: %w", err)
	}

	if certExists && keyExists {
		return nil
	}
	if certExists != keyExists {
		return fmt.Errorf("only one of cert/key exists; fix or remove files: cert=%s key=%s", certPath, keyPath)
	}

	certPEM, keyPEM, err := generateSelfSignedCertificate(clientName)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(certPath), 0o755); err != nil {
		return fmt.Errorf("create cert directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o755); err != nil {
		return fmt.Errorf("create key directory: %w", err)
	}

	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		return fmt.Errorf("write cert file: %w", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		return fmt.Errorf("write key file: %w", err)
	}

	return nil
}

func generateSelfSignedCertificate(clientName string) ([]byte, []byte, error) {
	if clientName == "" {
		clientName = "shrmt"
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, fmt.Errorf("generate rsa key: %w", err)
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, nil, fmt.Errorf("generate serial number: %w", err)
	}

	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: clientName,
		},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{clientName},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("create certificate: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})
	if certPEM == nil || keyPEM == nil {
		return nil, nil, errors.New("encode pem")
	}

	return certPEM, keyPEM, nil
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
