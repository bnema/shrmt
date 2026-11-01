package beyonder

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	pb "shrmt/internal/beyonder/proto"
)

func unhex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestEncodeSpecialInputGolden(t *testing.T) {
	got, err := EncodeSpecialInput(pb.VirtualInputPayload_ACTION_SLEEP_NOW)
	if err != nil {
		t.Fatal(err)
	}
	if want := unhex(t, "08e907120808141005201e401e"); !bytes.Equal(got, want) {
		t.Fatalf("got %x want %x", got, want)
	}
}

// Hello golden. The Handshake payload was derived with:
//
//	echo 'command: COMMAND_HELLO name: "shrmt" device_os: DEVICEOS_ANDROID capability: 81915' \
//	  | protoc -I internal/beyonder/proto --encode=beyonder.Handshake handshake.proto | xxd -p
//	=> 080112057368726d74180128fbff04
//
// and wrapped with
//
//	echo 'service_id: SERVICEID_HANDSHAKE payload: "\010\001\022\005shrmt\030\001(\373\377\004"' \
//	  | protoc -I internal/beyonder/proto --encode=beyonder.BaseMessage base.proto | xxd -p
//	=> 0801120f080112057368726d74180128fbff04
func TestEncodeHelloGolden(t *testing.T) {
	got, err := EncodeHello("shrmt")
	if err != nil {
		t.Fatal(err)
	}
	if want := unhex(t, "0801120f080112057368726d74180128fbff04"); !bytes.Equal(got, want) {
		t.Fatalf("got %x want %x", got, want)
	}
	if _, err := EncodeHello(""); err == nil {
		t.Fatal("empty name must fail")
	}
}

func TestEncodePinGolden(t *testing.T) {
	got, _ := EncodePinRequest()
	if want := unhex(t, "080a120308cd08"); !bytes.Equal(got, want) {
		t.Fatalf("got %x want %x", got, want)
	}
	f, _ := EncodePinCode("1234", "dev")
	m, err := Decode(f)
	if err != nil || m.Authentication.Command != pb.AuthenticationServicePayload_COMMAND_PIN_CODE {
		t.Fatalf("%v %v", m, err)
	}
	pc := &pb.PairingPinCode{}
	if err := proto.Unmarshal(m.Authentication.Object, pc); err != nil || pc.PinCode != "1234" || pc.DeviceId != "dev" {
		t.Fatalf("%v %v", pc, err)
	}
}

func TestDecodeRoundTrips(t *testing.T) {
	f, _ := EncodeHello("shrmt")
	m, err := Decode(f)
	if err != nil || m.Handshake == nil || m.Handshake.Name != "shrmt" || m.Handshake.Capability != ClientCapability {
		t.Fatalf("%+v %v", m, err)
	}

	f, _ = EncodeRTTProbe(1234567)
	m, _ = Decode(f)
	if !m.IsRTT() || m.Base.TimeStamp != 1234567 || !strings.Contains(m.String(), "1234567") {
		t.Fatalf("%v", m)
	}

	lvl, _ := proto.Marshal(&pb.AuthorizationLevel{Level: pb.AuthorizationLevel_LEVEL_VIRTUAL_INPUT, DeviceName: "SHIELD", Capability: 0x400})
	a, _ := proto.Marshal(&pb.AuthenticationServicePayload{Command: pb.AuthenticationServicePayload_COMMAND_AUTHORIZATION_LEVEL, Object: lvl})
	f, _ = proto.Marshal(&pb.BaseMessage{ServiceId: pb.BaseMessage_SERVICEID_AUTHENTICATION, Payload: a})
	m, err = Decode(f)
	if err != nil || m.AuthLevel == nil || m.AuthLevel.DeviceName != "SHIELD" || !strings.Contains(m.String(), "SHIELD") {
		t.Fatalf("%+v %v", m, err)
	}

	pr, _ := proto.Marshal(&pb.PairingResult{ResultType: pb.PairingResult_RESULT_SUCCESS, PrivateKey: []byte("k"), Certificate: []byte("c")})
	a, _ = proto.Marshal(&pb.AuthenticationServicePayload{Command: pb.AuthenticationServicePayload_COMMAND_PAIRING_RESULT, Object: pr})
	f, _ = proto.Marshal(&pb.BaseMessage{ServiceId: pb.BaseMessage_SERVICEID_AUTHENTICATION, Payload: a})
	m, _ = Decode(f)
	if m.PairingResult == nil || m.PairingResult.ResultType != pb.PairingResult_RESULT_SUCCESS {
		t.Fatalf("%+v", m)
	}

	a, _ = proto.Marshal(&pb.AuthenticationServicePayload{Command: pb.AuthenticationServicePayload_COMMAND_PIN_CHALLENGE})
	f, _ = proto.Marshal(&pb.BaseMessage{ServiceId: pb.BaseMessage_SERVICEID_AUTHENTICATION, Payload: a})
	if m, _ = Decode(f); !m.PinChallenge {
		t.Fatal("pin challenge")
	}

	hi, _ := proto.Marshal(&pb.HostInformation{DeviceName: "tv", Port: 8987})
	h, _ := proto.Marshal(&pb.HostInfoServicePayload{Command: pb.HostInfoServicePayload_COMMAND_HOST_INFO_RESULT, Object: hi})
	f, _ = proto.Marshal(&pb.BaseMessage{ServiceId: pb.BaseMessage_SERVICEID_HOST_INFO, Payload: h})
	if m, _ = Decode(f); m.HostInfo == nil || m.HostInfo.Port != 8987 {
		t.Fatalf("%+v", m)
	}

	f, _ = proto.Marshal(&pb.BaseMessage{ServiceId: pb.BaseMessage_SERVICEID_AUDIO_INPUT, Payload: []byte{1, 2}})
	m, err = Decode(f)
	if err != nil || !bytes.Equal(m.Base.Payload, []byte{1, 2}) || !strings.Contains(m.String(), "0102") {
		t.Fatalf("%+v %v", m, err)
	}
}

func TestFramesAndReassembler(t *testing.T) {
	small, _ := EncodeHello("x")
	if fr := Frames(small); len(fr) != 1 {
		t.Fatalf("small frames: %d", len(fr))
	}

	big, _ := proto.Marshal(&pb.BaseMessage{
		ServiceId: pb.BaseMessage_SERVICEID_AUDIO_INPUT,
		Payload:   bytes.Repeat([]byte{0xAB}, 5000),
	})
	fr := Frames(big)
	if len(fr) != 2 {
		t.Fatalf("frames: %d", len(fr))
	}
	ann, _ := Decode(fr[0])
	if int(ann.Base.Size) != len(big) || !strings.HasPrefix(ann.String(), "SIZE") {
		t.Fatalf("announcement: %v", ann)
	}

	var r Reassembler
	// small message, announcement, then big split in chunks, then small again.
	if ms, err := r.Feed(small); err != nil || len(ms) != 1 {
		t.Fatalf("%v %v", ms, err)
	}
	if ms, err := r.Feed(fr[0]); err != nil || len(ms) != 0 {
		t.Fatalf("%v %v", ms, err)
	}
	var got []Message
	for rest := fr[1]; len(rest) > 0; {
		n := min(1500, len(rest))
		ms, err := r.Feed(rest[:n])
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, ms...)
		rest = rest[n:]
	}
	if len(got) != 1 || len(got[0].Base.Payload) != 5000 || got[0].ServiceID != pb.BaseMessage_SERVICEID_AUDIO_INPUT {
		t.Fatalf("got %d msgs", len(got))
	}
	if ms, _ := r.Feed(small); len(ms) != 1 {
		t.Fatal("reassembler did not return to idle")
	}

	// Announcement + message coalesced in one chunk, with trailing small msg.
	chunk := append(append(append([]byte{}, fr[0]...), fr[1]...), small...)
	r.Reset()
	ms, err := r.Feed(chunk)
	if err != nil || len(ms) != 2 {
		t.Fatalf("coalesced: %d %v", len(ms), err)
	}
}
