package beyonder

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"

	pb "shrmt/internal/beyonder/proto"
)

const (
	// CapRTTMessaging is the RTT_MESSAGING capability bit.
	CapRTTMessaging int32 = 1024
	// LevelVirtualInput is LEVEL_VIRTUAL_INPUT.
	LevelVirtualInput = int32(pb.AuthorizationLevel_LEVEL_VIRTUAL_INPUT)
)

// ErrNotPaired is returned when input is attempted without input authorization.
var ErrNotPaired = errors.New("beyonder: not paired (authorization level is not VIRTUAL_INPUT)")

// AuthLevel is the server's reply to hello.
type AuthLevel struct {
	Level      int32
	DeviceName string
	Capability int32
}

// wait returns the first message accepted by pred.
func (c *Client) wait(ctx context.Context, pred func(Message) bool) (Message, error) {
	for {
		select {
		case <-ctx.Done():
			return Message{}, ctx.Err()
		case m, ok := <-c.msgs:
			if !ok {
				if err := c.Err(); err != nil {
					return Message{}, fmt.Errorf("beyonder: connection closed: %w", err)
				}
				return Message{}, errors.New("beyonder: connection closed")
			}
			if pred(m) {
				return m, nil
			}
		}
	}
}

// Hello sends the handshake and waits for AUTHORIZATION_LEVEL. The RTT
// keepalive is started (bound to ctx) if the server advertises RTT_MESSAGING;
// pass a long-lived ctx or use StartKeepalive yourself with a derived one.
func (c *Client) Hello(ctx context.Context, name string) (AuthLevel, error) {
	h, err := EncodeHello(name)
	if err != nil {
		return AuthLevel{}, err
	}
	if err := c.Send(ctx, h); err != nil {
		return AuthLevel{}, err
	}
	m, err := c.wait(ctx, func(m Message) bool { return m.AuthLevel != nil })
	if err != nil {
		return AuthLevel{}, fmt.Errorf("beyonder: waiting authorization level: %w", err)
	}
	al := AuthLevel{
		Level:      int32(m.AuthLevel.Level),
		DeviceName: m.AuthLevel.DeviceName,
		Capability: m.AuthLevel.Capability,
	}
	c.mu.Lock()
	c.level, c.caps, c.name = al.Level, al.Capability, al.DeviceName
	c.mu.Unlock()
	if al.Capability&CapRTTMessaging != 0 {
		c.touchRTT()
		c.StartKeepalive(ctx)
	}
	return al, nil
}

// Pair runs the PIN pairing flow and returns the client certificate issued
// by the SHIELD.
func (c *Client) Pair(ctx context.Context, pinPrompt func() (string, error), deviceID string) (tls.Certificate, error) {
	req, err := EncodePinRequest()
	if err != nil {
		return tls.Certificate{}, err
	}
	if err := c.Send(ctx, req); err != nil {
		return tls.Certificate{}, err
	}
	if _, err := c.wait(ctx, func(m Message) bool { return m.PinChallenge }); err != nil {
		return tls.Certificate{}, fmt.Errorf("beyonder: waiting PIN challenge: %w", err)
	}
	pin, err := pinPrompt()
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("beyonder: PIN prompt: %w", err)
	}
	code, err := EncodePinCode(pin, deviceID)
	if err != nil {
		return tls.Certificate{}, err
	}
	if err := c.Send(ctx, code); err != nil {
		return tls.Certificate{}, err
	}
	m, err := c.wait(ctx, func(m Message) bool { return m.PairingResult != nil })
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("beyonder: waiting pairing result: %w", err)
	}
	pr := m.PairingResult
	if pr.ResultType != pb.PairingResult_RESULT_SUCCESS {
		return tls.Certificate{}, fmt.Errorf("beyonder: pairing failed: %v (%q)", pr.ResultType, pr.ResultString)
	}
	if len(pr.PrivateKey) == 0 || len(pr.Certificate) == 0 {
		return tls.Certificate{}, errors.New("beyonder: pairing succeeded but key or certificate is missing")
	}
	return ParseCredentials(pr.Certificate, pr.PrivateKey)
}

// SendSpecialInput sends a SPECIAL input; requires VIRTUAL_INPUT level.
func (c *Client) SendSpecialInput(ctx context.Context, v pb.VirtualInputPayload_SpecialInput) error {
	c.mu.Lock()
	level := c.level
	c.mu.Unlock()
	if level != LevelVirtualInput {
		return ErrNotPaired
	}
	f, err := EncodeSpecialInput(v)
	if err != nil {
		return err
	}
	return c.Send(ctx, f)
}

// LaunchApp launches an Android package; requires VIRTUAL_INPUT level.
func (c *Client) LaunchApp(ctx context.Context, pkg string) error {
	c.mu.Lock()
	level := c.level
	c.mu.Unlock()
	if level != LevelVirtualInput {
		return ErrNotPaired
	}
	frame, err := EncodeLaunch(pkg)
	if err != nil {
		return err
	}
	if err := c.Send(ctx, frame); err != nil {
		return err
	}
	m, err := c.wait(ctx, func(m Message) bool { return m.LaunchResult != nil })
	if err != nil {
		return fmt.Errorf("beyonder: waiting launch response: %w", err)
	}
	switch m.LaunchResult.Result {
	case pb.LaunchResponse_SUCCESS:
		return nil
	case pb.LaunchResponse_ACTIVITY_NOT_FOUND:
		return fmt.Errorf("beyonder: %s is not installed", pkg)
	default:
		return fmt.Errorf("beyonder: launch %s failed: %v", pkg, m.LaunchResult.Result)
	}
}

// ParseCredentials builds a tls.Certificate from a certificate (X.509 DER or
// PEM) and a private key (PKCS#8, or PKCS#1/EC as fallback; DER or PEM).
func ParseCredentials(cert, key []byte) (tls.Certificate, error) {
	certDER := cert
	if b, _ := pem.Decode(cert); b != nil {
		certDER = b.Bytes
	}
	leaf, err := x509.ParseCertificate(certDER)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("beyonder: parse certificate: %w", err)
	}
	keyDER := key
	if b, _ := pem.Decode(key); b != nil {
		keyDER = b.Bytes
	}
	var priv any
	if priv, err = x509.ParsePKCS8PrivateKey(keyDER); err != nil {
		if k, e := x509.ParsePKCS1PrivateKey(keyDER); e == nil {
			priv, err = k, nil
		} else if k, e := x509.ParseECPrivateKey(keyDER); e == nil {
			priv, err = k, nil
		}
	}
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("beyonder: parse private key: %w", err)
	}
	tc := tls.Certificate{Certificate: [][]byte{certDER}, PrivateKey: priv, Leaf: leaf}
	// Validate the pair.
	if _, err := tls.X509KeyPair(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}),
		mustKeyPEM(priv)); err != nil {
		return tls.Certificate{}, fmt.Errorf("beyonder: key/certificate mismatch: %w", err)
	}
	return tc, nil
}

func mustKeyPEM(priv any) []byte {
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
}
