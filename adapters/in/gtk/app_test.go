package gtk

import "testing"

func TestAppShortcutsUseAndroidTVPackageIDs(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		"YouTube": "com.google.android.youtube.tv",
		"Twitch":  "tv.twitch.android.app",
		"Plex":    "com.plexapp.android",
	}

	if len(appShortcuts) != len(want) {
		t.Fatalf("appShortcuts length = %d, want %d", len(appShortcuts), len(want))
	}
	for _, shortcut := range appShortcuts {
		if got, ok := want[shortcut.Label]; !ok {
			t.Fatalf("unexpected shortcut label %q", shortcut.Label)
		} else if shortcut.Link != got {
			t.Fatalf("shortcut %q link = %q, want %q", shortcut.Label, shortcut.Link, got)
		}
		delete(want, shortcut.Label)
	}
	if len(want) > 0 {
		t.Fatalf("missing shortcuts: %v", want)
	}
}
