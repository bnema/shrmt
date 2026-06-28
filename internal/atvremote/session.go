package atvremote

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"time"

	pb "github.com/drosocode/atvremote/pkg/v2/proto"
)

const sessionStartGracePeriod = 750 * time.Millisecond

type Session struct {
	params SendKeyParams
	client *remoteClient
}

func DialSession(ctx context.Context, params SendKeyParams) (*Session, error) {
	params = normalizeSessionParams(params)
	if params.Host == "" {
		return nil, errors.New("host is required")
	}

	cert, err := tls.LoadX509KeyPair(params.CertPath, params.KeyPath)
	if err != nil {
		return nil, fmt.Errorf("load cert/key: %w", err)
	}

	dialer := &tls.Dialer{
		Config: &tls.Config{
			Certificates: []tls.Certificate{cert},
			// Android TV Remote uses self-signed mutual TLS certs established during pairing,
			// so default CA-based verification does not apply here.
			InsecureSkipVerify: true,
			ServerName:         inferServerName(params.Host),
		},
	}

	conn, err := dialer.DialContext(ctx, "tcp", endpoint(params.Host, params.Port))
	if err != nil {
		return nil, fmt.Errorf("connect remote endpoint: %w", err)
	}
	tlsConn, ok := conn.(*tls.Conn)
	if !ok {
		_ = conn.Close()
		return nil, errors.New("expected tls connection")
	}

	client := newRemoteClient(tlsConn)
	go client.run()

	if err := client.waitReady(params.ReadyTimeout); err != nil {
		client.close()
		return nil, fmt.Errorf("wait remote ready: %w", err)
	}
	client.waitStarted(sessionStartGracePeriod)
	if err := client.getErr(); err != nil {
		client.close()
		return nil, err
	}

	return &Session{params: params, client: client}, nil
}

func normalizeSessionParams(params SendKeyParams) SendKeyParams {
	if params.Port == 0 {
		params.Port = DefaultRemotePort
	}
	if params.ReadyTimeout == 0 {
		params.ReadyTimeout = 5 * time.Second
	}
	if params.PostDelay == 0 {
		params.PostDelay = 300 * time.Millisecond
	}
	return params
}

func (s *Session) SendKey(ctx context.Context, action string) (*SendKeyResult, error) {
	return s.SendKeyWithDelay(ctx, action, 0)
}

func (s *Session) SendKeyWithDelay(ctx context.Context, action string, postDelay time.Duration) (*SendKeyResult, error) {
	keyCode, err := ResolveKeyCode(action)
	if err != nil {
		return nil, err
	}
	return s.sendKeyCodeWithDelay(ctx, keyCode, pb.RemoteDirection_SHORT, postDelay, normalizeAction(action))
}

func (s *Session) SendKeyCode(ctx context.Context, keyCode pb.RemoteKeyCode, direction pb.RemoteDirection) (*SendKeyResult, error) {
	return s.SendKeyCodeWithDelay(ctx, keyCode, direction, 0)
}

func (s *Session) SendKeyCodeWithDelay(ctx context.Context, keyCode pb.RemoteKeyCode, direction pb.RemoteDirection, postDelay time.Duration) (*SendKeyResult, error) {
	return s.sendKeyCodeWithDelay(ctx, keyCode, direction, postDelay, keyCode.String())
}

func (s *Session) sendKeyCodeWithDelay(ctx context.Context, keyCode pb.RemoteKeyCode, direction pb.RemoteDirection, postDelay time.Duration, resultAction string) (*SendKeyResult, error) {
	if s == nil || s.client == nil {
		return nil, errors.New("session is not connected")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := s.client.getErr(); err != nil {
		return nil, err
	}
	if err := s.client.sendKey(keyCode, direction); err != nil {
		return nil, fmt.Errorf("send key: %w", err)
	}

	if postDelay > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(postDelay):
		}
	}

	if err := s.client.getErr(); err != nil {
		return nil, err
	}
	return s.Snapshot(resultAction), nil
}

func (s *Session) Snapshot(action string) *SendKeyResult {
	if s == nil || s.client == nil {
		return nil
	}
	supported, active := s.client.getFeatures()
	powered, hasPower := s.client.getPowerState()
	return &SendKeyResult{
		Host:              s.params.Host,
		Port:              s.params.Port,
		Action:            action,
		SupportedFeatures: supported,
		ActiveFeatures:    active,
		Powered:           powered,
		HasPowerState:     hasPower,
	}
}

func (s *Session) PowerState() (bool, bool) {
	if s == nil || s.client == nil {
		return false, false
	}
	return s.client.getPowerState()
}

func (s *Session) LaunchAppLink(ctx context.Context, link string) error {
	if s == nil || s.client == nil {
		return errors.New("session is not connected")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if link == "" {
		return errors.New("app link is required")
	}
	if err := s.client.getErr(); err != nil {
		return err
	}
	if err := s.client.sendAppLink(link); err != nil {
		return fmt.Errorf("launch app link: %w", err)
	}
	if err := s.client.getErr(); err != nil {
		return err
	}
	return nil
}

func (s *Session) Close() {
	if s == nil || s.client == nil {
		return
	}
	s.client.close()
	s.client = nil
}
