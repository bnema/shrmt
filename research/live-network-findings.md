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
