package atvremote

import (
	"testing"
	"time"
)

func TestNormalizeSessionParamsDefaults(t *testing.T) {
	t.Parallel()

	params := normalizeSessionParams(SendKeyParams{})

	if params.Port != DefaultRemotePort {
		t.Fatalf("Port = %d, want %d", params.Port, DefaultRemotePort)
	}
	if params.ReadyTimeout != 5*time.Second {
		t.Fatalf("ReadyTimeout = %s, want 5s", params.ReadyTimeout)
	}
	if params.PostDelay != 300*time.Millisecond {
		t.Fatalf("PostDelay = %s, want 300ms", params.PostDelay)
	}
}
