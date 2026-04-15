# shield-poc

Small Go proof of concept for discovering, pairing with, and controlling an NVIDIA SHIELD TV from Linux.

## What works

- discover SHIELD / Android TV services on the local network
- probe exposed endpoints and TLS behavior
- pair with Android TV Remote v2
- send basic commands like `home` and `power`

## Quick start

Build:

```bash
rtk proxy go build ./...
```

Discover devices:

```bash
rtk proxy go run . discover --timeout 5s
```

Probe endpoints:

```bash
rtk proxy go run . probe --timeout 5s
```

Pair:

```bash
rtk proxy go run . pair
```

Send a key:

```bash
rtk proxy go run . key home
rtk proxy go run . power
```

## Credentials

Pairing credentials are stored locally in your user config directory, for example:

- `~/.config/shield-poc/androidtv-client-cert.pem`
- `~/.config/shield-poc/androidtv-client-key.pem`

These files are local-only and should not be committed.

## Research docs

See [`research/`](./research) for:

- official references
- exposed services and usability notes
- APK reverse-engineering notes
- sanitized live network findings
- POC plan

## Privacy

The published research is sanitized and avoids storing personal network identifiers, pairing codes, and local file paths.

## License

MIT — see [`LICENSE`](./LICENSE).
