package main

import (
	"reflect"
	"testing"

	pb "shrmt/internal/beyonder/proto"
)

func TestCapabilityNames(t *testing.T) {
	got := capabilityNames(1 | 1024 | 1<<20)
	want := []string{"INPUT_GENERIC", "RTT_MESSAGING", "UNKNOWN(0x100000)"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if len(capabilityNames(0)) != 0 {
		t.Fatal("expected none")
	}
}

func TestParseSpecialName(t *testing.T) {
	v, err := parseSpecialName("Sleep")
	if err != nil || v != pb.VirtualInputPayload_ACTION_SLEEP_NOW {
		t.Fatalf("got %v %v", v, err)
	}
	if _, err := parseSpecialName("nope"); err == nil {
		t.Fatal("expected error")
	}
}
