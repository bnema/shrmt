package action

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

var ErrUnknownAction = errors.New("unknown action")

type Action string

const (
	Back        Action = "back"
	Down        Action = "down"
	Enter       Action = "enter"
	FastForward Action = "fast-forward"
	Home        Action = "home"
	Left        Action = "left"
	Menu        Action = "menu"
	Mute        Action = "mute"
	PlayPause   Action = "play-pause"
	Power       Action = "power"
	Rewind      Action = "rewind"
	Right       Action = "right"
	Sleep       Action = "sleep"
	Up          Action = "up"
	VolumeDown  Action = "volume-down"
	VolumeUp    Action = "volume-up"
	Wakeup      Action = "wakeup"
)

var aliases = map[string]Action{
	"back":         Back,
	"center":       Enter,
	"down":         Down,
	"enter":        Enter,
	"ff":           FastForward,
	"fast-forward": FastForward,
	"home":         Home,
	"left":         Left,
	"menu":         Menu,
	"mute":         Mute,
	"ok":           Enter,
	"play-pause":   PlayPause,
	"power":        Power,
	"rew":          Rewind,
	"rewind":       Rewind,
	"right":        Right,
	"sleep":        Sleep,
	"up":           Up,
	"vol-down":     VolumeDown,
	"vol-up":       VolumeUp,
	"voldown":      VolumeDown,
	"volume-down":  VolumeDown,
	"volume-up":    VolumeUp,
	"volup":        VolumeUp,
	"wake":         Wakeup,
	"wake-up":      Wakeup,
	"wakeup":       Wakeup,
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
		FastForward,
		Home,
		Left,
		Menu,
		Mute,
		PlayPause,
		Power,
		Rewind,
		Right,
		Sleep,
		Up,
		VolumeDown,
		VolumeUp,
		Wakeup,
	}
	slices.Sort(out)
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
