package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	xdgstore "shrmt/adapters/out/xdg"
	intatv "shrmt/internal/atvremote"

	pb "github.com/drosocode/atvremote/pkg/v2/proto"
)

type config struct {
	host           string
	port           int
	certPath       string
	keyPath        string
	mode           string
	withLong       bool
	settle         time.Duration
	pollWindow     time.Duration
	pollInterval   time.Duration
	delayBetween   time.Duration
	connectTimeout time.Duration
	startAt        int
	limit          int
	stopOnWake     bool
	logFile        string
}

type stateSample struct {
	Powered  bool   `json:"powered"`
	HasPower bool   `json:"has_power"`
	Error    string `json:"error,omitempty"`
}

type attemptLog struct {
	Index        int            `json:"index"`
	Total        int            `json:"total"`
	KeyCode      string         `json:"key_code"`
	Direction    string         `json:"direction"`
	StartedAt    time.Time      `json:"started_at"`
	EndedAt      time.Time      `json:"ended_at"`
	SendError    string         `json:"send_error,omitempty"`
	Before       stateSample    `json:"before"`
	After        stateSample    `json:"after"`
	PollSamples  []stateSample  `json:"poll_samples,omitempty"`
	WakeDetected bool           `json:"wake_detected"`
	Extra        map[string]any `json:"extra,omitempty"`
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

	candidates, err := candidateAttempts(cfg.mode, cfg.withLong)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if cfg.startAt > 0 {
		if cfg.startAt >= len(candidates) {
			fmt.Fprintf(os.Stderr, "start-at=%d is past the candidate list (%d)\n", cfg.startAt, len(candidates))
			os.Exit(1)
		}
		candidates = candidates[cfg.startAt:]
	}
	if cfg.limit > 0 && cfg.limit < len(candidates) {
		candidates = candidates[:cfg.limit]
	}

	fmt.Printf("Wake probe starting: host=%s port=%d mode=%s candidates=%d withLong=%t\n", cfg.host, cfg.port, cfg.mode, len(candidates), cfg.withLong)
	fmt.Printf("Log file: %s\n", cfg.logFile)

	for idx, attempt := range candidates {
		before := samplePowerState(cfg)
		fmt.Printf("[%03d/%03d] %s / %s ... ", idx+1, len(candidates), attempt.keyCode.String(), attempt.direction.String())

		startedAt := time.Now()
		sendErr := sendAttempt(cfg, attempt.keyCode, attempt.direction)
		polls, after, woke := pollForWake(cfg)
		endedAt := time.Now()

		entry := attemptLog{
			Index:        idx + 1,
			Total:        len(candidates),
			KeyCode:      attempt.keyCode.String(),
			Direction:    attempt.direction.String(),
			StartedAt:    startedAt,
			EndedAt:      endedAt,
			Before:       before,
			After:        after,
			PollSamples:  polls,
			WakeDetected: woke,
		}
		if sendErr != nil {
			entry.SendError = sendErr.Error()
		}
		if err := enc.Encode(entry); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write log entry: %v\n", err)
		}

		switch {
		case sendErr != nil:
			fmt.Printf("send-error: %v\n", sendErr)
		case woke:
			fmt.Printf("WAKE DETECTED (hasPower=%t powered=%t)\n", after.HasPower, after.Powered)
			if cfg.stopOnWake {
				fmt.Println("Stopping after successful wake detection.")
				return
			}
		default:
			fmt.Printf("no change (hasPower=%t powered=%t)\n", after.HasPower, after.Powered)
		}

		if cfg.delayBetween > 0 {
			time.Sleep(cfg.delayBetween)
		}
	}

	fmt.Println("Wake probe completed without detecting powered=true.")
}

type keyAttempt struct {
	keyCode   pb.RemoteKeyCode
	direction pb.RemoteDirection
}

func parseConfig() (config, error) {
	cfg := config{}
	flag.StringVar(&cfg.host, "host", "", "SHIELD host/IP (defaults to saved target)")
	flag.IntVar(&cfg.port, "port", 0, "remote port (defaults to saved/default port)")
	flag.StringVar(&cfg.certPath, "cert", "", "client certificate path (defaults to saved credential path)")
	flag.StringVar(&cfg.keyPath, "key", "", "client private key path (defaults to saved credential path)")
	flag.StringVar(&cfg.mode, "mode", "likely", "candidate mode: likely or full")
	flag.BoolVar(&cfg.withLong, "with-long", false, "also test START_LONG and END_LONG directions")
	flag.DurationVar(&cfg.settle, "settle", 750*time.Millisecond, "wait after sending a key before polling")
	flag.DurationVar(&cfg.pollWindow, "poll-window", 3*time.Second, "how long to poll power state after each attempt")
	flag.DurationVar(&cfg.pollInterval, "poll-interval", 250*time.Millisecond, "interval between power-state polls")
	flag.DurationVar(&cfg.delayBetween, "delay-between", 500*time.Millisecond, "extra delay between attempts")
	flag.DurationVar(&cfg.connectTimeout, "connect-timeout", 5*time.Second, "session connection timeout")
	flag.IntVar(&cfg.startAt, "start-at", 0, "start at candidate index (0-based, after mode expansion)")
	flag.IntVar(&cfg.limit, "limit", 0, "maximum number of attempts to run (0 = all)")
	flag.BoolVar(&cfg.stopOnWake, "stop-on-wake", true, "stop immediately when powered=true is observed")
	flag.StringVar(&cfg.logFile, "log-file", "", "newline-delimited JSON log file path")
	flag.Parse()

	if err := populateDefaults(&cfg); err != nil {
		return config{}, err
	}
	if cfg.host == "" {
		return config{}, errors.New("host is required")
	}
	if cfg.certPath == "" || cfg.keyPath == "" {
		return config{}, errors.New("cert and key paths are required")
	}
	if cfg.port == 0 {
		cfg.port = intatv.DefaultRemotePort
	}
	if cfg.logFile == "" {
		cfg.logFile = filepath.Join(os.TempDir(), fmt.Sprintf("wakeprobe-%s.ndjson", time.Now().Format("20060102-150405")))
	}
	return cfg, nil
}

func populateDefaults(cfg *config) error {
	ctx := context.Background()

	if cfg.host == "" || cfg.port == 0 {
		target, err := xdgstore.NewTargetStore().Load(ctx)
		if err == nil {
			if cfg.host == "" {
				cfg.host = target.Host
			}
			if cfg.port == 0 {
				cfg.port = target.Port
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

func candidateAttempts(mode string, withLong bool) ([]keyAttempt, error) {
	var codes []pb.RemoteKeyCode
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "likely":
		codes = []pb.RemoteKeyCode{
			pb.RemoteKeyCode_KEYCODE_WAKEUP,
			pb.RemoteKeyCode_KEYCODE_POWER,
			pb.RemoteKeyCode_KEYCODE_TV_POWER,
			pb.RemoteKeyCode_KEYCODE_STB_POWER,
			pb.RemoteKeyCode_KEYCODE_AVR_POWER,
			pb.RemoteKeyCode_KEYCODE_HOME,
			pb.RemoteKeyCode_KEYCODE_SLEEP,
			pb.RemoteKeyCode_KEYCODE_SOFT_SLEEP,
			pb.RemoteKeyCode_KEYCODE_ALL_APPS,
			pb.RemoteKeyCode_KEYCODE_SYSTEM_NAVIGATION_UP,
		}
	case "full":
		codes = allKnownKeyCodes()
	default:
		return nil, fmt.Errorf("unsupported mode %q (want likely or full)", mode)
	}

	attempts := make([]keyAttempt, 0, len(codes)*3)
	for _, code := range codes {
		attempts = append(attempts, keyAttempt{keyCode: code, direction: pb.RemoteDirection_SHORT})
		if withLong {
			attempts = append(attempts,
				keyAttempt{keyCode: code, direction: pb.RemoteDirection_START_LONG},
				keyAttempt{keyCode: code, direction: pb.RemoteDirection_END_LONG},
			)
		}
	}
	return attempts, nil
}

func allKnownKeyCodes() []pb.RemoteKeyCode {
	values := make([]int, 0, len(pb.RemoteKeyCode_name))
	for v := range pb.RemoteKeyCode_name {
		if v == 0 {
			continue
		}
		values = append(values, int(v))
	}
	sort.Ints(values)
	codes := make([]pb.RemoteKeyCode, 0, len(values))
	for _, v := range values {
		codes = append(codes, pb.RemoteKeyCode(v))
	}
	return codes
}

func sendAttempt(cfg config, keyCode pb.RemoteKeyCode, direction pb.RemoteDirection) error {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.connectTimeout)
	defer cancel()

	session, err := intatv.DialSession(ctx, intatv.SendKeyParams{
		Host:     cfg.host,
		Port:     cfg.port,
		CertPath: cfg.certPath,
		KeyPath:  cfg.keyPath,
	})
	if err != nil {
		return err
	}
	defer session.Close()

	sendCtx, sendCancel := context.WithTimeout(context.Background(), cfg.connectTimeout)
	defer sendCancel()
	_, err = session.SendKeyCode(sendCtx, keyCode, direction)
	if cfg.settle > 0 {
		time.Sleep(cfg.settle)
	}
	return err
}

func pollForWake(cfg config) ([]stateSample, stateSample, bool) {
	samples := make([]stateSample, 0, 1+int(cfg.pollWindow/cfg.pollInterval))
	deadline := time.Now().Add(cfg.pollWindow)
	for {
		sample := samplePowerState(cfg)
		samples = append(samples, sample)
		if sample.HasPower && sample.Powered {
			return samples, sample, true
		}
		if time.Now().After(deadline) {
			return samples, sample, false
		}
		time.Sleep(cfg.pollInterval)
	}
}

func samplePowerState(cfg config) stateSample {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.connectTimeout)
	defer cancel()

	session, err := intatv.DialSession(ctx, intatv.SendKeyParams{
		Host:     cfg.host,
		Port:     cfg.port,
		CertPath: cfg.certPath,
		KeyPath:  cfg.keyPath,
	})
	if err != nil {
		return stateSample{Error: err.Error()}
	}
	defer session.Close()

	powered, hasPower := session.PowerState()
	return stateSample{Powered: powered, HasPower: hasPower}
}
