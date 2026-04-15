package atvremote

import (
	"bufio"
	"context"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"

	atvpb "shield-poc/internal/atvremote/proto"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

const (
	DefaultRemotePort  = 6466
	DefaultPairingPort = 6467
	DefaultServiceName = "atvremote"
)

type PairParams struct {
	Host         string
	Port         int
	ClientName   string
	ServiceName  string
	PairingCode  string
	CodeProvider func() (string, error)
	CertPath     string
	KeyPath      string
}

type PairResult struct {
	Host       string
	Port       int
	ClientName string
	ServerName string
	CertPath   string
	KeyPath    string
}

type pairingClient struct {
	conn        *tls.Conn
	r           *bufio.Reader
	serviceName string
	clientName  string
	serverName  string
}

func normalizePairParams(params PairParams) PairParams {
	if params.Port == 0 {
		params.Port = DefaultPairingPort
	}
	if params.ClientName == "" {
		params.ClientName = "shield-poc"
	}
	if params.ServiceName == "" {
		params.ServiceName = DefaultServiceName
	}
	return params
}

func Pair(ctx context.Context, params PairParams) (*PairResult, error) {
	params = normalizePairParams(params)
	if params.Host == "" {
		return nil, errors.New("host is required")
	}

	if err := EnsureClientCertificate(params.CertPath, params.KeyPath, params.ClientName); err != nil {
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
		return nil, fmt.Errorf("pair dial: %w", err)
	}
	defer conn.Close()

	tlsConn, ok := conn.(*tls.Conn)
	if !ok {
		return nil, errors.New("expected tls connection")
	}

	client := newPairingClient(tlsConn, params.ServiceName, params.ClientName)
	if err := client.start(); err != nil {
		return nil, err
	}

	code := params.PairingCode
	if strings.TrimSpace(code) == "" {
		if params.CodeProvider == nil {
			return nil, errors.New("pairing code is required")
		}
		var err error
		code, err = params.CodeProvider()
		if err != nil {
			return nil, err
		}
	}
	code, err = NormalizePairingCode(code)
	if err != nil {
		return nil, err
	}

	if err := client.finish(code, &cert); err != nil {
		return nil, err
	}

	return &PairResult{
		Host:       params.Host,
		Port:       params.Port,
		ClientName: params.ClientName,
		ServerName: client.serverName,
		CertPath:   params.CertPath,
		KeyPath:    params.KeyPath,
	}, nil
}

func NormalizePairingCode(raw string) (string, error) {
	code := strings.ToUpper(strings.TrimSpace(raw))
	if len(code) != 6 {
		return "", fmt.Errorf("pairing code must be 6 hex chars, got %q", raw)
	}
	if _, err := hex.DecodeString(code); err != nil {
		return "", fmt.Errorf("pairing code must be hex: %w", err)
	}
	return code, nil
}

func newPairingClient(conn *tls.Conn, serviceName, clientName string) *pairingClient {
	return &pairingClient{
		conn:        conn,
		r:           bufio.NewReader(conn),
		serviceName: serviceName,
		clientName:  clientName,
	}
}

func (p *pairingClient) start() error {
	request := defaultPairingMessage()
	request.PairingRequest = &atvpb.PairingRequest{
		ServiceName: proto.String(p.serviceName),
		ClientName:  proto.String(p.clientName),
	}
	if err := p.writeMessage(request); err != nil {
		return fmt.Errorf("pair request: %w", err)
	}

	msg, err := p.readMessage()
	if err != nil {
		return fmt.Errorf("pair request ack: %w", err)
	}
	if err := ensureStatusOK(msg, "pair request ack"); err != nil {
		return err
	}
	if msg.PairingRequestAck != nil && msg.PairingRequestAck.ServerName != nil {
		p.serverName = msg.PairingRequestAck.GetServerName()
	}

	options := defaultPairingMessage()
	options.Options = &atvpb.Options{
		PreferredRole: atvpb.Options_ROLE_TYPE_INPUT.Enum(),
		InputEncodings: []*atvpb.Options_Encoding{{
			Type:         atvpb.Options_Encoding_ENCODING_TYPE_HEXADECIMAL.Enum(),
			SymbolLength: proto.Uint32(6),
		}},
	}
	if err := p.writeMessage(options); err != nil {
		return fmt.Errorf("pair options: %w", err)
	}

	msg, err = p.readMessage()
	if err != nil {
		return fmt.Errorf("pair options ack: %w", err)
	}
	if err := ensureStatusOK(msg, "pair options ack"); err != nil {
		return err
	}

	configuration := defaultPairingMessage()
	configuration.Configuration = &atvpb.Configuration{
		ClientRole: atvpb.Options_ROLE_TYPE_INPUT.Enum(),
		Encoding: &atvpb.Options_Encoding{
			Type:         atvpb.Options_Encoding_ENCODING_TYPE_HEXADECIMAL.Enum(),
			SymbolLength: proto.Uint32(6),
		},
	}
	if err := p.writeMessage(configuration); err != nil {
		return fmt.Errorf("pair configuration: %w", err)
	}

	msg, err = p.readMessage()
	if err != nil {
		return fmt.Errorf("pair configuration ack: %w", err)
	}
	if err := ensureStatusOK(msg, "pair configuration ack"); err != nil {
		return err
	}

	return nil
}

func (p *pairingClient) finish(pairingCode string, cert *tls.Certificate) error {
	clientCert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return fmt.Errorf("parse client cert: %w", err)
	}

	peerCerts := p.conn.ConnectionState().PeerCertificates
	if len(peerCerts) == 0 {
		return errors.New("no peer certificate from tv")
	}

	secret, err := computePairingSecret(pairingCode, clientCert, peerCerts[0])
	if err != nil {
		return err
	}

	msg := defaultPairingMessage()
	msg.Secret = &atvpb.Secret{Secret: secret}
	if err := p.writeMessage(msg); err != nil {
		return fmt.Errorf("pair secret: %w", err)
	}

	msg, err = p.readMessage()
	if err != nil {
		return fmt.Errorf("pair secret ack: %w", err)
	}
	if err := ensureStatusOK(msg, "pair secret ack"); err != nil {
		return err
	}

	return nil
}

func (p *pairingClient) writeMessage(msg *atvpb.OuterMessage) error {
	raw, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal pairing message: %w", err)
	}
	prefix := protowire.AppendVarint(nil, uint64(len(raw)))
	if _, err := p.conn.Write(prefix); err != nil {
		return fmt.Errorf("write pairing message prefix: %w", err)
	}
	if _, err := p.conn.Write(raw); err != nil {
		return fmt.Errorf("write pairing message payload: %w", err)
	}
	return nil
}

func (p *pairingClient) readMessage() (*atvpb.OuterMessage, error) {
	size, err := binary.ReadUvarint(p.r)
	if err != nil {
		return nil, fmt.Errorf("read pairing message size: %w", err)
	}
	if size == 0 {
		return &atvpb.OuterMessage{}, nil
	}
	if size > 1024*1024 {
		return nil, fmt.Errorf("pairing message too large: %d", size)
	}

	raw := make([]byte, size)
	if _, err := io.ReadFull(p.r, raw); err != nil {
		return nil, fmt.Errorf("read pairing message payload: %w", err)
	}

	var msg atvpb.OuterMessage
	if err := proto.Unmarshal(raw, &msg); err != nil {
		return nil, fmt.Errorf("unmarshal pairing message: %w", err)
	}
	return &msg, nil
}

func defaultPairingMessage() *atvpb.OuterMessage {
	return &atvpb.OuterMessage{
		ProtocolVersion: proto.Uint32(2),
		Status:          atvpb.OuterMessage_STATUS_OK.Enum(),
	}
}

func ensureStatusOK(msg *atvpb.OuterMessage, stage string) error {
	if msg == nil {
		return fmt.Errorf("%s: empty message", stage)
	}
	if msg.GetStatus() != atvpb.OuterMessage_STATUS_OK {
		return fmt.Errorf("%s: unexpected status %v", stage, msg.GetStatus())
	}
	return nil
}

func computePairingSecret(pairingCode string, clientCert, serverCert *x509.Certificate) ([]byte, error) {
	clientPublicKey, ok := clientCert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("client certificate key is not RSA")
	}
	serverPublicKey, ok := serverCert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("server certificate key is not RSA")
	}

	code, err := NormalizePairingCode(pairingCode)
	if err != nil {
		return nil, err
	}

	hash := sha256.New()
	hash.Write(clientPublicKey.N.Bytes())
	hash.Write(bigEndianIntBytes(clientPublicKey.E))
	hash.Write(serverPublicKey.N.Bytes())
	hash.Write(bigEndianIntBytes(serverPublicKey.E))
	pairTail, err := hex.DecodeString(code[2:])
	if err != nil {
		return nil, fmt.Errorf("decode pairing code tail: %w", err)
	}
	hash.Write(pairTail)
	secret := hash.Sum(nil)

	prefix, err := hex.DecodeString(code[:2])
	if err != nil {
		return nil, fmt.Errorf("decode pairing code prefix: %w", err)
	}
	if len(prefix) != 1 || secret[0] != prefix[0] {
		return nil, fmt.Errorf("unexpected hash for pairing code: %s", code)
	}

	return secret, nil
}

func bigEndianIntBytes(value int) []byte {
	if value == 0 {
		return []byte{0}
	}

	var out []byte
	for value > 0 {
		out = append([]byte{byte(value & 0xff)}, out...)
		value >>= 8
	}
	return out
}

func inferServerName(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
	}
	if i := strings.Index(host, "%"); i >= 0 {
		host = host[:i]
	}
	if ip := net.ParseIP(host); ip != nil {
		return ""
	}
	return host
}

func endpoint(host string, port int) string {
	return net.JoinHostPort(host, strconv.Itoa(port))
}
