# POC plan

## Principles

- prefer public / standard protocols first
- keep the first CLI tiny and observable
- document findings before broadening scope
- avoid storing private device material in the repository

## Phase 1 — discovery

Status: **in progress / partially implemented**

Goals:
- scan likely mDNS services
- confirm which protocols are actually exposed by a SHIELD device
- probe discovered endpoints for TCP / TLS behavior
- record only sanitized findings in public docs

Candidate services:
- `_androidtvremote._tcp`
- `_androidtvremote2._tcp`
- `_nv_shield_remote._tcp`

## Phase 1.5 — endpoint probing

Status: **implemented as an early CLI command**

Goals:
- verify that discovered endpoints are reachable
- confirm whether they expect TLS
- capture certificate naming patterns and protocol versions during local testing

Why this matters:
- helps distinguish the standard Android TV path from the NVIDIA-specific path
- gives us a safer next step before implementing pairing

## Phase 2 — standard Android TV path

Status: **started**

Goals:
- pair against Android TV Remote v2
- validate basic remote commands from Go
- confirm minimal useful feature set

Implemented so far:
- local client certificate generation
- Android TV Remote v2 pairing protobufs
- certificate-based TLS pairing CLI command
- initial remote command flow on the Android TV Remote v2 port
- basic `key` / `power` CLI commands

Target capabilities:
- home
- back
- d-pad navigation
- select / enter
- play / pause
- volume if supported
- power / sleep if supported

Validated so far:
- pairing
- `key home`
- command-channel handshake on the remote port

Why this first:
- likely less proprietary
- potentially enough for an initial usable CLI
- easier to compare against existing open-source implementations

## Phase 3 — NVIDIA proprietary path

Goals:
- characterize the `nvbeyonder` protocol surface
- identify message framing and protobuf usage
- determine how pairing, virtual input, and launcher services work

Likely areas of interest:
- authentication / PIN challenge
- host info
- virtual input
- remote launcher
- accessory locator ("find my remote")

## Phase 4 — app launching

Goals:
- decide whether app launching should use:
  1. standard Android intents / deep links,
  2. Android TV remote capabilities,
  3. NVIDIA remote launcher service

Examples to validate:
- YouTube
- Twitch

## Phase 5 — polish

Goals:
- stable Cobra command structure
- config handling if needed
- helpful logging and debug output
- public docs that others can reproduce without private data

## Suggested command roadmap

- `discover`
- `probe`
- `pair`
- `key`
- `power`
- `launch`
