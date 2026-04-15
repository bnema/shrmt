package gtk

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"shrmt/core/action"
	"shrmt/core/device"
	"shrmt/core/pairing"
	"shrmt/core/remote"
	"shrmt/ports"

	"github.com/bnema/puregotk/v4/gdk"
	"github.com/bnema/puregotk/v4/gio"
	"github.com/bnema/puregotk/v4/glib"
	pgtk "github.com/bnema/puregotk/v4/gtk"
	"github.com/bnema/puregotk/v4/layershell"
)

const (
	appID                = "dev.bnema.shrmt"
	windowTitle          = "shrmt"
	layerShellNamespace  = appID + ".remote"
	fallbackWindowWidth  = 320
	fallbackWindowHeight = 640
)

var appShortcuts = []struct {
	Label string
	Link  string
}{
	{Label: "YouTube", Link: "https://www.youtube.com/"},
	{Label: "Twitch", Link: "twitch://open"},
	{Label: "Plex", Link: "https://app.plex.tv/"},
}

type App struct {
	controller ports.Controller
	app        *pgtk.Application
	window     *pgtk.ApplicationWindow

	hostEntry     *pgtk.Entry
	pairEntry     *pgtk.Entry
	targetSection *pgtk.Box
	pairSection   *pgtk.Box
	statusLabel   *pgtk.Label
	targetLabel   *pgtk.Label

	actionButtons    []*pgtk.Button
	actionQueue      chan action.Action
	actionQueueDepth atomic.Int64
	callbacks        []interface{}
}

func Run(ctrl ports.Controller) int {
	prepareRuntime()
	id := appID
	app := pgtk.NewApplication(&id, gio.GApplicationDefaultFlagsValue)
	defer app.Unref()
	ui := &App{controller: ctrl, app: app, actionQueue: make(chan action.Action, 64)}
	ui.startActionWorker()
	activate := func(_ gio.Application) { ui.activate() }
	ui.retain(activate)
	app.ConnectActivate(&activate)
	return app.Run(len(os.Args), os.Args)
}

func (a *App) activate() {
	window := pgtk.NewApplicationWindow(a.app)
	a.window = window
	window.SetDefaultSize(fallbackWindowWidth, fallbackWindowHeight)
	title := windowTitle
	window.SetTitle(&title)
	window.SetDecorated(true)
	window.SetResizable(true)
	if tryInitLayerShell(window) {
		window.SetDecorated(false)
		window.SetResizable(false)
	}

	content := a.buildUI()
	window.SetChild(&content.Widget)

	closeRequest := func(_ pgtk.Window) bool {
		a.app.Quit()
		return true
	}
	a.retain(closeRequest)
	window.ConnectCloseRequest(&closeRequest)
	a.installKeyController(window)

	a.refreshStateAsync()
	window.Present()
}

func tryInitLayerShell(window *pgtk.ApplicationWindow) bool {
	if window == nil {
		return false
	}
	if !layershell.Available() || !layershell.IsSupported() {
		return false
	}
	layershell.InitForWindow(&window.Window)
	layershell.SetLayer(&window.Window, layershell.LayerOverlayValue)
	layershell.SetExclusiveZone(&window.Window, 0)
	namespace := layerShellNamespace
	layershell.SetNamespace(&window.Window, &namespace)
	layershell.SetKeyboardMode(&window.Window, layershell.KeyboardModeExclusiveValue)
	layershell.SetAnchor(&window.Window, layershell.EdgeTopValue, true)
	layershell.SetAnchor(&window.Window, layershell.EdgeRightValue, true)
	layershell.SetMargin(&window.Window, layershell.EdgeTopValue, 24)
	layershell.SetMargin(&window.Window, layershell.EdgeRightValue, 24)
	return true
}

func (a *App) buildUI() *pgtk.Box {
	root := pgtk.NewBox(pgtk.OrientationVerticalValue, 12)
	root.SetMarginTop(16)
	root.SetMarginBottom(16)
	root.SetMarginStart(16)
	root.SetMarginEnd(16)

	title := pgtk.NewLabel(nil)
	title.SetText("shrmt")
	title.SetXalign(0)
	title.AddCssClass("title-3")
	root.Append(&title.Widget)

	a.targetLabel = pgtk.NewLabel(nil)
	a.targetLabel.SetXalign(0)
	a.targetLabel.SetText("Target: resolving…")
	root.Append(&a.targetLabel.Widget)

	a.statusLabel = pgtk.NewLabel(nil)
	a.statusLabel.SetXalign(0)
	a.statusLabel.SetText("Ready")
	root.Append(&a.statusLabel.Widget)

	a.targetSection = pgtk.NewBox(pgtk.OrientationVerticalValue, 8)

	hostLabel := pgtk.NewLabel(nil)
	hostLabel.SetText("Host or IP")
	hostLabel.SetXalign(0)
	a.targetSection.Append(&hostLabel.Widget)

	a.hostEntry = pgtk.NewEntry()
	hostPlaceholder := "Auto-discovered or manual host"
	a.hostEntry.SetPlaceholderText(&hostPlaceholder)
	a.hostEntry.SetWidthChars(28)
	hostActivate := func(_ pgtk.Entry) { a.saveTargetAsync() }
	a.retain(hostActivate)
	a.hostEntry.ConnectActivate(&hostActivate)
	a.targetSection.Append(&a.hostEntry.Widget)

	hostButtons := pgtk.NewBox(pgtk.OrientationHorizontalValue, 8)
	discoverBtn := a.newButton("Discover", func() { a.discoverAsync() })
	saveBtn := a.newButton("Save Target", func() { a.saveTargetAsync() })
	hostButtons.Append(&discoverBtn.Widget)
	hostButtons.Append(&saveBtn.Widget)
	a.targetSection.Append(&hostButtons.Widget)
	root.Append(&a.targetSection.Widget)

	a.pairSection = pgtk.NewBox(pgtk.OrientationVerticalValue, 8)

	pairLabel := pgtk.NewLabel(nil)
	pairLabel.SetText("Pairing code")
	pairLabel.SetXalign(0)
	a.pairSection.Append(&pairLabel.Widget)

	a.pairEntry = pgtk.NewEntry()
	pairPlaceholder := "6-character hex code"
	a.pairEntry.SetPlaceholderText(&pairPlaceholder)
	pairActivate := func(_ pgtk.Entry) { a.pairAsync() }
	a.retain(pairActivate)
	a.pairEntry.ConnectActivate(&pairActivate)
	a.pairSection.Append(&a.pairEntry.Widget)

	pairBtn := a.newButton("Pair", func() { a.pairAsync() })
	a.pairSection.Append(&pairBtn.Widget)
	root.Append(&a.pairSection.Widget)

	grid := pgtk.NewGrid()
	grid.SetColumnSpacing(8)
	grid.SetRowSpacing(8)
	grid.SetMarginTop(8)
	grid.Attach(&a.newSpacer().Widget, 0, 0, 1, 1)
	grid.Attach(&a.newActionButton("↑", action.Up).Widget, 1, 0, 1, 1)
	grid.Attach(&a.newSpacer().Widget, 2, 0, 1, 1)
	grid.Attach(&a.newActionButton("←", action.Left).Widget, 0, 1, 1, 1)
	grid.Attach(&a.newActionButton("OK", action.Enter).Widget, 1, 1, 1, 1)
	grid.Attach(&a.newActionButton("→", action.Right).Widget, 2, 1, 1, 1)
	grid.Attach(&a.newSpacer().Widget, 0, 2, 1, 1)
	grid.Attach(&a.newActionButton("↓", action.Down).Widget, 1, 2, 1, 1)
	grid.Attach(&a.newSpacer().Widget, 2, 2, 1, 1)
	root.Append(&grid.Widget)

	topRow := pgtk.NewBox(pgtk.OrientationHorizontalValue, 8)
	topRow.Append(&a.newActionButton("Power", action.Power).Widget)
	topRow.Append(&a.newActionButton("Home", action.Home).Widget)
	topRow.Append(&a.newActionButton("Back", action.Back).Widget)
	root.Append(&topRow.Widget)

	mediaRow := pgtk.NewBox(pgtk.OrientationHorizontalValue, 8)
	mediaRow.Append(&a.newActionButton("Play/Pause", action.PlayPause).Widget)
	mediaRow.Append(&a.newActionButton("Mute", action.Mute).Widget)
	mediaRow.Append(&a.newActionButton("Sleep", action.Sleep).Widget)
	root.Append(&mediaRow.Widget)

	volumeRow := pgtk.NewBox(pgtk.OrientationHorizontalValue, 8)
	volumeRow.Append(&a.newActionButton("Vol-", action.VolumeDown).Widget)
	volumeRow.Append(&a.newActionButton("Vol+", action.VolumeUp).Widget)
	root.Append(&volumeRow.Widget)

	appsRow := pgtk.NewBox(pgtk.OrientationHorizontalValue, 8)
	for _, shortcut := range appShortcuts {
		appsRow.Append(&a.newLaunchButton(shortcut.Label, shortcut.Link).Widget)
	}
	root.Append(&appsRow.Widget)

	closeBtn := a.newButton("Close", func() { a.app.Quit() })
	root.Append(&closeBtn.Widget)

	return root
}

func (a *App) newActionButton(label string, act action.Action) *pgtk.Button {
	btn := a.newButton(label, func() { a.sendAsync(act) })
	btn.SetHexpand(true)
	btn.SetSizeRequest(88, 52)
	a.actionButtons = append(a.actionButtons, btn)
	return btn
}

func (a *App) newLaunchButton(label, link string) *pgtk.Button {
	btn := a.newButton(label, func() { a.launchAsync(label, link) })
	btn.SetHexpand(true)
	btn.SetSizeRequest(88, 52)
	a.actionButtons = append(a.actionButtons, btn)
	return btn
}

func (a *App) newButton(label string, onClick func()) *pgtk.Button {
	btn := pgtk.NewButtonWithLabel(label)
	cb := func(_ pgtk.Button) { onClick() }
	a.retain(cb)
	btn.ConnectClicked(&cb)
	return btn
}

func (a *App) newSpacer() *pgtk.Box {
	box := pgtk.NewBox(pgtk.OrientationHorizontalValue, 0)
	box.SetSizeRequest(8, 8)
	return box
}

func (a *App) installKeyController(window *pgtk.ApplicationWindow) {
	keyCtrl := pgtk.NewEventControllerKey()
	keyPressed := func(_ pgtk.EventControllerKey, keyval uint, _ uint, _ gdk.ModifierType) bool {
		switch keyval {
		case uint(gdk.KEY_Escape):
			a.app.Quit()
			return true
		case uint(gdk.KEY_BackSpace):
			a.sendAsync(action.Back)
			return true
		case uint(gdk.KEY_Left):
			a.sendAsync(action.Left)
			return true
		case uint(gdk.KEY_Right):
			a.sendAsync(action.Right)
			return true
		case uint(gdk.KEY_Up):
			a.sendAsync(action.Up)
			return true
		case uint(gdk.KEY_Down):
			a.sendAsync(action.Down)
			return true
		case uint(gdk.KEY_Return), uint(gdk.KEY_KP_Enter), uint(gdk.KEY_space):
			a.sendAsync(action.Enter)
			return true
		case uint(gdk.KEY_h), uint(gdk.KEY_H):
			a.sendAsync(action.Home)
			return true
		case uint(gdk.KEY_p), uint(gdk.KEY_P):
			a.sendAsync(action.PlayPause)
			return true
		case uint(gdk.KEY_m), uint(gdk.KEY_M):
			a.sendAsync(action.Mute)
			return true
		case uint(gdk.KEY_plus), uint(gdk.KEY_KP_Add):
			a.sendAsync(action.VolumeUp)
			return true
		case uint(gdk.KEY_minus), uint(gdk.KEY_KP_Subtract):
			a.sendAsync(action.VolumeDown)
			return true
		default:
			return false
		}
	}
	a.retain(keyPressed)
	keyCtrl.ConnectKeyPressed(&keyPressed)
	window.AddController(&keyCtrl.EventController)
}

func (a *App) refreshStateAsync() {
	a.setBusy(true)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		state, err := a.controller.Load(ctx, ports.LoadRequest{Target: a.explicitTarget()})
		a.post(func() {
			a.setBusy(false)
			if err != nil {
				a.setStatus(err.Error())
				return
			}
			a.applyRemoteState(state)
		})
	}()
}

func (a *App) discoverAsync() {
	a.setBusy(true)
	a.setStatus("Discovering devices…")
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		devices, err := a.controller.Discover(ctx)
		a.post(func() {
			a.setBusy(false)
			if err != nil {
				a.setStatus(err.Error())
				return
			}
			if len(devices) == 0 {
				a.setStatus("No devices found")
				return
			}
			if len(devices) == 1 {
				target, err := device.TargetFromDevice(devices[0], 6466)
				if err == nil {
					a.hostEntry.SetText(target.Host)
					_ = a.controller.SelectTarget(context.Background(), ports.SelectTargetRequest{Target: target})
				}
				a.setStatus(fmt.Sprintf("Discovered %s", firstNonEmpty(devices[0].Instance, devices[0].HostName, targetLabel(target))))
				a.refreshStateAsync()
				return
			}
			names := make([]string, 0, len(devices))
			for _, dev := range devices {
				names = append(names, firstNonEmpty(dev.Instance, dev.HostName))
			}
			a.setStatus(fmt.Sprintf("Found %d devices: %s", len(devices), strings.Join(names, ", ")))
		})
	}()
}

func (a *App) saveTargetAsync() {
	target := a.explicitTarget()
	if target == nil {
		a.setStatus("Enter a host or IP first")
		return
	}
	a.setBusy(true)
	go func() {
		err := a.controller.SelectTarget(context.Background(), ports.SelectTargetRequest{Target: *target})
		a.post(func() {
			a.setBusy(false)
			if err != nil {
				a.setStatus(err.Error())
				return
			}
			a.setStatus("Saved target")
			a.refreshStateAsync()
		})
	}()
}

func (a *App) pairAsync() {
	code, err := pairing.ParseCode(a.pairEntry.GetText())
	if err != nil {
		a.setStatus(err.Error())
		return
	}
	a.setBusy(true)
	a.setStatus("Pairing… check the TV")
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		state, err := a.controller.Pair(ctx, ports.PairRequest{Target: a.explicitTarget(), Code: code})
		a.post(func() {
			a.setBusy(false)
			if err != nil {
				a.setStatus(err.Error())
				return
			}
			a.pairEntry.SetText("")
			a.applyPairingState(state)
			a.refreshStateAsync()
		})
	}()
}

func (a *App) sendAsync(act action.Action) {
	if a.actionQueue == nil {
		a.setStatus("Action queue unavailable")
		return
	}
	depth := a.actionQueueDepth.Add(1)
	select {
	case a.actionQueue <- act:
		if depth == 1 {
			a.setStatus(fmt.Sprintf("Queued %s", act))
		} else {
			a.setStatus(fmt.Sprintf("Queued %s · %d pending", act, depth))
		}
	default:
		a.actionQueueDepth.Add(-1)
		a.setStatus("Action queue full")
	}
}

func (a *App) launchAsync(label, link string) {
	a.setBusy(true)
	a.setStatus(fmt.Sprintf("Launching %s…", label))
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := a.controller.Launch(ctx, ports.LaunchRequest{Target: a.explicitTarget(), Link: link})
		a.post(func() {
			a.setBusy(false)
			if err != nil {
				a.setStatus(err.Error())
				return
			}
			a.setStatus(fmt.Sprintf("Launched %s", label))
		})
	}()
}

func (a *App) explicitTarget() *device.Target {
	if a.hostEntry == nil {
		return nil
	}
	host := strings.TrimSpace(a.hostEntry.GetText())
	if host == "" {
		return nil
	}
	port := 6466
	if parsedHost, parsedPort, err := hostPort(host); err == nil {
		host = parsedHost
		port = parsedPort
	}
	return &device.Target{Host: host, Port: port, Label: host}
}

func hostPort(raw string) (string, int, error) {
	host, portText, err := net.SplitHostPort(raw)
	if err != nil {
		return "", 0, err
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return "", 0, err
	}
	return host, port, nil
}

func (a *App) applyRemoteState(state remote.State) {
	if state.HasTarget {
		if strings.TrimSpace(a.hostEntry.GetText()) == "" {
			a.hostEntry.SetText(state.Target.Host)
		}
		a.targetLabel.SetText("Target: " + targetLabel(state.Target))
	} else {
		a.targetLabel.SetText("Target: none")
	}
	if a.targetSection != nil {
		a.targetSection.SetVisible(!state.SavedTarget)
	}
	a.applyPairingState(state.Pairing)
}

func (a *App) applyPairingState(state pairing.State) {
	if a.pairSection != nil {
		a.pairSection.SetVisible(!state.Available)
	}
	if state.Available {
		a.setStatus(fmt.Sprintf("Paired (%s)", state.Credentials.Source))
		return
	}
	a.setStatus("Not paired")
}

func (a *App) setBusy(busy bool) {
	for _, btn := range a.actionButtons {
		btn.SetSensitive(!busy)
	}
}

func (a *App) startActionWorker() {
	if a.actionQueue == nil {
		return
	}
	go func() {
		for act := range a.actionQueue {
			pending := a.actionQueueDepth.Load()
			a.post(func() {
				status := fmt.Sprintf("Sending %s…", act)
				if pending > 1 {
					status += fmt.Sprintf(" · %d queued", pending-1)
				}
				a.setStatus(status)
			})

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			result, err := a.controller.Send(ctx, ports.SendRequest{Target: a.explicitTarget(), Action: act})
			cancel()

			remaining := a.actionQueueDepth.Add(-1)
			a.post(func() {
				if err != nil {
					status := err.Error()
					if remaining > 0 {
						status += fmt.Sprintf(" · %d queued", remaining)
					}
					a.setStatus(status)
					return
				}
				status := fmt.Sprintf("Sent %s", result.Action)
				if result.Powered != nil {
					status += fmt.Sprintf(" · power=%t", *result.Powered)
				}
				if remaining > 0 {
					status += fmt.Sprintf(" · %d queued", remaining)
				}
				a.setStatus(status)
			})
		}
	}()
}

func (a *App) setStatus(text string) {
	if a.statusLabel != nil {
		a.statusLabel.SetText(text)
	}
}

func (a *App) retain(cb interface{}) {
	a.callbacks = append(a.callbacks, cb)
}

func (a *App) post(fn func()) {
	cb := glib.SourceFunc(func(uintptr) bool {
		fn()
		return false
	})
	glib.IdleAdd(&cb, 0)
}

func targetLabel(target device.Target) string {
	if target.Label != "" && target.Label != target.Host {
		return fmt.Sprintf("%s (%s:%d)", target.Label, target.Host, target.Port)
	}
	return fmt.Sprintf("%s:%d", target.Host, target.Port)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
