package beyonder

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	pb "shrmt/internal/beyonder/proto"
)

func selfSigned(t *testing.T, key any, pub any) []byte {
	t.Helper()
	tpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, pub, key)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

func authMsg(t *testing.T, cmd pb.AuthenticationServicePayload_CommandType, obj proto.Message) []byte {
	t.Helper()
	b, err := auth(cmd, obj)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

type fakeShield struct {
	ln       net.Listener
	level    pb.AuthorizationLevel_Level
	issuedC  []byte
	issuedK  []byte
	received chan []byte
}

func newShield(t *testing.T, level pb.AuthorizationLevel_Level, certDER, keyDER []byte) *fakeShield {
	t.Helper()
	ek, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	sc := tls.Certificate{Certificate: [][]byte{selfSigned(t, ek, &ek.PublicKey)}, PrivateKey: ek}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{sc}, ClientAuth: tls.RequestClientCert})
	if err != nil {
		t.Fatal(err)
	}
	s := &fakeShield{ln: ln, level: level, issuedC: certDER, issuedK: keyDER, received: make(chan []byte, 16)}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serve(t, c)
		}
	}()
	return s
}

func (s *fakeShield) serve(t *testing.T, c net.Conn) {
	defer c.Close()
	buf := make([]byte, 65536)
	for {
		n, err := c.Read(buf)
		if err != nil {
			return
		}
		chunk := append([]byte(nil), buf[:n]...)
		s.received <- chunk
		m, err := Decode(chunk)
		if err != nil {
			return
		}
		switch {
		case m.Handshake != nil:
			c.Write(authMsg(t, pb.AuthenticationServicePayload_COMMAND_AUTHORIZATION_LEVEL,
				&pb.AuthorizationLevel{Level: s.level, DeviceName: "SHIELD", Capability: 0x13FFB}))
		case m.Authentication != nil && m.Authentication.Command == pb.AuthenticationServicePayload_COMMAND_PIN_REQUEST:
			c.Write(authMsg(t, pb.AuthenticationServicePayload_COMMAND_PIN_CHALLENGE, nil))
		case m.Authentication != nil && m.Authentication.Command == pb.AuthenticationServicePayload_COMMAND_PIN_CODE:
			c.Write(authMsg(t, pb.AuthenticationServicePayload_COMMAND_PAIRING_RESULT,
				&pb.PairingResult{ResultType: pb.PairingResult_RESULT_SUCCESS, PrivateKey: s.issuedK, Certificate: s.issuedC}))
		}
	}
}

func TestPairAndInput(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rk, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	certDER := selfSigned(t, rk, &rk.PublicKey)
	keyDER, _ := x509.MarshalPKCS8PrivateKey(rk)

	sh := newShield(t, pb.AuthorizationLevel_LEVEL_NONE, certDER, keyDER)
	c, err := Dial(ctx, sh.ln.Addr().String(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	al, err := c.Hello(ctx, "shrmt")
	if err != nil || al.Level != 1 || al.DeviceName != "SHIELD" {
		t.Fatalf("hello: %+v %v", al, err)
	}
	if err := c.SendSpecialInput(ctx, pb.VirtualInputPayload_ACTION_SLEEP_NOW); err != ErrNotPaired {
		t.Fatalf("want ErrNotPaired, got %v", err)
	}
	cert, err := c.Pair(ctx, func() (string, error) { return "1234", nil }, "dev")
	if err != nil {
		t.Fatal(err)
	}
	if cert.Leaf == nil || len(cert.Certificate) != 1 {
		t.Fatal("bad cert")
	}
	c.Close()

	// PEM-encoded variants parse too.
	if _, err := ParseCredentials(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})); err != nil {
		t.Fatal(err)
	}

	sh2 := newShield(t, pb.AuthorizationLevel_LEVEL_VIRTUAL_INPUT, nil, nil)
	c2, err := Dial(ctx, sh2.ln.Addr().String(), &cert)
	if err != nil {
		t.Fatal(err)
	}
	defer c2.Close()
	if al, err := c2.Hello(ctx, "shrmt"); err != nil || al.Level != 1000 {
		t.Fatalf("hello2: %+v %v", al, err)
	}
	<-sh2.received // hello
	if err := c2.SendSpecialInput(ctx, pb.VirtualInputPayload_ACTION_SLEEP_NOW); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-sh2.received:
		if hex.EncodeToString(got) != "08e907120808141005201e401e" {
			t.Fatalf("got %x", got)
		}
	case <-ctx.Done():
		t.Fatal("timeout")
	}
}

func TestPairFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sh := newShield(t, pb.AuthorizationLevel_LEVEL_NONE, nil, nil) // empty key/cert => handled as failure
	c, err := Dial(ctx, sh.ln.Addr().String(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := c.Hello(ctx, "x"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Pair(ctx, func() (string, error) { return "0", nil }, "d"); err == nil {
		t.Fatal("expected error")
	}
}

func TestCreds(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if c, err := LoadCredentials(); c != nil || err != nil {
		t.Fatalf("want nil,nil got %v %v", c, err)
	}
	rk, _ := rsa.GenerateKey(rand.Reader, 2048)
	certDER := selfSigned(t, rk, &rk.PublicKey)
	keyDER, _ := x509.MarshalPKCS8PrivateKey(rk)
	if err := SaveCredentials(certDER, keyDER); err != nil {
		t.Fatal(err)
	}
	c, err := LoadCredentials()
	if err != nil || c == nil {
		t.Fatalf("load: %v %v", c, err)
	}
	dir, _ := configDir()
	for _, f := range []string{certFile, keyFile} {
		st, err := os.Stat(filepath.Join(dir, f))
		if err != nil || st.Mode().Perm() != 0o600 {
			t.Fatalf("%s: %v %v", f, st, err)
		}
	}
	id1, err := LoadOrCreateDeviceID()
	if err != nil || len(id1) != 32 || strings.Trim(id1, "0123456789abcdef") != "" {
		t.Fatalf("id %q %v", id1, err)
	}
	if id2, _ := LoadOrCreateDeviceID(); id2 != id1 {
		t.Fatal("device id not stable")
	}
}
