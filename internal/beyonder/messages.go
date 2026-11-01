// Package beyonder implements the pure (no I/O) encoding layer of the NVIDIA
// SHIELD "beyonder" remote protocol. See research/beyonder-protocol.md.
package beyonder

import (
	"errors"
	"fmt"
	"strings"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"

	pb "shrmt/internal/beyonder/proto"
)

const (
	// ClientCapability is CLIENT_ALL_CAPABILITY as sent by the official app.
	ClientCapability int32 = 0x13FFB
	// MaxUnframedSize is the largest message sent without a size announcement.
	MaxUnframedSize = 2048
)

func base(svc pb.BaseMessage_ServiceId, payload proto.Message) ([]byte, error) {
	p, err := proto.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return proto.Marshal(&pb.BaseMessage{ServiceId: svc, Payload: p})
}

func auth(cmd pb.AuthenticationServicePayload_CommandType, obj proto.Message) ([]byte, error) {
	a := &pb.AuthenticationServicePayload{Command: cmd}
	if obj != nil {
		o, err := proto.Marshal(obj)
		if err != nil {
			return nil, err
		}
		a.Object = o
	}
	return base(pb.BaseMessage_SERVICEID_AUTHENTICATION, a)
}

// EncodeHello builds the client HELLO handshake frame. name must be non-empty.
func EncodeHello(name string) ([]byte, error) {
	if name == "" {
		return nil, errors.New("beyonder: hello name must not be empty")
	}
	return base(pb.BaseMessage_SERVICEID_HANDSHAKE, &pb.Handshake{
		Command:    pb.Handshake_COMMAND_HELLO,
		Name:       name,
		DeviceOs:   pb.Handshake_DEVICEOS_ANDROID,
		Capability: ClientCapability,
	})
}

// EncodePinRequest builds the COMMAND_PIN_REQUEST frame.
func EncodePinRequest() ([]byte, error) {
	return auth(pb.AuthenticationServicePayload_COMMAND_PIN_REQUEST, nil)
}

// EncodePinCode builds the COMMAND_PIN_CODE frame.
func EncodePinCode(pin, deviceID string) ([]byte, error) {
	return auth(pb.AuthenticationServicePayload_COMMAND_PIN_CODE,
		&pb.PairingPinCode{PinCode: pin, DeviceId: deviceID})
}

// EncodeSpecialInput builds a VirtualInput SPECIAL command frame.
func EncodeSpecialInput(v pb.VirtualInputPayload_SpecialInput) ([]byte, error) {
	return base(pb.BaseMessage_SERVICEID_VIRTUAL_INPUT, &pb.VirtualInputPayload{
		Request:       pb.VirtualInputPayload_REQUEST_COMMAND,
		AccessoryType: pb.VirtualInputPayload_ACCESSORY_CONTROLLER,
		InputType:     pb.VirtualInputPayload_INPUT_SPECIAL,
		SpecialInput:  v,
	})
}

// EncodeLaunch builds a REMOTE_LAUNCHER request for an Android package.
func EncodeLaunch(pkg string) ([]byte, error) {
	object, err := proto.Marshal(&pb.LaunchRequest{PackageName: pkg})
	if err != nil {
		return nil, err
	}
	return base(pb.BaseMessage_SERVICEID_REMOTE_LAUNCHER, &pb.RemoteLauncherServicePayload{
		Command: pb.RemoteLauncherServicePayload_REQUEST_LAUNCH, Object: object,
	})
}

// EncodeRTTProbe builds an RTT probe: BaseMessage{time_stamp=nowMs}.
func EncodeRTTProbe(nowMs uint64) ([]byte, error) {
	return proto.Marshal(&pb.BaseMessage{TimeStamp: nowMs})
}

// Message is a decoded frame. At most one typed payload field is set; for
// unknown services (or undecodable-by-design ones) only Base.Payload is kept.
type Message struct {
	ServiceID pb.BaseMessage_ServiceId
	Base      *pb.BaseMessage

	Handshake *pb.Handshake

	// Authentication holds the envelope; exactly one of the following is set
	// depending on its command.
	Authentication *pb.AuthenticationServicePayload
	AuthLevel      *pb.AuthorizationLevel
	PairingResult  *pb.PairingResult
	PinChallenge   bool // COMMAND_PIN_CHALLENGE received (no object)

	Launcher     *pb.RemoteLauncherServicePayload
	LaunchResult *pb.LaunchResponse

	HostInfoPayload *pb.HostInfoServicePayload
	HostInfo        *pb.HostInformation
}

// IsRTT reports whether the frame is an RTT message (carries a timestamp).
func (m Message) IsRTT() bool { return m.Base != nil && m.Base.TimeStamp != 0 }

// Decode parses a single BaseMessage frame and its payload when known.
func Decode(frame []byte) (Message, error) {
	b := &pb.BaseMessage{}
	if err := proto.Unmarshal(frame, b); err != nil {
		return Message{}, fmt.Errorf("beyonder: decode base: %w", err)
	}
	m := Message{ServiceID: b.ServiceId, Base: b}
	var err error
	switch b.ServiceId {
	case pb.BaseMessage_SERVICEID_HANDSHAKE:
		m.Handshake = &pb.Handshake{}
		err = proto.Unmarshal(b.Payload, m.Handshake)
	case pb.BaseMessage_SERVICEID_AUTHENTICATION:
		err = m.decodeAuth(b.Payload)
	case pb.BaseMessage_SERVICEID_REMOTE_LAUNCHER:
		m.Launcher = &pb.RemoteLauncherServicePayload{}
		if err = proto.Unmarshal(b.Payload, m.Launcher); err == nil && m.Launcher.Command == pb.RemoteLauncherServicePayload_RESPONSE_LAUNCH {
			m.LaunchResult = &pb.LaunchResponse{}
			err = proto.Unmarshal(m.Launcher.Object, m.LaunchResult)
		}
	case pb.BaseMessage_SERVICEID_HOST_INFO:
		m.HostInfoPayload = &pb.HostInfoServicePayload{}
		if err = proto.Unmarshal(b.Payload, m.HostInfoPayload); err == nil &&
			m.HostInfoPayload.Command == pb.HostInfoServicePayload_COMMAND_HOST_INFO_RESULT &&
			len(m.HostInfoPayload.Object) > 0 {
			m.HostInfo = &pb.HostInformation{}
			err = proto.Unmarshal(m.HostInfoPayload.Object, m.HostInfo)
		}
	}
	if err != nil {
		return Message{}, fmt.Errorf("beyonder: decode service %v payload: %w", b.ServiceId, err)
	}
	return m, nil
}

func (m *Message) decodeAuth(payload []byte) error {
	a := &pb.AuthenticationServicePayload{}
	if err := proto.Unmarshal(payload, a); err != nil {
		return err
	}
	m.Authentication = a
	switch a.Command {
	case pb.AuthenticationServicePayload_COMMAND_AUTHORIZATION_LEVEL:
		m.AuthLevel = &pb.AuthorizationLevel{}
		return proto.Unmarshal(a.Object, m.AuthLevel)
	case pb.AuthenticationServicePayload_COMMAND_PAIRING_RESULT:
		m.PairingResult = &pb.PairingResult{}
		return proto.Unmarshal(a.Object, m.PairingResult)
	case pb.AuthenticationServicePayload_COMMAND_PIN_CHALLENGE:
		m.PinChallenge = true
	}
	return nil
}

// String renders the message for humans. Secrets (private key) are elided.
func (m Message) String() string {
	var sb strings.Builder
	if m.Base == nil {
		return "<empty>"
	}
	switch {
	case m.IsRTT():
		fmt.Fprintf(&sb, "RTT timestamp=%d", m.Base.TimeStamp)
		return sb.String()
	case m.Base.Size > 0 && len(m.Base.Payload) == 0:
		fmt.Fprintf(&sb, "SIZE %d", m.Base.Size)
		return sb.String()
	}
	fmt.Fprintf(&sb, "%v", m.ServiceID)
	switch {
	case m.Handshake != nil:
		fmt.Fprintf(&sb, " %v name=%q os=%v capability=0x%X",
			m.Handshake.Command, m.Handshake.Name, m.Handshake.DeviceOs, m.Handshake.Capability)
	case m.AuthLevel != nil:
		fmt.Fprintf(&sb, " %v level=%v device=%q capability=0x%X", m.Authentication.Command,
			m.AuthLevel.Level, m.AuthLevel.DeviceName, m.AuthLevel.Capability)
	case m.PairingResult != nil:
		fmt.Fprintf(&sb, " %v result=%v %q key=%dB cert=%dB", m.Authentication.Command,
			m.PairingResult.ResultType, m.PairingResult.ResultString,
			len(m.PairingResult.PrivateKey), len(m.PairingResult.Certificate))
	case m.Authentication != nil:
		fmt.Fprintf(&sb, " %v object=%x", m.Authentication.Command, m.Authentication.Object)
	case m.LaunchResult != nil:
		fmt.Fprintf(&sb, " %v result=%v", m.Launcher.Command, m.LaunchResult.Result)
	case m.Launcher != nil:
		fmt.Fprintf(&sb, " %v object=%x", m.Launcher.Command, m.Launcher.Object)
	case m.HostInfo != nil:
		fmt.Fprintf(&sb, " %v name=%q ip=%q port=%d androidId=%q abis=%v", m.HostInfoPayload.Command,
			m.HostInfo.DeviceName, m.HostInfo.Ip, m.HostInfo.Port, m.HostInfo.AndroidId, m.HostInfo.SupportedAbis)
	case m.HostInfoPayload != nil:
		fmt.Fprintf(&sb, " %v object=%x", m.HostInfoPayload.Command, m.HostInfoPayload.Object)
	default:
		fmt.Fprintf(&sb, " payload=%x", m.Base.Payload)
	}
	return sb.String()
}

// Frames returns the writes needed to send an encoded message: the message
// alone if <= MaxUnframedSize, else a size announcement followed by it. Each
// element must be written to the TLS stream as a single write.
func Frames(msg []byte) [][]byte {
	if len(msg) <= MaxUnframedSize {
		return [][]byte{msg}
	}
	// Marshal of a plain int32 field cannot fail.
	ann, _ := proto.Marshal(&pb.BaseMessage{Size: int32(len(msg))})
	return [][]byte{ann, msg}
}

// Reassembler turns received chunks (one TLS read each) into messages,
// handling size announcements. It is not safe for concurrent use.
type Reassembler struct {
	want int
	buf  []byte
}

// Feed consumes one received chunk. Size announcements are consumed and not
// returned. While a framed message is incomplete, nothing is returned. If a
// chunk completes a framed message and carries extra bytes, the extra bytes
// are fed again as a new chunk.
func (r *Reassembler) Feed(chunk []byte) ([]Message, error) {
	var out []Message
	for len(chunk) > 0 {
		if r.want > 0 {
			need := r.want - len(r.buf)
			take := min(need, len(chunk))
			r.buf = append(r.buf, chunk[:take]...)
			chunk = chunk[take:]
			if len(r.buf) < r.want {
				return out, nil
			}
			frame := r.buf
			r.want, r.buf = 0, nil
			m, err := Decode(frame)
			if err != nil {
				return out, err
			}
			out = append(out, m)
			continue
		}
		// A size announcement is exactly tag 0x20 (field 4, varint) + varint.
		// Detect it by prefix so a coalesced chunk can be split after it.
		if chunk[0] == 0x20 {
			if n, l := protowire.ConsumeVarint(chunk[1:]); l > 0 && n > 0 && n < 1<<30 {
				r.want = int(n)
				r.buf = make([]byte, 0, r.want)
				chunk = chunk[1+l:]
				continue
			}
		}
		m, err := Decode(chunk)
		if err != nil {
			return out, err
		}
		chunk = nil
		out = append(out, m)
	}
	return out, nil
}

// Reset drops any partially received framed message.
func (r *Reassembler) Reset() { r.want, r.buf = 0, nil }
