package pairing_test

import (
	"errors"
	"testing"

	"shrmt/core/pairing"
)

func TestParseCodeNormalizesLowercase(t *testing.T) {
	code, err := pairing.ParseCode("a1b2c3")
	if err != nil {
		t.Fatalf("ParseCode returned error: %v", err)
	}
	if got := code.String(); got != "A1B2C3" {
		t.Fatalf("code = %q, want %q", got, "A1B2C3")
	}
}

func TestParseCodeRejectsInvalidLength(t *testing.T) {
	_, err := pairing.ParseCode("abc")
	if !errors.Is(err, pairing.ErrInvalidCode) {
		t.Fatalf("expected ErrInvalidCode, got %v", err)
	}
}

func TestParseCodeRejectsNonHex(t *testing.T) {
	_, err := pairing.ParseCode("zzzzzz")
	if !errors.Is(err, pairing.ErrInvalidCode) {
		t.Fatalf("expected ErrInvalidCode, got %v", err)
	}
}
