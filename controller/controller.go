package controller

import (
	"context"

	"shrmt/core/device"
	"shrmt/core/pairing"
	"shrmt/core/remote"
	"shrmt/ports"
)

type Controller struct {
	devices *device.Service
	pairing *pairing.Service
	remote  *remote.Service
}

func New(devices *device.Service, pairingSvc *pairing.Service, remoteSvc *remote.Service) *Controller {
	return &Controller{devices: devices, pairing: pairingSvc, remote: remoteSvc}
}

func (c *Controller) Load(ctx context.Context, req ports.LoadRequest) (remote.State, error) {
	return c.remote.Load(ctx, req.Target)
}

func (c *Controller) Discover(ctx context.Context) ([]device.Device, error) {
	return c.devices.Discover(ctx)
}

func (c *Controller) SelectTarget(ctx context.Context, req ports.SelectTargetRequest) error {
	return c.devices.SaveDefault(ctx, req.Target)
}

func (c *Controller) Pair(ctx context.Context, req ports.PairRequest) (pairing.State, error) {
	target, err := c.devices.Resolve(ctx, req.Target)
	if err != nil {
		return pairing.State{}, err
	}
	state, err := c.pairing.Pair(ctx, target, req.Code, req.CodeProvider)
	if err != nil {
		return pairing.State{}, err
	}
	_ = c.devices.SaveDefault(ctx, target)
	return state, nil
}

func (c *Controller) Send(ctx context.Context, req ports.SendRequest) (remote.SendResult, error) {
	return c.remote.Send(ctx, remote.SendInput{Target: req.Target, Action: req.Action})
}
