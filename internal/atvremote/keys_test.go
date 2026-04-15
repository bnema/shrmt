package atvremote

import "testing"

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
	if _, err := ResolveKeyCode("definitely-not-a-key"); err == nil {
		t.Fatal("ResolveKeyCode should fail for unknown action")
	}
}
