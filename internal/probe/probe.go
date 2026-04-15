package probe

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"slices"
	"strconv"
	"strings"
)

type Target struct {
	Service  string   `json:"service,omitempty"`
	Instance string   `json:"instance,omitempty"`
	HostName string   `json:"host_name,omitempty"`
	Address  string   `json:"address"`
	Port     int      `json:"port"`
	Text     []string `json:"text,omitempty"`
}

type TLSInfo struct {
	Enabled          bool     `json:"enabled"`
	ServerName       string   `json:"server_name,omitempty"`
	Protocol         string   `json:"protocol,omitempty"`
	CipherSuite      string   `json:"cipher_suite,omitempty"`
	Subject          string   `json:"subject,omitempty"`
	Issuer           string   `json:"issuer,omitempty"`
	CommonName       string   `json:"common_name,omitempty"`
	IssuerCommonName string   `json:"issuer_common_name,omitempty"`
	DNSNames         []string `json:"dns_names,omitempty"`
	SelfSigned       bool     `json:"self_signed"`
}

type Result struct {
	Target       Target   `json:"target"`
	TCPReachable bool     `json:"tcp_reachable"`
	TLS          *TLSInfo `json:"tls,omitempty"`
	Error        string   `json:"error,omitempty"`
}

func Run(ctx context.Context, target Target) Result {
	result := Result{Target: target}

	if err := tcpReachable(ctx, target.Address, target.Port); err != nil {
		result.Error = fmt.Sprintf("tcp connect failed: %v", err)
		return result
	}
	result.TCPReachable = true

	tlsInfo, err := tlsProbe(ctx, target)
	if err != nil {
		result.Error = fmt.Sprintf("tls handshake failed: %v", err)
		return result
	}
	result.TLS = tlsInfo

	return result
}

func tcpReachable(ctx context.Context, address string, port int) error {
	dialer := newDialer(ctx)
	conn, err := dialer.DialContext(ctx, "tcp", endpoint(address, port))
	if err != nil {
		return err
	}
	_ = conn.Close()
	return nil
}

func tlsProbe(ctx context.Context, target Target) (*TLSInfo, error) {
	dialer := newDialer(ctx)
	rawConn, err := dialer.DialContext(ctx, "tcp", endpoint(target.Address, target.Port))
	if err != nil {
		return nil, err
	}
	defer rawConn.Close()

	serverName := inferServerName(target)
	conn := tls.Client(rawConn, &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         serverName,
	})
	defer conn.Close()

	if err := conn.HandshakeContext(ctx); err != nil {
		return nil, err
	}

	state := conn.ConnectionState()
	info := &TLSInfo{
		Enabled:     true,
		ServerName:  serverName,
		Protocol:    tlsVersion(state.Version),
		CipherSuite: tls.CipherSuiteName(state.CipherSuite),
	}

	if len(state.PeerCertificates) > 0 {
		cert := state.PeerCertificates[0]
		info.Subject = cert.Subject.String()
		info.Issuer = cert.Issuer.String()
		info.CommonName = cert.Subject.CommonName
		info.IssuerCommonName = cert.Issuer.CommonName
		info.DNSNames = sanitizeStrings(cert.DNSNames)
		info.SelfSigned = selfSigned(cert)
	}

	return info, nil
}

func newDialer(ctx context.Context) *net.Dialer {
	dialer := &net.Dialer{}
	if deadline, ok := ctx.Deadline(); ok {
		dialer.Deadline = deadline
	}
	return dialer
}

func inferServerName(target Target) string {
	if host := strings.TrimSpace(target.HostName); host != "" && net.ParseIP(host) == nil {
		return host
	}
	if addr := strings.TrimSpace(target.Address); addr != "" && net.ParseIP(addr) == nil {
		return addr
	}
	return ""
}

func endpoint(address string, port int) string {
	return net.JoinHostPort(address, strconv.Itoa(port))
}

func tlsVersion(v uint16) string {
	switch v {
	case tls.VersionTLS10:
		return "TLS1.0"
	case tls.VersionTLS11:
		return "TLS1.1"
	case tls.VersionTLS12:
		return "TLS1.2"
	case tls.VersionTLS13:
		return "TLS1.3"
	default:
		return fmt.Sprintf("0x%x", v)
	}
}

func selfSigned(cert *x509.Certificate) bool {
	if cert == nil {
		return false
	}
	return slices.Equal(cert.RawSubject, cert.RawIssuer)
}

func sanitizeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	slices.Sort(out)
	return out
}
