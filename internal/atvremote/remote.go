package atvremote

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	pb "github.com/drosocode/atvremote/pkg/v2/proto"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

const (
	featurePing   int32 = 1 << 0
	featureKey    int32 = 1 << 1
	featurePower  int32 = 1 << 5
	featureVolume int32 = 1 << 6
	featureApp    int32 = 1 << 9
	protoCode1    int32 = 622
)

const defaultFeatures int32 = featurePing | featureKey | featurePower | featureVolume | featureApp

type SendKeyParams struct {
	Host         string
	Port         int
	CertPath     string
	KeyPath      string
	ReadyTimeout time.Duration
	PostDelay    time.Duration
}

type SendKeyResult struct {
	Host              string
	Port              int
	Action            string
	SupportedFeatures int32
	ActiveFeatures    int32
	Powered           bool
	HasPowerState     bool
}

type remoteClient struct {
	conn *tls.Conn
	r    *bufio.Reader

	writeMu sync.Mutex

	activeFeatures int32
	supportedBits  int32

	stateMu       sync.RWMutex
	powered       bool
	hasPowerState bool
	lastErr       error

	readyOnce sync.Once
	readyCh   chan struct{}
}

func SendKey(ctx context.Context, params SendKeyParams, action string) (*SendKeyResult, error) {
	params = normalizeSendKeyParams(params)
	if params.Host == "" {
		return nil, errors.New("host is required")
	}

	keyCode, err := ResolveKeyCode(action)
	if err != nil {
		return nil, err
	}

	cert, err := tls.LoadX509KeyPair(params.CertPath, params.KeyPath)
	if err != nil {
		return nil, fmt.Errorf("load cert/key: %w", err)
	}

	dialer := &tls.Dialer{
		Config: &tls.Config{
			Certificates:       []tls.Certificate{cert},
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
	defer client.close()
	go client.run()

	if err := client.waitReady(params.ReadyTimeout); err != nil {
		return nil, fmt.Errorf("wait remote ready: %w", err)
	}

	if err := client.sendKey(keyCode, pb.RemoteDirection_SHORT); err != nil {
		return nil, fmt.Errorf("send key: %w", err)
	}

	if params.PostDelay > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(params.PostDelay):
		}
	}

	if err := client.getErr(); err != nil {
		return nil, err
	}

	supported, active := client.getFeatures()
	powered, hasPower := client.getPowerState()

	return &SendKeyResult{
		Host:              params.Host,
		Port:              params.Port,
		Action:            normalizeAction(action),
		SupportedFeatures: supported,
		ActiveFeatures:    active,
		Powered:           powered,
		HasPowerState:     hasPower,
	}, nil
}

func normalizeSendKeyParams(params SendKeyParams) SendKeyParams {
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

func newRemoteClient(conn *tls.Conn) *remoteClient {
	return &remoteClient{
		conn:           conn,
		r:              bufio.NewReader(conn),
		activeFeatures: defaultFeatures,
		supportedBits:  0,
		readyCh:        make(chan struct{}),
	}
}

func (c *remoteClient) run() {
	for {
		msg, err := c.readMessage()
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
				c.setErr(err)
			}
			return
		}
		if err := c.handleIncoming(msg); err != nil {
			c.setErr(err)
			return
		}
	}
}

func (c *remoteClient) waitReady(timeout time.Duration) error {
	select {
	case <-c.readyCh:
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("timeout waiting for remote_start")
	}
}

func (c *remoteClient) readMessage() (*pb.RemoteMessage, error) {
	size, err := binary.ReadUvarint(c.r)
	if err != nil {
		return nil, err
	}
	if size == 0 {
		return &pb.RemoteMessage{}, nil
	}
	if size > 4*1024*1024 {
		return nil, fmt.Errorf("incoming message too large: %d", size)
	}

	raw := make([]byte, size)
	if _, err := io.ReadFull(c.r, raw); err != nil {
		return nil, err
	}

	var msg pb.RemoteMessage
	if err := proto.Unmarshal(raw, &msg); err != nil {
		return nil, fmt.Errorf("protobuf unmarshal: %w", err)
	}
	return &msg, nil
}

func (c *remoteClient) writeMessage(msg *pb.RemoteMessage) error {
	raw, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("protobuf marshal: %w", err)
	}
	prefix := protowire.AppendVarint(nil, uint64(len(raw)))

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if _, err := c.conn.Write(prefix); err != nil {
		return fmt.Errorf("write prefix: %w", err)
	}
	if _, err := c.conn.Write(raw); err != nil {
		return fmt.Errorf("write payload: %w", err)
	}
	return nil
}

func (c *remoteClient) handleIncoming(msg *pb.RemoteMessage) error {
	switch {
	case msg.RemoteConfigure != nil:
		supported := msg.RemoteConfigure.Code1
		c.stateMu.Lock()
		c.supportedBits = supported
		c.activeFeatures = protoCode1
		c.stateMu.Unlock()

		return c.writeMessage(&pb.RemoteMessage{
			RemoteConfigure: &pb.RemoteConfigure{
				Code1: protoCode1,
				DeviceInfo: &pb.RemoteDeviceInfo{
					Model:       "shield-poc",
					Vendor:      "shield-poc",
					Unknown1:    1,
					Unknown2:    "1",
					PackageName: "shield-poc",
					AppVersion:  "0.1.0",
				},
			},
		})

	case msg.RemoteSetActive != nil:
		err := c.writeMessage(&pb.RemoteMessage{
			RemoteSetActive: &pb.RemoteSetActive{Active: protoCode1},
		})
		if err == nil {
			c.markReady()
		}
		return err

	case msg.RemotePingRequest != nil:
		return c.writeMessage(&pb.RemoteMessage{
			RemotePingResponse: &pb.RemotePingResponse{Val1: msg.RemotePingRequest.Val1},
		})

	case msg.RemoteStart != nil:
		c.stateMu.Lock()
		c.powered = msg.RemoteStart.Started
		c.hasPowerState = true
		c.stateMu.Unlock()
		c.markReady()
		return nil

	case msg.RemoteError != nil:
		if msg.RemoteError.Value {
			return errors.New("received remote_error from device")
		}
	}
	return nil
}

func (c *remoteClient) sendKey(key pb.RemoteKeyCode, direction pb.RemoteDirection) error {
	return c.writeMessage(&pb.RemoteMessage{
		RemoteKeyInject: &pb.RemoteKeyInject{
			KeyCode:   key,
			Direction: direction,
		},
	})
}

func (c *remoteClient) close() {
	_ = c.conn.Close()
}

func (c *remoteClient) setErr(err error) {
	if err == nil {
		return
	}
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if c.lastErr == nil {
		c.lastErr = err
	}
}

func (c *remoteClient) getErr() error {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.lastErr
}

func (c *remoteClient) markReady() {
	c.readyOnce.Do(func() {
		close(c.readyCh)
	})
}

func (c *remoteClient) getPowerState() (bool, bool) {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.powered, c.hasPowerState
}

func (c *remoteClient) getFeatures() (supported int32, active int32) {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.supportedBits, c.activeFeatures
}
