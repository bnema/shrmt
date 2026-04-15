# shield-poc

Research-driven proof of concept for controlling an NVIDIA SHIELD TV from a Linux desktop.

## Goals

- document the protocol landscape around NVIDIA SHIELD TV remote control
- identify what is already covered by Android TV / Google TV standards
- reverse engineer only what is necessary for interoperability
- build a small Go CLI using **Go 1.26.2** and **Cobra**

## Current status

This repository currently contains:

- shareable research notes under [`research/`](./research)
- a tiny discovery CLI that can browse likely SHIELD / Android TV mDNS services

## Documentation index

- [`research/README.md`](./research/README.md) — research index
- [`research/official-references.md`](./research/official-references.md) — official Android / AOSP references
- [`research/nvidia-shield-tv-apk-notes.md`](./research/nvidia-shield-tv-apk-notes.md) — APK reverse-engineering notes
- [`research/live-network-findings.md`](./research/live-network-findings.md) — sanitized live discovery / TLS findings
- [`research/poc-plan.md`](./research/poc-plan.md) — phased implementation plan
- [`research/apk/README.md`](./research/apk/README.md) — handling rules for APK-related artifacts

## Sanitization policy

This repository is intended to be shareable.

The published notes intentionally avoid storing:

- private LAN IP addresses
- device-specific hostnames where avoidable
- full certificate fingerprints
- Bluetooth MAC addresses
- captured pairing codes
- personal APK download paths
- raw packet captures or pairing material

Examples use placeholders or redacted values when needed.

## Tiny CLI

The current CLI is focused on discovery.

```bash
rtk proxy go run . discover --timeout 5s
```

Example output shape:

```text
service=_androidtvremote2._tcp instance="<device-name>" host=<host>.local port=6466
  ipv4: <redacted>
  txt:  bt=<redacted>

service=_nv_shield_remote._tcp instance="<device-name>" host=<host>.local port=8987
  ipv4: <redacted>
  txt:  SERVER=<redacted>, SERVER_CAPABILITY=<value>
```

## Build

```bash
rtk proxy go build ./...
```

## Next steps

1. keep polishing public research docs
2. implement pairing for Android TV Remote v2
3. validate basic commands: home, back, d-pad, select, power
4. investigate the proprietary NVIDIA `nvbeyonder` path for SHIELD-specific features
