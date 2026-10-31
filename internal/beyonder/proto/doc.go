// Package beyonderpb contains Go types generated from the reconstructed
// beyonder (NVIDIA SHIELD remote) protobuf schemas.
package beyonderpb

//go:generate protoc --proto_path=../../.. --go_out=../../.. --go_opt=module=shrmt internal/beyonder/proto/base.proto internal/beyonder/proto/handshake.proto internal/beyonder/proto/authentication.proto internal/beyonder/proto/host_info.proto internal/beyonder/proto/capability.proto internal/beyonder/proto/virtual_input.proto
