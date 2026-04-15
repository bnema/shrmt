# Official references

## Android TV / Android platform

### Android TV developer hub
- https://developer.android.com/tv

Useful for:
- platform conventions
- TV-focused app behavior
- supported interaction models

### Android `KeyEvent` reference
- https://developer.android.com/reference/android/view/KeyEvent

Useful for:
- `DPAD_UP`, `DPAD_DOWN`, `DPAD_LEFT`, `DPAD_RIGHT`, `DPAD_CENTER`
- `HOME`, `BACK`, `MEDIA_PLAY_PAUSE`, volume-related key codes
- mapping CLI commands to Android input semantics

### Intents and intent filters
- https://developer.android.com/guide/components/intents-filters

Useful for:
- launching activities
- understanding exported app entry points
- evaluating whether app launch can be solved with standard Android behavior

### Android App Links / deep links
- https://developer.android.com/training/app-links

Useful for:
- launching apps directly into content
- evaluating whether services like YouTube or Twitch can be opened without vendor-specific APIs

### ADB documentation
- https://developer.android.com/tools/adb

Useful for:
- fallback control path when network remote protocols are insufficient
- shell-based testing
- sending key events and launching activities during early prototyping

## AOSP / source references

### Google TV pairing protocol (AOSP)
- https://android.googlesource.com/platform/external/google-tv-pairing-protocol/

Useful for:
- understanding the official pairing-side implementation lineage
- comparing Android TV Remote v2 behavior against source-level references

## Important limitation

These official references are helpful for:

- key mapping
- intents and deep links
- ADB-based control
- Android TV behavior in general

They do **not** provide a polished public desktop API for:

- NVIDIA SHIELD-specific remote control
- proprietary SHIELD discovery
- proprietary SHIELD pairing
- proprietary SHIELD launcher / remote-locator features

That part still requires source inspection and reverse engineering.
