package atvremote

import (
	"fmt"
	"sort"
	"strings"

	pb "github.com/drosocode/atvremote/pkg/v2/proto"
)

var keyActions = map[string]pb.RemoteKeyCode{
	"back":        pb.RemoteKeyCode_KEYCODE_BACK,
	"center":      pb.RemoteKeyCode_KEYCODE_DPAD_CENTER,
	"down":        pb.RemoteKeyCode_KEYCODE_DPAD_DOWN,
	"enter":       pb.RemoteKeyCode_KEYCODE_DPAD_CENTER,
	"home":        pb.RemoteKeyCode_KEYCODE_HOME,
	"left":        pb.RemoteKeyCode_KEYCODE_DPAD_LEFT,
	"mute":        pb.RemoteKeyCode_KEYCODE_MUTE,
	"ok":          pb.RemoteKeyCode_KEYCODE_DPAD_CENTER,
	"play-pause":  pb.RemoteKeyCode_KEYCODE_MEDIA_PLAY_PAUSE,
	"power":       pb.RemoteKeyCode_KEYCODE_POWER,
	"right":       pb.RemoteKeyCode_KEYCODE_DPAD_RIGHT,
	"sleep":       pb.RemoteKeyCode_KEYCODE_SLEEP,
	"soft-sleep":  pb.RemoteKeyCode_KEYCODE_SOFT_SLEEP,
	"up":          pb.RemoteKeyCode_KEYCODE_DPAD_UP,
	"vol-down":    pb.RemoteKeyCode_KEYCODE_VOLUME_DOWN,
	"vol-up":      pb.RemoteKeyCode_KEYCODE_VOLUME_UP,
	"voldown":     pb.RemoteKeyCode_KEYCODE_VOLUME_DOWN,
	"volume-down": pb.RemoteKeyCode_KEYCODE_VOLUME_DOWN,
	"volume-up":   pb.RemoteKeyCode_KEYCODE_VOLUME_UP,
	"volup":       pb.RemoteKeyCode_KEYCODE_VOLUME_UP,
}

func ResolveKeyCode(action string) (pb.RemoteKeyCode, error) {
	normalized := normalizeAction(action)
	keyCode, ok := keyActions[normalized]
	if !ok {
		return pb.RemoteKeyCode_KEYCODE_UNKNOWN, fmt.Errorf("unknown action %q (supported: %s)", action, strings.Join(AvailableKeyActions(), ", "))
	}
	return keyCode, nil
}

func AvailableKeyActions() []string {
	actions := make([]string, 0, len(keyActions))
	seen := map[string]struct{}{}
	for action := range keyActions {
		canonical := action
		switch action {
		case "ok":
			canonical = "enter"
		case "center":
			canonical = "enter"
		case "voldown", "vol-down":
			canonical = "volume-down"
		case "volup", "vol-up":
			canonical = "volume-up"
		}
		if _, ok := seen[canonical]; ok {
			continue
		}
		seen[canonical] = struct{}{}
		actions = append(actions, canonical)
	}
	sort.Strings(actions)
	return actions
}

func normalizeAction(action string) string {
	action = strings.ToLower(strings.TrimSpace(action))
	action = strings.ReplaceAll(action, "_", "-")
	action = strings.ReplaceAll(action, " ", "-")
	return action
}
