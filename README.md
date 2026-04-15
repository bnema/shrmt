# shrmt

`shrmt` is a GTK4 + layer-shell NVIDIA SHIELD remote built on top of the Android TV Remote v2 path already proven in this repository.

## Current shape

- `core/`
  - domain/business packages: `action`, `device`, `pairing`, `remote`
- `ports/`
  - inbound controller contract used by CLI and GTK
- `controller/`
  - thin composition/orchestration layer
- `adapters/in/`
  - `cli`, `gtk`
- `adapters/out/`
  - `androidtv`, `zeroconf`, `xdg`

The older POC transport code still lives under `internal/atvremote` and `internal/discovery`, and is now wrapped by the outbound adapters.

## Runtime dependencies

Wayland overlay mode needs:

- `gtk4`
- `gtk4-layer-shell`

Arch:

```bash
sudo pacman -S gtk4 gtk4-layer-shell
```

## Build

```bash
make build
```

This writes the binary to `./bin/shrmt`.

## Install

```bash
make install
```

This installs `shrmt` to `~/.local/bin/shrmt` by default.

## Run

Launch the GTK remote:

```bash
go run ./cmd/shrmt
```

Use the CLI:

```bash
go run ./cmd/shrmt discover
go run ./cmd/shrmt pair --host <shield-ip>
go run ./cmd/shrmt key home --host <shield-ip>
go run ./cmd/shrmt power --host <shield-ip>
```

## Credentials and target storage

`shrmt` uses:

- `~/.config/shrmt/androidtv-client-cert.pem`
- `~/.config/shrmt/androidtv-client-key.pem`
- `~/.config/shrmt/target.json`

It also falls back to legacy credentials and target config in:

- `~/.config/shremote/androidtv-client-cert.pem`
- `~/.config/shremote/androidtv-client-key.pem`
- `~/.config/shremote/target.json`
- `~/.config/shield-poc/androidtv-client-cert.pem`
- `~/.config/shield-poc/androidtv-client-key.pem`

## Niri example

```kdl
Mod+Ctrl+S hotkey-overlay-title="NVIDIA Shield: shrmt" { spawn "shrmt"; }
```

This matches the same hotkey-launched overlay style already used for `dumber omnibox` and `sekeve omnibox`.

## Testing

```bash
make test
```

## Mocks

Mockery v3 config lives in `.mockery.yaml`.

```bash
make mock
```
