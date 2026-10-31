# shrmt

`shrmt` is a GTK4 + layer-shell remote control app for NVIDIA SHIELD TV.

## Runtime dependencies

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

Launch the GTK app:

```bash
shrmt
```

Run from source:

```bash
go run ./cmd/shrmt
```

CLI examples:

```bash
shrmt discover
shrmt pair --host <shield-ip>
shrmt key home --host <shield-ip>
```

## Remote layout

Top to bottom: Volume and Power toolbar (Volume reveals Vol-, Mute, Vol+), D-pad with OK, Back/Home/Menu, Rewind/Play-Pause/Fast-forward, app shortcuts, Close. Power sends a single toggle; the SHIELD decides on or off. `sleep` and `wakeup` remain available in the CLI.

Keyboard: arrows, Enter/Space (OK), Backspace (Back), `h` (Home), Menu key (Menu), `,` (Rewind), `.` (Fast-forward), `p` (Play/Pause), `m` (Mute), `+`/`-` (Volume).

## Credentials and target storage

`shrmt` uses:

- `~/.config/shrmt/androidtv-client-cert.pem`
- `~/.config/shrmt/androidtv-client-key.pem`
- `~/.config/shrmt/target.json`

## Testing

```bash
make test
```

## Mocks

Mockery v3 config lives in `.mockery.yaml`.

```bash
make mock
```
