package beyonder

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

const (
	rttTick     = 3 * time.Second
	rttIdle     = 5 * time.Second
	readBufSize = 65536
)

// Client is a TLS connection to a SHIELD beyonder server.
type Client struct {
	// OnFrame, if set, is called for every chunk
	// sent ("tx") or received ("rx"). Set it before the first Send; it must be
	// safe for concurrent use.
	OnFrame func(dir string, b []byte)

	conn net.Conn
	msgs chan Message
	done chan struct{}

	wmu sync.Mutex

	mu      sync.Mutex
	err     error
	lastRTT time.Time
	level   int32
	caps    int32
	name    string

	closeOnce sync.Once
}

// Dial connects to addr (host:port) over TLS. cert may be nil (unpaired).
func Dial(ctx context.Context, addr string, cert *tls.Certificate) (*Client, error) {
	// The official app installs a permissive trust manager: the SHIELD uses a
	// self-signed certificate that cannot be verified, so any server cert is
	// accepted (protocol §1).
	cfg := &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	if cert != nil {
		cfg.Certificates = []tls.Certificate{*cert}
	}
	d := &tls.Dialer{NetDialer: &net.Dialer{Timeout: 5 * time.Second}, Config: cfg}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("beyonder: dial %s: %w", addr, err)
	}
	if tc, ok := conn.(*tls.Conn); ok {
		if nc, ok := tc.NetConn().(*net.TCPConn); ok {
			_ = nc.SetNoDelay(true)
		}
	}
	c := &Client{
		conn:    conn,
		msgs:    make(chan Message, 64),
		done:    make(chan struct{}),
		lastRTT: time.Now(),
	}
	go c.readLoop()
	return c, nil
}

func (c *Client) hook(dir string, b []byte) {
	if c.OnFrame != nil {
		c.OnFrame(dir, b)
	}
}

func (c *Client) readLoop() {
	defer close(c.msgs)
	var r Reassembler
	buf := make([]byte, readBufSize)
	for {
		n, err := c.conn.Read(buf)
		if n > 0 {
			chunk := append([]byte(nil), buf[:n]...)
			c.hook("rx", chunk)
			ms, ferr := r.Feed(chunk)
			for _, m := range ms {
				if m.IsRTT() {
					c.touchRTT()
					continue // handled internally, not forwarded
				}
				select {
				case c.msgs <- m:
				case <-c.done:
					return
				}
			}
			if ferr != nil {
				c.setErr(ferr)
				return
			}
		}
		if err != nil {
			c.setErr(err)
			return
		}
	}
}

func (c *Client) setErr(err error) {
	c.mu.Lock()
	if c.err == nil {
		c.err = err
	}
	c.mu.Unlock()
}

func (c *Client) touchRTT() {
	c.mu.Lock()
	c.lastRTT = time.Now()
	c.mu.Unlock()
}

// Err returns the error that ended the reader (valid once Messages is closed).
func (c *Client) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

// Messages returns decoded non-RTT messages; closed when the connection ends.
func (c *Client) Messages() <-chan Message { return c.msgs }

// Send writes frame (splitting per Frames), one Write per element.
func (c *Client) Send(ctx context.Context, frame []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if dl, ok := ctx.Deadline(); ok {
		_ = c.conn.SetWriteDeadline(dl)
		defer func() { _ = c.conn.SetWriteDeadline(time.Time{}) }()
	}
	for _, f := range Frames(frame) {
		c.hook("tx", f)
		if _, err := c.conn.Write(f); err != nil {
			return fmt.Errorf("beyonder: send: %w", err)
		}
	}
	return nil
}

// Close closes the connection and stops background goroutines.
func (c *Client) Close() error {
	var err error
	c.closeOnce.Do(func() {
		close(c.done)
		err = c.conn.Close()
	})
	return err
}

// StartKeepalive runs the RTT keepalive in the background until ctx is done
// or the client is closed: every 3 s, send a probe if >=5 s elapsed since the
// last RTT activity.
func (c *Client) StartKeepalive(ctx context.Context) {
	go func() {
		t := time.NewTicker(rttTick)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-c.done:
				return
			case <-t.C:
			}
			c.mu.Lock()
			idle := time.Since(c.lastRTT) >= rttIdle
			c.mu.Unlock()
			if !idle {
				continue
			}
			p, err := EncodeRTTProbe(uint64(time.Now().UnixMilli()))
			if err != nil {
				continue
			}
			if err := c.Send(ctx, p); err != nil {
				if !errors.Is(err, context.Canceled) {
					c.setErr(err)
				}
				return
			}
			c.touchRTT()
		}
	}()
}
