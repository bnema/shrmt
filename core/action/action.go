package action

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

var ErrUnknownAction = errors.New("unknown action")

type Action string

const (
	Back       Action = "back"
	Down       Action = "down"
	Enter      Action = "enter"
	Home       Action = "home"
	Left       Action = "left"
	Mute       Action = "mute"
	PlayPause  Action = "play-pause"
	Power      Action = "power"
	Right      Action = "right"
	Sleep      Action = "sleep"
	Up         Action = "up"
	VolumeDown Action = "volume-down"
	VolumeUp   Action = "volume-up"
)

var aliases = map[string]Action{
	"back":        Back,
	"center":      Enter,
	"down":        Down,
	"enter":       Enter,
	"home":        Home,
	"left":        Left,
	"mute":        Mute,
	"ok":          Enter,
	"play-pause":  PlayPause,
	"power":       Power,
	"right":       Right,
	"sleep":       Sleep,
	"up":          Up,
	"vol-down":    VolumeDown,
	"vol-up":      VolumeUp,
	"voldown":     VolumeDown,
	"volume-down": VolumeDown,
	"volume-up":   VolumeUp,
	"volup":       VolumeUp,
}

func Parse(raw string) (Action, error) {
	normalized := normalize(raw)
	act, ok := aliases[normalized]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrUnknownAction, normalized)
	}
	return act, nil
}

func MustParse(raw string) Action {
	act, err := Parse(raw)
	if err != nil {
		panic(err)
	}
	return act
}

func All() []Action {
	out := []Action{
		Back,
		Down,
		Enter,
		Home,
		Left,
		Mute,
		PlayPause,
		Power,
		Right,
		Sleep,
		Up,
		VolumeDown,
		VolumeUp,
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func normalize(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	raw = strings.ReplaceAll(raw, "_", "-")
	raw = strings.ReplaceAll(raw, " ", "-")
	return raw
}

func (a Action) String() string {
	return string(a)
}
