package main

import (
	"fmt"
	"os"
	"strings"

	cliadapter "shrmt/adapters/in/cli"
	gtkadapter "shrmt/adapters/in/gtk"
	androidtv "shrmt/adapters/out/androidtv"
	xdgstore "shrmt/adapters/out/xdg"
	zeroconf "shrmt/adapters/out/zeroconf"
	"shrmt/controller"
	"shrmt/core/device"
	"shrmt/core/pairing"
	"shrmt/core/remote"
)

const (
	clientName  = "shrmt"
	serviceName = androidtv.DefaultServiceName
)

func main() {
	ctrl := buildController()
	if shouldRunCLI(os.Args[1:]) {
		root := cliadapter.NewRoot(ctrl)
		if err := root.Execute(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	os.Exit(gtkadapter.Run(ctrl))
}

func buildController() *controller.Controller {
	discoverer := zeroconf.New("local")
	targetStore := xdgstore.NewTargetStore()
	deviceService := device.NewService(discoverer, targetStore, androidtv.DefaultRemotePort)
	credentialStore := xdgstore.NewCredentialStore()
	pairingService := pairing.NewService(androidtv.NewPairer(), credentialStore, clientName, serviceName)
	remoteService := remote.NewService(deviceService, pairingService, androidtv.NewSender())
	return controller.New(deviceService, pairingService, remoteService)
}

func shouldRunCLI(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch strings.ToLower(args[0]) {
	case "discover", "pair", "key", "power", "help", "completion":
		return true
	default:
		return strings.HasPrefix(args[0], "-")
	}
}
