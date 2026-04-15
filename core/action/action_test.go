package action_test

import (
	"errors"
	"testing"

	"shrmt/core/action"
)

func TestParseCanonicalizesAliases(t *testing.T) {
	tests := map[string]action.Action{
		"enter":      action.Enter,
		"ok":         action.Enter,
		"center":     action.Enter,
		"vol-up":     action.VolumeUp,
		"VOL_DOWN":   action.VolumeDown,
		"play pause": action.PlayPause,
	}
	for input, want := range tests {
		got, err := action.Parse(input)
		if err != nil {
			t.Fatalf("Parse(%q) returned error: %v", input, err)
		}
		if got != want {
			t.Fatalf("Parse(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestParseUnknownAction(t *testing.T) {
	_, err := action.Parse("banana")
	if !errors.Is(err, action.ErrUnknownAction) {
		t.Fatalf("expected ErrUnknownAction, got %v", err)
	}
}
