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

func TestNormalizeAppLaunchTarget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "package name becomes market launch URI",
			raw:  "com.google.android.youtube.tv",
			want: "market://launch?id=com.google.android.youtube.tv",
		},
		{
			name: "https link stays unchanged",
			raw:  "https://www.youtube.com",
			want: "https://www.youtube.com",
		},
		{
			name: "custom scheme stays unchanged",
			raw:  "plex://",
			want: "plex://",
		},
		{
			name: "trims surrounding whitespace",
			raw:  "  tv.twitch.android.app  ",
			want: "market://launch?id=tv.twitch.android.app",
		},
		{
			name: "protocol-relative URL stays unchanged",
			raw:  "//www.youtube.com/watch?v=example",
			want: "//www.youtube.com/watch?v=example",
		},
		{
			name: "slash path stays unchanged",
			raw:  "www.youtube.com/watch?v=example",
			want: "www.youtube.com/watch?v=example",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := normalizeAppLaunchTarget(tt.raw); got != tt.want {
				t.Fatalf("normalizeAppLaunchTarget(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}
