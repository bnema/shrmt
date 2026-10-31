package atvremote

import (
	"testing"

	pb "github.com/drosocode/atvremote/pkg/v2/proto"
)

func TestResolveKeyCode(t *testing.T) {
	t.Parallel()

	if _, err := ResolveKeyCode("home"); err != nil {
		t.Fatalf("ResolveKeyCode(home) returned error: %v", err)
	}
	if _, err := ResolveKeyCode("power"); err != nil {
		t.Fatalf("ResolveKeyCode(power) returned error: %v", err)
	}
	if _, err := ResolveKeyCode("volume-up"); err != nil {
		t.Fatalf("ResolveKeyCode(volume-up) returned error: %v", err)
	}
	if _, err := ResolveKeyCode("wake-up"); err != nil {
		t.Fatalf("ResolveKeyCode(wake-up) returned error: %v", err)
	}
	for name, want := range map[string]pb.RemoteKeyCode{
		"menu":         pb.RemoteKeyCode_KEYCODE_MENU,
		"rewind":       pb.RemoteKeyCode_KEYCODE_MEDIA_REWIND,
		"rew":          pb.RemoteKeyCode_KEYCODE_MEDIA_REWIND,
		"fast-forward": pb.RemoteKeyCode_KEYCODE_MEDIA_FAST_FORWARD,
		"ff":           pb.RemoteKeyCode_KEYCODE_MEDIA_FAST_FORWARD,
	} {
		got, err := ResolveKeyCode(name)
		if err != nil || got != want {
			t.Fatalf("ResolveKeyCode(%s) = %v, %v; want %v", name, got, err, want)
		}
	}
	if _, err := ResolveKeyCode("definitely-not-a-key"); err == nil {
		t.Fatal("ResolveKeyCode should fail for unknown action")
	}
}
