package main

import (
	"context"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	xdgstore "shrmt/adapters/out/xdg"
	intatv "shrmt/internal/atvremote"
)

type config struct {
	host                  string
	port                  int
	monitor               bool
	monitorHost           string
	monitorPort           int
	certPath              string
	keyPath               string
	mode                  string
	frameStyles           []string
	serviceTags           []int
	commandTags           []int
	payloadTags           []int
	inputTags             []int
	specialTags           []int
	serviceIDs            []int
	commandIDs            []int
	inputTypes            []int
	specialInputs         []int
	helloDevInfoTags      []int
	helloCapabilityTags   []int
	helloLayouts          []string
	helloName             string
	helloPackageName      string
	helloAndroidID        string
	helloHostValue        string
	helloRemotePorts      []int
	helloDeviceOSValues   []int
	helloCapabilityValues []int
	helloPreamble         bool
	connectTimeout        time.Duration
	responseWait          time.Duration
	pollWindow            time.Duration
	pollInterval          time.Duration
	delayBetween          time.Duration
	startAt               int
	limit                 int
	stopOnChange          bool
	logFile               string
}

type powerSample struct {
	Powered  bool   `json:"powered"`
	HasPower bool   `json:"has_power"`
	Error    string `json:"error,omitempty"`
}

type attempt struct {
	FrameStyle       string `json:"frame_style"`
	ServiceTag       int    `json:"service_tag"`
	CommandTag       int    `json:"command_tag"`
	PayloadTag       int    `json:"payload_tag"`
	InputTag         int    `json:"input_tag"`
	SpecialTag       int    `json:"special_tag"`
	ServiceID        int    `json:"service_id"`
	CommandID        int    `json:"command_id"`
	InputType        int    `json:"input_type"`
	SpecialInput     int    `json:"special_input"`
	Template         string `json:"template,omitempty"`
	PayloadDesc      string `json:"payload_desc,omitempty"`
	PreambleDesc     string `json:"preamble_desc,omitempty"`
	HasCustomPayload bool   `json:"-"`
	PayloadBytes     []byte `json:"-"`
	PreambleBytes    []byte `json:"-"`
}

type attemptLog struct {
	Index        int           `json:"index"`
	Total        int           `json:"total"`
	Attempt      attempt       `json:"attempt"`
	StartedAt    time.Time     `json:"started_at"`
	EndedAt      time.Time     `json:"ended_at"`
	SentHex      string        `json:"sent_hex"`
	SentHexes    []string      `json:"sent_hexes,omitempty"`
	ResponseHex  string        `json:"response_hex,omitempty"`
	SendError    string        `json:"send_error,omitempty"`
	PollSamples  []powerSample `json:"poll_samples,omitempty"`
	StateChanged bool          `json:"state_changed"`
}

func main() {
	cfg, err := parseConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	logWriter, err := openLogFile(cfg.logFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer logWriter.Close()
	enc := json.NewEncoder(logWriter)

	baseline := powerSample{}
	if cfg.monitor {
		baseline = samplePowerState(cfg)
	}
	fmt.Printf("nvprobe start: mode=%s host=%s:%d monitor=%t target=%s:%d attempts=%d log=%s\n", cfg.mode, cfg.host, cfg.port, cfg.monitor, cfg.monitorHost, cfg.monitorPort, countAttempts(cfg), cfg.logFile)
	if cfg.monitor {
		fmt.Printf("baseline power: hasPower=%t powered=%t err=%q\n", baseline.HasPower, baseline.Powered, baseline.Error)
	}

	attempts := buildAttempts(cfg)
	if cfg.startAt > 0 {
		if cfg.startAt >= len(attempts) {
			fmt.Fprintf(os.Stderr, "start-at=%d is past the attempt list (%d)\n", cfg.startAt, len(attempts))
			os.Exit(1)
		}
		attempts = attempts[cfg.startAt:]
	}
	if cfg.limit > 0 && cfg.limit < len(attempts) {
		attempts = attempts[:cfg.limit]
	}

	for idx, candidate := range attempts {
		startedAt := time.Now()
		raw := buildMessage(candidate)
		payloads := make([][]byte, 0, 2)
		sentHexes := make([]string, 0, 2)
		if len(candidate.PreambleBytes) > 0 {
			preamble := applyFrame(candidate.FrameStyle, candidate.PreambleBytes)
			payloads = append(payloads, preamble)
			sentHexes = append(sentHexes, hex.EncodeToString(preamble))
		}
		sent := applyFrame(candidate.FrameStyle, raw)
		payloads = append(payloads, sent)
		sentHexes = append(sentHexes, hex.EncodeToString(sent))
		response, sendErr := sendTLS(cfg, payloads)
		pollSamples, changed := pollForStateChange(cfg, baseline)
		endedAt := time.Now()

		entry := attemptLog{
			Index:        idx + 1,
			Total:        len(attempts),
			Attempt:      candidate,
			StartedAt:    startedAt,
			EndedAt:      endedAt,
			SentHex:      hex.EncodeToString(sent),
			SentHexes:    sentHexes,
			ResponseHex:  hex.EncodeToString(response),
			PollSamples:  pollSamples,
			StateChanged: changed,
		}
		if sendErr != nil {
			entry.SendError = sendErr.Error()
		}
		if err := enc.Encode(entry); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write log entry: %v\n", err)
		}

		fmt.Printf("[%04d/%04d] frame=%s svc(tag=%d,val=%d) cmd(tag=%d,val=%d) payload(tag=%d input %d=%d special %d=%d)",
			idx+1,
			len(attempts),
			candidate.FrameStyle,
			candidate.ServiceTag,
			candidate.ServiceID,
			candidate.CommandTag,
			candidate.CommandID,
			candidate.PayloadTag,
			candidate.InputTag,
			candidate.InputType,
			candidate.SpecialTag,
			candidate.SpecialInput,
		)
		if candidate.Template != "" {
			fmt.Printf(" template=%s", candidate.Template)
		}
		if candidate.PayloadDesc != "" {
			fmt.Printf(" desc=%s", candidate.PayloadDesc)
		}
		if candidate.PreambleDesc != "" {
			fmt.Printf(" preamble=%s", candidate.PreambleDesc)
		}
		switch {
		case sendErr != nil:
			fmt.Printf(" -> send-error: %v\n", sendErr)
		case changed:
			last := lastSample(pollSamples)
			fmt.Printf(" -> STATE CHANGED hasPower=%t powered=%t err=%q\n", last.HasPower, last.Powered, last.Error)
			if cfg.stopOnChange {
				fmt.Println("Stopping after observed power-state change.")
				return
			}
		case len(response) > 0:
			fmt.Printf(" -> response=%s\n", hex.EncodeToString(response))
		default:
			fmt.Println(" -> no observable change")
		}

		if cfg.delayBetween > 0 {
			time.Sleep(cfg.delayBetween)
		}
	}

	fmt.Println("nvprobe completed without an observable state change.")
}

func parseConfig() (config, error) {
	cfg := config{}
	var frameStyles string
	var serviceTags string
	var commandTags string
	var payloadTags string
	var inputTags string
	var specialTags string
	var serviceIDs string
	var commandIDs string
	var inputTypes string
	var specialInputs string
	var helloDevInfoTags string
	var helloCapabilityTags string
	var helloLayouts string
	var helloRemotePorts string
	var helloDeviceOSValues string
	var helloCapabilityValues string

	flag.StringVar(&cfg.host, "host", "", "NVIDIA service host/IP (defaults to saved target host)")
	flag.IntVar(&cfg.port, "port", 8987, "NVIDIA service port")
	flag.BoolVar(&cfg.monitor, "monitor", true, "poll Android TV power state after each attempt")
	flag.StringVar(&cfg.monitorHost, "monitor-host", "", "Android TV remote host/IP for power-state polling (defaults to saved target host)")
	flag.IntVar(&cfg.monitorPort, "monitor-port", 0, "Android TV remote monitor port (defaults to saved target/default 6466)")
	flag.StringVar(&cfg.certPath, "cert", "", "client certificate path for monitor polling")
	flag.StringVar(&cfg.keyPath, "key", "", "client key path for monitor polling")
	flag.StringVar(&cfg.mode, "mode", "special", "probe mode: special, empty, or hello")
	flag.StringVar(&frameStyles, "frame-styles", "varint", "frame styles to test: raw,varint,u32be,u32le")
	flag.StringVar(&serviceTags, "service-tags", "1", "protobuf field tags to try for serviceId")
	flag.StringVar(&commandTags, "command-tags", "2", "protobuf field tags to try for commandId")
	flag.StringVar(&payloadTags, "payload-tags", "3", "protobuf field tags to try for payload bytes")
	flag.StringVar(&inputTags, "input-tags", "1", "protobuf field tags to try for inputType inside the payload")
	flag.StringVar(&specialTags, "special-tags", "2", "protobuf field tags to try for specialInput inside the payload")
	flag.StringVar(&serviceIDs, "service-ids", "1-16", "serviceId values to try")
	flag.StringVar(&commandIDs, "command-ids", "0-1", "commandId values to try")
	flag.StringVar(&inputTypes, "input-types", "1-2", "virtual input inputType values to try")
	flag.StringVar(&specialInputs, "special-inputs", "1-8", "virtual input specialInput values to try")
	flag.StringVar(&helloDevInfoTags, "hello-devinfo-tags", "1-2", "nested handshake payload tags to try for devInfo")
	flag.StringVar(&helloCapabilityTags, "hello-capability-tags", "1-2", "nested handshake payload tags to try for capability")
	flag.StringVar(&helloLayouts, "hello-layouts", "os-name-package-android-host-port,name-os-package-android-host-port,name-package-android-os-host-port", "candidate field-tag layouts for the hello devInfo message")
	flag.StringVar(&cfg.helloName, "hello-name", "shrmt", "candidate client name to encode in hello devInfo")
	flag.StringVar(&cfg.helloPackageName, "hello-package-name", "com.nvidia.shield.remote", "candidate package name to encode in hello devInfo")
	flag.StringVar(&cfg.helloAndroidID, "hello-android-id", "shrmt", "candidate androidId/device identifier to encode in hello devInfo")
	flag.StringVar(&cfg.helloHostValue, "hello-host-value", "", "candidate local host/IP to encode in hello devInfo (defaults to detected local IP)")
	flag.BoolVar(&cfg.helloPreamble, "hello-preamble", false, "send a canonical hello message before each non-hello probe attempt")
	flag.StringVar(&helloRemotePorts, "hello-remote-ports", "0,8987", "candidate remotePort values to encode in hello devInfo")
	flag.StringVar(&helloDeviceOSValues, "hello-device-os-values", "0-2", "candidate deviceOs enum values to encode in hello devInfo")
	flag.StringVar(&helloCapabilityValues, "hello-capability-values", "0-4", "candidate capability enum values to encode in the hello payload")
	flag.DurationVar(&cfg.connectTimeout, "connect-timeout", 3*time.Second, "TLS connect timeout")
	flag.DurationVar(&cfg.responseWait, "response-wait", 250*time.Millisecond, "how long to wait for a server response after sending")
	flag.DurationVar(&cfg.pollWindow, "poll-window", 2*time.Second, "how long to poll Android TV power state after each send")
	flag.DurationVar(&cfg.pollInterval, "poll-interval", 250*time.Millisecond, "interval between power-state polls")
	flag.DurationVar(&cfg.delayBetween, "delay-between", 100*time.Millisecond, "extra delay between attempts")
	flag.IntVar(&cfg.startAt, "start-at", 0, "start at attempt index (0-based)")
	flag.IntVar(&cfg.limit, "limit", 0, "maximum number of attempts to run (0 = all)")
	flag.BoolVar(&cfg.stopOnChange, "stop-on-change", true, "stop immediately when the Android TV power state changes")
	flag.StringVar(&cfg.logFile, "log-file", "", "newline-delimited JSON log file path")
	flag.Parse()

	if err := populateDefaults(&cfg); err != nil {
		return config{}, err
	}
	if cfg.host == "" {
		return config{}, errors.New("host is required")
	}
	if cfg.monitorHost == "" {
		cfg.monitorHost = cfg.host
	}
	if cfg.monitorPort == 0 {
		cfg.monitorPort = intatv.DefaultRemotePort
	}
	if cfg.logFile == "" {
		cfg.logFile = filepath.Join(os.TempDir(), fmt.Sprintf("nvprobe-%s.ndjson", time.Now().Format("20060102-150405")))
	}
	cfg.mode = strings.ToLower(strings.TrimSpace(cfg.mode))
	if cfg.mode != "special" && cfg.mode != "empty" && cfg.mode != "hello" {
		return config{}, fmt.Errorf("unsupported mode %q (want special, empty, or hello)", cfg.mode)
	}
	if cfg.mode == "hello" && cfg.helloPreamble {
		return config{}, errors.New("hello-preamble cannot be used together with mode=hello")
	}

	var parseErr error
	cfg.frameStyles = parseFrameStyles(frameStyles)
	if len(cfg.frameStyles) == 0 {
		return config{}, errors.New("at least one frame style is required")
	}
	if cfg.serviceTags, parseErr = parseIntList(serviceTags); parseErr != nil {
		return config{}, fmt.Errorf("parse service-tags: %w", parseErr)
	}
	if cfg.commandTags, parseErr = parseIntList(commandTags); parseErr != nil {
		return config{}, fmt.Errorf("parse command-tags: %w", parseErr)
	}
	if cfg.payloadTags, parseErr = parseIntList(payloadTags); parseErr != nil {
		return config{}, fmt.Errorf("parse payload-tags: %w", parseErr)
	}
	if cfg.inputTags, parseErr = parseIntList(inputTags); parseErr != nil {
		return config{}, fmt.Errorf("parse input-tags: %w", parseErr)
	}
	if cfg.specialTags, parseErr = parseIntList(specialTags); parseErr != nil {
		return config{}, fmt.Errorf("parse special-tags: %w", parseErr)
	}
	if cfg.serviceIDs, parseErr = parseIntList(serviceIDs); parseErr != nil {
		return config{}, fmt.Errorf("parse service-ids: %w", parseErr)
	}
	if cfg.commandIDs, parseErr = parseIntList(commandIDs); parseErr != nil {
		return config{}, fmt.Errorf("parse command-ids: %w", parseErr)
	}
	if cfg.inputTypes, parseErr = parseIntList(inputTypes); parseErr != nil {
		return config{}, fmt.Errorf("parse input-types: %w", parseErr)
	}
	if cfg.specialInputs, parseErr = parseIntList(specialInputs); parseErr != nil {
		return config{}, fmt.Errorf("parse special-inputs: %w", parseErr)
	}
	if cfg.helloDevInfoTags, parseErr = parseIntList(helloDevInfoTags); parseErr != nil {
		return config{}, fmt.Errorf("parse hello-devinfo-tags: %w", parseErr)
	}
	if cfg.helloCapabilityTags, parseErr = parseIntList(helloCapabilityTags); parseErr != nil {
		return config{}, fmt.Errorf("parse hello-capability-tags: %w", parseErr)
	}
	cfg.helloLayouts = parseStringList(helloLayouts)
	if len(cfg.helloLayouts) == 0 {
		return config{}, errors.New("at least one hello layout is required")
	}
	if _, err := resolveHelloLayouts(cfg.helloLayouts); err != nil {
		return config{}, fmt.Errorf("parse hello-layouts: %w", err)
	}
	if cfg.helloRemotePorts, parseErr = parseIntList(helloRemotePorts); parseErr != nil {
		return config{}, fmt.Errorf("parse hello-remote-ports: %w", parseErr)
	}
	if cfg.helloDeviceOSValues, parseErr = parseIntList(helloDeviceOSValues); parseErr != nil {
		return config{}, fmt.Errorf("parse hello-device-os-values: %w", parseErr)
	}
	if cfg.helloCapabilityValues, parseErr = parseIntList(helloCapabilityValues); parseErr != nil {
		return config{}, fmt.Errorf("parse hello-capability-values: %w", parseErr)
	}
	return cfg, nil
}

func populateDefaults(cfg *config) error {
	ctx := context.Background()
	if cfg.host == "" || cfg.monitorHost == "" || cfg.monitorPort == 0 {
		target, err := xdgstore.NewTargetStore().Load(ctx)
		if err == nil {
			if cfg.host == "" {
				cfg.host = target.Host
			}
			if cfg.monitorHost == "" {
				cfg.monitorHost = target.Host
			}
			if cfg.monitorPort == 0 && target.Port != 0 {
				cfg.monitorPort = target.Port
			}
		}
	}
	if cfg.certPath == "" || cfg.keyPath == "" {
		creds, err := xdgstore.NewCredentialStore().Load(ctx)
		if err == nil {
			if cfg.certPath == "" {
				cfg.certPath = creds.CertPath
			}
			if cfg.keyPath == "" {
				cfg.keyPath = creds.KeyPath
			}
		}
	}
	return nil
}

func parseFrameStyles(raw string) []string {
	parts := strings.Split(raw, ",")
	styles := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		style := strings.ToLower(strings.TrimSpace(part))
		if style == "" {
			continue
		}
		switch style {
		case "raw", "varint", "u32be", "u32le":
			if _, ok := seen[style]; ok {
				continue
			}
			styles = append(styles, style)
			seen[style] = struct{}{}
		}
	}
	return styles
}

func parseStringList(raw string) []string {
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, ok := seen[part]; ok {
			continue
		}
		values = append(values, part)
		seen[part] = struct{}{}
	}
	return values
}

func parseIntList(raw string) ([]int, error) {
	parts := strings.Split(raw, ",")
	values := make([]int, 0, len(parts))
	seen := map[int]struct{}{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "-") {
			bounds := strings.SplitN(part, "-", 2)
			if len(bounds) != 2 {
				return nil, fmt.Errorf("invalid range %q", part)
			}
			start, err := strconv.Atoi(strings.TrimSpace(bounds[0]))
			if err != nil {
				return nil, fmt.Errorf("invalid range start %q: %w", bounds[0], err)
			}
			end, err := strconv.Atoi(strings.TrimSpace(bounds[1]))
			if err != nil {
				return nil, fmt.Errorf("invalid range end %q: %w", bounds[1], err)
			}
			if end < start {
				return nil, fmt.Errorf("descending range %q", part)
			}
			for value := start; value <= end; value++ {
				if _, ok := seen[value]; ok {
					continue
				}
				values = append(values, value)
				seen[value] = struct{}{}
			}
			continue
		}
		value, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("invalid integer %q: %w", part, err)
		}
		if _, ok := seen[value]; ok {
			continue
		}
		values = append(values, value)
		seen[value] = struct{}{}
	}
	sort.Ints(values)
	if len(values) == 0 {
		return nil, errors.New("empty list")
	}
	return values, nil
}

func countAttempts(cfg config) int {
	return len(buildAttempts(cfg))
}

func buildAttempts(cfg config) []attempt {
	var attempts []attempt
	switch cfg.mode {
	case "empty":
		attempts = buildEmptyAttempts(cfg)
	case "special":
		attempts = buildSpecialAttempts(cfg)
	case "hello":
		attempts = buildHelloAttempts(cfg)
	default:
		return nil
	}
	if cfg.helloPreamble {
		attempts = attachHelloPreambles(cfg, attempts)
	}
	return attempts
}

func buildEmptyAttempts(cfg config) []attempt {
	attempts := make([]attempt, 0)
	for _, frameStyle := range cfg.frameStyles {
		for _, serviceTag := range cfg.serviceTags {
			for _, commandTag := range cfg.commandTags {
				if commandTag == serviceTag {
					continue
				}
				for _, serviceID := range cfg.serviceIDs {
					for _, commandID := range cfg.commandIDs {
						attempts = append(attempts, attempt{
							FrameStyle: frameStyle,
							ServiceTag: serviceTag,
							CommandTag: commandTag,
							ServiceID:  serviceID,
							CommandID:  commandID,
						})
					}
				}
			}
		}
	}
	return attempts
}

func buildSpecialAttempts(cfg config) []attempt {
	attempts := make([]attempt, 0)
	for _, frameStyle := range cfg.frameStyles {
		for _, serviceTag := range cfg.serviceTags {
			for _, commandTag := range cfg.commandTags {
				if commandTag == serviceTag {
					continue
				}
				for _, serviceID := range cfg.serviceIDs {
					for _, commandID := range cfg.commandIDs {
						for _, payloadTag := range cfg.payloadTags {
							if payloadTag == serviceTag || payloadTag == commandTag {
								continue
							}
							for _, inputTag := range cfg.inputTags {
								for _, specialTag := range cfg.specialTags {
									if inputTag == specialTag {
										continue
									}
									for _, inputType := range cfg.inputTypes {
										for _, specialInput := range cfg.specialInputs {
											attempts = append(attempts, attempt{
												FrameStyle:   frameStyle,
												ServiceTag:   serviceTag,
												CommandTag:   commandTag,
												PayloadTag:   payloadTag,
												InputTag:     inputTag,
												SpecialTag:   specialTag,
												ServiceID:    serviceID,
												CommandID:    commandID,
												InputType:    inputType,
												SpecialInput: specialInput,
											})
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}
	return attempts
}

func buildMessage(a attempt) []byte {
	msg := make([]byte, 0, 32)
	msg = append(msg, fieldVarint(a.ServiceTag, a.ServiceID)...)
	msg = append(msg, fieldVarint(a.CommandTag, a.CommandID)...)
	if a.PayloadTag == 0 {
		return msg
	}
	if a.HasCustomPayload {
		msg = append(msg, fieldBytes(a.PayloadTag, a.PayloadBytes)...)
		return msg
	}
	payload := append(fieldVarint(a.InputTag, a.InputType), fieldVarint(a.SpecialTag, a.SpecialInput)...)
	msg = append(msg, fieldBytes(a.PayloadTag, payload)...)
	return msg
}

func applyFrame(style string, msg []byte) []byte {
	switch style {
	case "raw":
		return msg
	case "varint":
		return append(encodeVarint(len(msg)), msg...)
	case "u32be":
		return append([]byte{byte(len(msg) >> 24), byte(len(msg) >> 16), byte(len(msg) >> 8), byte(len(msg))}, msg...)
	case "u32le":
		return append([]byte{byte(len(msg)), byte(len(msg) >> 8), byte(len(msg) >> 16), byte(len(msg) >> 24)}, msg...)
	default:
		return msg
	}
}

func sendTLS(cfg config, payloads [][]byte) ([]byte, error) {
	dialer := &net.Dialer{Timeout: cfg.connectTimeout}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.connectTimeout)
	defer cancel()

	conn, err := tls.DialWithDialer(dialer, "tcp", net.JoinHostPort(cfg.host, strconv.Itoa(cfg.port)), &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         cfg.host,
	})
	if err != nil {
		return nil, fmt.Errorf("connect tls: %w", err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(cfg.connectTimeout + time.Duration(len(payloads))*cfg.responseWait)); err != nil {
		return nil, fmt.Errorf("set deadline: %w", err)
	}
	if err := conn.HandshakeContext(ctx); err != nil {
		return nil, fmt.Errorf("handshake: %w", err)
	}
	buf := make([]byte, 4096)
	for _, payload := range payloads {
		if _, err := conn.Write(payload); err != nil {
			return nil, fmt.Errorf("write payload: %w", err)
		}
		if cfg.responseWait <= 0 {
			continue
		}
		if err := conn.SetReadDeadline(time.Now().Add(cfg.responseWait)); err != nil {
			return nil, fmt.Errorf("set read deadline: %w", err)
		}
		n, err := conn.Read(buf)
		if err == nil {
			return append([]byte(nil), buf[:n]...), nil
		}
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			continue
		}
		if errors.Is(err, os.ErrDeadlineExceeded) {
			continue
		}
		return nil, fmt.Errorf("read response: %w", err)
	}
	return nil, nil
}

func pollForStateChange(cfg config, baseline powerSample) ([]powerSample, bool) {
	if !cfg.monitor || cfg.certPath == "" || cfg.keyPath == "" {
		return nil, false
	}
	samples := make([]powerSample, 0, 1+int(cfg.pollWindow/cfg.pollInterval))
	deadline := time.Now().Add(cfg.pollWindow)
	for {
		sample := samplePowerState(cfg)
		samples = append(samples, sample)
		if stateChanged(baseline, sample) {
			return samples, true
		}
		if time.Now().After(deadline) {
			return samples, false
		}
		time.Sleep(cfg.pollInterval)
	}
}

func samplePowerState(cfg config) powerSample {
	if !cfg.monitor || cfg.certPath == "" || cfg.keyPath == "" || cfg.monitorHost == "" {
		return powerSample{Error: "monitoring is not configured"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.connectTimeout)
	defer cancel()
	session, err := intatv.DialSession(ctx, intatv.SendKeyParams{
		Host:     cfg.monitorHost,
		Port:     cfg.monitorPort,
		CertPath: cfg.certPath,
		KeyPath:  cfg.keyPath,
	})
	if err != nil {
		return powerSample{Error: err.Error()}
	}
	defer session.Close()
	powered, hasPower := session.PowerState()
	return powerSample{Powered: powered, HasPower: hasPower}
}

func stateChanged(before, after powerSample) bool {
	if before.Error != after.Error {
		return true
	}
	if before.HasPower != after.HasPower {
		return true
	}
	if before.Powered != after.Powered {
		return true
	}
	return false
}

func lastSample(samples []powerSample) powerSample {
	if len(samples) == 0 {
		return powerSample{}
	}
	return samples[len(samples)-1]
}

func openLogFile(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	return f, nil
}

func encodeVarint(value int) []byte {
	if value < 0 {
		value = 0
	}
	out := make([]byte, 0, 10)
	for {
		b := byte(value & 0x7f)
		value >>= 7
		if value == 0 {
			out = append(out, b)
			return out
		}
		out = append(out, b|0x80)
	}
}

func fieldVarint(tag int, value int) []byte {
	out := make([]byte, 0, 12)
	out = append(out, encodeVarint(tag<<3)...)
	out = append(out, encodeVarint(value)...)
	return out
}

func fieldBytes(tag int, payload []byte) []byte {
	out := make([]byte, 0, len(payload)+12)
	out = append(out, encodeVarint((tag<<3)|2)...)
	out = append(out, encodeVarint(len(payload))...)
	out = append(out, payload...)
	return out
}
