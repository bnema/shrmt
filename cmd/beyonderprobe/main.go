// Command beyonderprobe pairs with an NVIDIA SHIELD and sends special inputs
// over the beyonder protocol (port 8987).
package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	xdgstore "shrmt/adapters/out/xdg"
	"shrmt/internal/beyonder"
	pb "shrmt/internal/beyonder/proto"
)

type options struct {
	host    string
	port    int
	name    string
	timeout time.Duration
	dump    bool
	wait    time.Duration
}

var specialNames = map[string]pb.VirtualInputPayload_SpecialInput{
	"home":       pb.VirtualInputPayload_ACTION_HOME,
	"back":       pb.VirtualInputPayload_ACTION_BACK,
	"settings":   pb.VirtualInputPayload_ACTION_SETTINGS,
	"power-menu": pb.VirtualInputPayload_ACTION_POWER_MENU,
	"sleep":      pb.VirtualInputPayload_ACTION_SLEEP_NOW,
	"assist":     pb.VirtualInputPayload_ACTION_GOOGLE_ASSIST,
	"osc":        pb.VirtualInputPayload_ACTION_TOGGLE_OSC,
}

// parseSpecialName maps a short name to a SpecialInput.
func parseSpecialName(s string) (pb.VirtualInputPayload_SpecialInput, error) {
	if v, ok := specialNames[strings.ToLower(strings.TrimSpace(s))]; ok {
		return v, nil
	}
	return 0, fmt.Errorf("unknown special name %q (home|back|settings|power-menu|sleep|assist|osc)", s)
}

// capabilityNames lists the names of the set capability bits, in bit order.
func capabilityNames(caps int32) []string {
	var out []string
	var known int32
	for bit := int32(1); bit > 0 && bit <= 1<<30; bit <<= 1 {
		if name, ok := pb.Capability_name[bit]; ok {
			known |= bit
			if caps&bit != 0 {
				out = append(out, name)
			}
		}
	}
	if rest := caps &^ known; rest != 0 {
		out = append(out, fmt.Sprintf("UNKNOWN(0x%x)", rest))
	}
	return out
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "beyonderprobe:", err)
		os.Exit(1)
	}
}

func usage(w io.Writer) {
	fmt.Fprint(w, `usage: beyonderprobe <command> [flags]

commands:
  hello     connect, handshake, print auth level and capabilities
  pair      pair with the SHIELD using the PIN shown on the TV
  power     send ACTION_SLEEP_NOW (needs pairing)
  special   send a special input (--value N or --name home|back|settings|power-menu|sleep|assist|osc)

common flags: --host --port --name --timeout --dump --wait (see <command> -h)
`)
}

func run(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		usage(os.Stdout)
		if len(args) == 0 {
			return errors.New("missing command")
		}
		return nil
	}
	cmd := args[0]
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	var o options
	fs.StringVar(&o.host, "host", "", "SHIELD host (default: saved target)")
	fs.IntVar(&o.port, "port", 8987, "beyonder port")
	fs.DurationVar(&o.timeout, "timeout", 10*time.Second, "per-step timeout")
	fs.BoolVar(&o.dump, "dump", false, "print every sent/received chunk")
	fs.DurationVar(&o.wait, "wait", 2*time.Second, "keep listening this long after the command")
	if cmd != "special" {
		fs.StringVar(&o.name, "name", "shrmt", "name sent in hello")
	}
	var value int
	var sname string
	switch cmd {
	case "hello", "pair", "power":
	case "special":
	default:
		usage(os.Stderr)
		return fmt.Errorf("unknown command %q", cmd)
	}
	if cmd == "special" {
		// "--name" selects the special input here, so the hello name is
		// "--hello-name" for this command.
		fs = flag.NewFlagSet(cmd, flag.ContinueOnError)
		fs.StringVar(&o.host, "host", "", "SHIELD host (default: saved target)")
		fs.IntVar(&o.port, "port", 8987, "beyonder port")
		fs.StringVar(&o.name, "hello-name", "shrmt", "name sent in hello")
		fs.DurationVar(&o.timeout, "timeout", 10*time.Second, "per-step timeout")
		fs.BoolVar(&o.dump, "dump", false, "print every sent/received chunk")
		fs.DurationVar(&o.wait, "wait", 2*time.Second, "keep listening this long after the command")
		fs.IntVar(&value, "value", -1, "SpecialInput numeric value")
		fs.StringVar(&sname, "name", "", "special input: home|back|settings|power-menu|sleep|assist|osc")
	}
	if err := fs.Parse(args[1:]); err != nil {
		return flag.ErrHelp
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if o.host == "" {
		t, err := xdgstore.NewTargetStore().Load(ctx)
		if err != nil || t.Host == "" {
			return errors.New("no --host given and no saved target")
		}
		o.host = t.Host
	}

	switch cmd {
	case "hello":
		return cmdHello(ctx, o)
	case "pair":
		return cmdPair(ctx, o)
	case "power":
		return cmdSend(ctx, o, pb.VirtualInputPayload_ACTION_SLEEP_NOW)
	default:
		var v pb.VirtualInputPayload_SpecialInput
		switch {
		case sname != "" && value >= 0:
			return errors.New("use either --value or --name, not both")
		case sname != "":
			var err error
			if v, err = parseSpecialName(sname); err != nil {
				return err
			}
		case value >= 0:
			v = pb.VirtualInputPayload_SpecialInput(value)
		default:
			return errors.New("special needs --value N or --name NAME")
		}
		return cmdSend(ctx, o, v)
	}
}

func (o options) addr() string { return net.JoinHostPort(o.host, strconv.Itoa(o.port)) }

func (o options) connect(ctx context.Context, cert *tls.Certificate) (*beyonder.Client, error) {
	dctx, cancel := context.WithTimeout(ctx, o.timeout)
	defer cancel()
	c, err := beyonder.Dial(dctx, o.addr(), cert)
	if err != nil {
		return nil, err
	}
	if o.dump {
		var r beyonder.Reassembler // rx frames come from a single reader goroutine
		c.OnFrame = func(dir string, b []byte) {
			if dir == "tx" {
				fmt.Printf("> %x\n", b)
				return
			}
			fmt.Printf("< %x\n", b)
			ms, err := r.Feed(b)
			for _, m := range ms {
				fmt.Println("  =", m.String())
			}
			if err != nil {
				fmt.Println("  = decode error:", err)
			}
		}
	}
	return c, nil
}

func printAuth(al beyonder.AuthLevel) {
	fmt.Printf("auth level: %d\n", al.Level)
	fmt.Printf("device name: %s\n", al.DeviceName)
	fmt.Printf("capabilities: 0x%x [%s]\n", al.Capability, strings.Join(capabilityNames(al.Capability), ", "))
}

// listen prints messages for d (or until the connection closes / ctx ends).
func (o options) listen(ctx context.Context, c *beyonder.Client, d time.Duration) {
	if d <= 0 {
		return
	}
	t := time.NewTimer(d)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			return
		case m, ok := <-c.Messages():
			if !ok {
				fmt.Println("connection closed:", c.Err())
				return
			}
			printMsg(m)
		}
	}
}

func printMsg(m beyonder.Message) { fmt.Println("<", m.String()) }

func loadCreds() (*tls.Certificate, error) {
	cert, err := beyonder.LoadCredentials()
	if err != nil {
		return nil, fmt.Errorf("load credentials: %w", err)
	}
	return cert, nil
}

func cmdHello(ctx context.Context, o options) error {
	cert, err := loadCreds()
	if err != nil {
		return err
	}
	c, err := o.connect(ctx, cert)
	if err != nil {
		return err
	}
	defer c.Close()
	al, err := o.helloTimeout(ctx, c)
	if err != nil {
		return err
	}
	printAuth(al)
	o.listen(ctx, c, o.wait)
	return nil
}

// helloTimeout runs Hello bounded by o.timeout for the handshake itself.
func (o options) helloTimeout(ctx context.Context, c *beyonder.Client) (beyonder.AuthLevel, error) {
	type res struct {
		al  beyonder.AuthLevel
		err error
	}
	ch := make(chan res, 1)
	go func() {
		al, err := c.Hello(ctx, o.name)
		ch <- res{al, err}
	}()
	select {
	case r := <-ch:
		return r.al, r.err
	case <-time.After(o.timeout):
		_ = c.Close()
		return beyonder.AuthLevel{}, fmt.Errorf("hello timed out after %s", o.timeout)
	case <-ctx.Done():
		_ = c.Close()
		return beyonder.AuthLevel{}, ctx.Err()
	}
}

func cmdPair(ctx context.Context, o options) error {
	c, err := o.connect(ctx, nil)
	if err != nil {
		return err
	}
	defer c.Close()
	al, err := o.helloTimeout(ctx, c)
	if err != nil {
		return err
	}
	printAuth(al)
	if al.Level == beyonder.LevelVirtualInput {
		fmt.Println("already authorized (level 1000); nothing to pair")
		return nil
	}
	id, err := beyonder.LoadOrCreateDeviceID()
	if err != nil {
		return fmt.Errorf("device id: %w", err)
	}
	in := bufio.NewReader(os.Stdin)
	prompt := func() (string, error) {
		fmt.Print("PIN shown on the TV: ")
		line, err := in.ReadString('\n')
		if err != nil && line == "" {
			return "", err
		}
		return strings.TrimSpace(line), nil
	}
	pctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cert, err := c.Pair(pctx, prompt, id)
	if err != nil {
		return err
	}
	if err := beyonder.SaveCertificate(cert); err != nil {
		return fmt.Errorf("save certificate: %w", err)
	}
	fmt.Println("paired; certificate saved")
	// Show any same-connection level change.
	o.listen(ctx, c, o.wait)
	_ = c.Close()

	c2, err := o.connect(ctx, &cert)
	if err != nil {
		return fmt.Errorf("reconnect: %w", err)
	}
	defer c2.Close()
	al2, err := o.helloTimeout(ctx, c2)
	if err != nil {
		return fmt.Errorf("reconnect hello: %w", err)
	}
	printAuth(al2)
	if al2.Level == beyonder.LevelVirtualInput {
		fmt.Println("confirmed: level 1000 with new certificate")
		return nil
	}
	return fmt.Errorf("after pairing the level is %d, expected %d", al2.Level, beyonder.LevelVirtualInput)
}

func cmdSend(ctx context.Context, o options, v pb.VirtualInputPayload_SpecialInput) error {
	cert, err := loadCreds()
	if err != nil {
		return err
	}
	if cert == nil {
		return errors.New("no stored credentials: run beyonderprobe pair first")
	}
	c, err := o.connect(ctx, cert)
	if err != nil {
		return err
	}
	defer c.Close()
	al, err := o.helloTimeout(ctx, c)
	if err != nil {
		return err
	}
	printAuth(al)
	sctx, cancel := context.WithTimeout(ctx, o.timeout)
	defer cancel()
	if err := c.SendSpecialInput(sctx, v); err != nil {
		if errors.Is(err, beyonder.ErrNotPaired) {
			return fmt.Errorf("%w: run beyonderprobe pair first", err)
		}
		return err
	}
	fmt.Printf("sent special input %s (%d)\n", v, int32(v))
	o.listen(ctx, c, o.wait)
	return nil
}
