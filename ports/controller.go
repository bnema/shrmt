package ports

import (
	"context"

	"shrmt/core/action"
	"shrmt/core/device"
	"shrmt/core/pairing"
	"shrmt/core/remote"
)

type LoadRequest struct {
	Target *device.Target
}

type SelectTargetRequest struct {
	Target device.Target
}

type PairRequest struct {
	Target       *device.Target
	Code         pairing.Code
	CodeProvider pairing.CodeProvider
}

type SendRequest struct {
	Target *device.Target
	Action action.Action
}

type Controller interface {
	Load(ctx context.Context, req LoadRequest) (remote.State, error)
	Discover(ctx context.Context) ([]device.Device, error)
	SelectTarget(ctx context.Context, req SelectTargetRequest) error
	Pair(ctx context.Context, req PairRequest) (pairing.State, error)
	Send(ctx context.Context, req SendRequest) (remote.SendResult, error)
}
