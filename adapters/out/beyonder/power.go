// Package beyonder routes power and app shortcuts through NVIDIA's beyonder
// protocol (TLS :8987). Other actions go to the wrapped sender.
package beyonder

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"shrmt/core/action"
	"shrmt/core/device"
	"shrmt/core/pairing"
	"shrmt/core/remote"
	intbey "shrmt/internal/beyonder"
	pb "shrmt/internal/beyonder/proto"
)

const (
	Port       = 8987
	helloName  = "shrmt"
	stepBudget = 5 * time.Second
	// flushDelay keeps the connection open briefly so the SHIELD reads the
	// frame before the TLS close.
	flushDelay = 300 * time.Millisecond
)

// Inner is the fallback sender for actions not routed through beyonder.
type Inner interface {
	remote.Sender
	remote.Warmupper
	remote.Launcher
}

// Sender sends power and app shortcuts over beyonder when credentials are stored,
// and falls back to Inner otherwise.
type Sender struct {
	inner Inner
}

func NewSender(inner Inner) *Sender {
	return &Sender{inner: inner}
}

func (s *Sender) Send(ctx context.Context, target device.Target, creds pairing.Credentials, act action.Action) (remote.SendResult, error) {
	if act != action.Power {
		return s.inner.Send(ctx, target, creds, act)
	}
	cert, err := intbey.LoadCredentials()
	if err != nil {
		return remote.SendResult{}, fmt.Errorf("load beyonder credentials: %w", err)
	}
	if cert == nil {
		return s.inner.Send(ctx, target, creds, act)
	}
	ctx, cancel := context.WithTimeout(ctx, stepBudget)
	defer cancel()
	c, err := connect(ctx, target, cert)
	if err != nil {
		return remote.SendResult{}, err
	}
	defer c.Close()
	if err := c.SendSpecialInput(ctx, pb.VirtualInputPayload_ACTION_SLEEP_NOW); err != nil {
		if errors.Is(err, intbey.ErrNotPaired) {
			return remote.SendResult{}, fmt.Errorf("%w: run beyonderprobe pair", err)
		}
		return remote.SendResult{}, err
	}
	select {
	case <-ctx.Done():
	case <-time.After(flushDelay):
	}
	return remote.SendResult{Action: act}, nil
}

func (s *Sender) Warmup(ctx context.Context, target device.Target, creds pairing.Credentials) error {
	return s.inner.Warmup(ctx, target, creds)
}

func (s *Sender) Launch(ctx context.Context, target device.Target, creds pairing.Credentials, link string) error {
	if strings.Contains(link, "://") {
		return s.inner.Launch(ctx, target, creds, link)
	}
	cert, err := intbey.LoadCredentials()
	if err != nil {
		return fmt.Errorf("load beyonder credentials: %w", err)
	}
	if cert == nil {
		return s.inner.Launch(ctx, target, creds, link)
	}
	ctx, cancel := context.WithTimeout(ctx, stepBudget)
	defer cancel()
	c, err := connect(ctx, target, cert)
	if err != nil {
		return err
	}
	defer c.Close()
	if err := c.LaunchApp(ctx, link); err != nil {
		if errors.Is(err, intbey.ErrNotPaired) {
			return fmt.Errorf("%w: run beyonderprobe pair", err)
		}
		return err
	}
	return nil
}

func connect(ctx context.Context, target device.Target, cert *tls.Certificate) (*intbey.Client, error) {
	c, err := intbey.Dial(ctx, net.JoinHostPort(target.Host, strconv.Itoa(Port)), cert)
	if err != nil {
		return nil, err
	}
	if _, err := c.Hello(ctx, helloName); err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
}
