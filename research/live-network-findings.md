# Sanitized live network findings

These notes summarize what was observed on real hardware while deliberately omitting device-specific identifiers.

## Discovery results

### Standard Android TV Remote v2

A SHIELD device was observed advertising the standard Android TV Remote v2 service:

- service: `_androidtvremote2._tcp`
- port: `6466`
- TXT records: includes a Bluetooth-related field

A pairing-related port was also observed open on:

- port: `6467`

### NVIDIA-specific SHIELD service

The same device was also observed advertising a NVIDIA-specific service:

- service: `_nv_shield_remote._tcp`
- port: `8987`
- TXT records: included server-oriented metadata fields

## TLS observations

### Standard Android TV path

The Android TV remote ports accepted TLS connections.

Observed characteristics:

- self-signed certificate
- certificate naming pattern included an `atvremote/...` prefix
- remote port behavior was consistent with Android TV / Google TV remote infrastructure

### NVIDIA-specific path

The NVIDIA-specific SHIELD service also accepted TLS connections.

Observed characteristics:

- self-signed certificate
- certificate naming pattern included an `nvbeyonder/...` prefix
- behavior was consistent with a proprietary NVIDIA protocol surface

## Active probing on the NVIDIA path

A first round of active `nvprobe` experiments was run directly against the NVIDIA service on `8987`.

Observed behavior so far:

- TLS connects cleanly without a client certificate
- no immediate server banner is sent after connect
- several candidate **varint-framed** protobuf envelopes produced:
  - no response bytes
  - no immediate connection close
  - no observable Android TV power-state change
- several candidate **raw protobuf** envelopes produced a more interesting split:
  - messages with `service_id=1` did **not** immediately close the connection
  - similar raw messages with `service_id>=2` were closed by the server with EOF

### Interpretation

This does **not** prove the full wire format yet, but it does suggest:

- `service_id=1` is a plausible handshake-like service candidate
- raw protobuf framing is still worth investigating
- the server appears to distinguish at least some candidate message shapes rather than blindly black-holing all input

## Hello-preamble follow-up probing

A second round of active probing used a **two-message sequence** on the same TLS connection:

1. send a candidate handshake-like `hello` message on the raw protobuf path
2. immediately send a second raw base message for another candidate service/command pair

Observed behavior:

- without any hello preamble, most raw `service_id >= 2` probes were rejected very quickly with EOF
- with a more complete hello candidate present first, the same follow-up probes were still rejected, but often **much later**
- the strongest delay was seen with hello candidates shaped like:
  - devInfo layout similar to `os, name, packageName, androidId, host, remotePort`
  - plus a `capability` field set to a small integer such as `1` or `2`

Practical meaning:

- the hello payload is likely being parsed more deeply than the earlier minimal probes
- the server appears to enter a different intermediate state after some hello candidates
- we still do **not** have a valid full follow-up request, but the search is now more constrained:
  - build a better hello
  - then probe likely `AUTHENTICATION`, `HOST_INFO`, and `VIRTUAL_INPUT` follow-ups on the same connection

## Practical interpretation

A SHIELD device appears to expose **two important network control surfaces**:

1. **Android TV Remote v2**
   - discovery via `_androidtvremote2._tcp`
   - remote traffic on `6466`
   - pairing-related traffic on `6467`

2. **NVIDIA proprietary SHIELD protocol**
   - discovery via `_nv_shield_remote._tcp`
   - traffic on `8987`

## Why this matters

This suggests a practical implementation strategy:

- use **Android TV Remote v2** first for generic remote/navigation behavior
- investigate the **NVIDIA-specific service** for richer SHIELD-only features such as launcher integration or accessory locating

## Redaction note

The original local observations included device-specific values such as:

- LAN IP addresses
- instance names
- hostnames
- TXT record identifiers
- certificate subjects and fingerprints

Those values are intentionally not published here.
