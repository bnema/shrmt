# Exposed surfaces and current usability

This document consolidates what the research currently says about an NVIDIA SHIELD TV device:

- what is **confirmed to be exposed** on the local network
- what is **confirmed usable** from this repository today
- what is **strongly suggested** by APK reverse engineering but not yet implemented here
- what is **not currently indicated** by the observed control protocols

All notes here are intentionally sanitized and avoid device-specific identifiers.

## 1. Confirmed exposed network surfaces

### 1.1 Android TV Remote v2

Confirmed exposed:

- mDNS service: `_androidtvremote2._tcp`
- remote command port: `6466`
- pairing-related port: `6467`
- transport: TLS

What this likely represents:

- the standard Android TV / Google TV remote-control path
- pairing for a remote client
- key injection / navigation commands
- remote state and capability exchange

### 1.2 NVIDIA proprietary SHIELD service

Confirmed exposed:

- mDNS service: `_nv_shield_remote._tcp`
- observed port: `8987`
- transport: TLS

What this likely represents:

- a NVIDIA-specific SHIELD control surface
- richer SHIELD-only functionality beyond the standard Android TV path

## 2. Confirmed usable from this repository

The following items are not just theoretical; they have already been exercised by the current Go CLI.

### 2.1 Discovery

Confirmed usable:

- discovery of `_androidtvremote2._tcp`
- discovery of `_nv_shield_remote._tcp`

CLI:

```bash
go run . discover --timeout 5s
```

### 2.2 Endpoint probing

Confirmed usable:

- TCP reachability checks
- TLS handshake checks
- observation of self-signed certificates
- distinction between Android TV and NVIDIA-specific TLS naming patterns

CLI:

```bash
go run . probe --timeout 5s
```

### 2.3 Android TV Remote v2 pairing

Confirmed usable:

- local client certificate generation
- TLS connection to the pairing port
- pairing protobuf exchange
- PIN-based pairing
- persistence of reusable client credentials

CLI:

```bash
go run . pair
```

### 2.4 Basic remote command path

Confirmed usable:

- remote connection on the Android TV command port
- remote handshake / feature exchange
- key injection path

Hardware-validated command so far:

- `key home`

CLI:

```bash
go run . key home
```

## 3. Implemented in the CLI but not yet fully validated on hardware

These commands are implemented in the repository, but they should still be treated as **needs explicit hardware confirmation** unless separately tested and documented.

### 3.1 Power-related command

Implemented:

- `power`

CLI:

```bash
go run . power
```

### 3.2 Additional key actions

Implemented action mapping includes:

- `back`
- `up`
- `down`
- `left`
- `right`
- `enter`
- `play-pause`
- `volume-up`
- `volume-down`
- `mute`
- `sleep`
- `soft-sleep`

The command path is in place, but each behavior may still vary by SHIELD state and should be validated individually.

## 4. Strongly suggested by APK reverse engineering

Based on static inspection of the NVIDIA SHIELD TV app, the following capabilities appear likely to exist behind the proprietary NVIDIA path.

### 4.1 Pairing / authentication

Strong evidence for:

- certificate management
- TLS-based authenticated sessions
- PIN challenge / response flow
- stored trusted hosts / pairing state

### 4.2 Input and remote control

Strong evidence for:

- D-pad / navigation
- keyboard input
- touchpad / mouse movement
- mouse wheel and click behavior

### 4.3 Launcher / app-related services

Strong evidence for:

- app list retrieval
- app launching
- remote launcher service messages

### 4.4 Accessory and SHIELD-specific features

Strong evidence for:

- remote locator / “find my remote”
- accessory-oriented services
- host info and service metadata

### 4.5 Media / audio related services

Strong evidence for:

- media session messages
- volume control messages
- voice / audio input-output related components
- private-listening-related functionality

### 4.6 File-transfer related services

Strings suggest the existence of file-transfer service payloads, but this has not been explored further in this repository.

## 5. What is not currently indicated by these control protocols

Based on the currently observed surfaces, there is **no evidence yet** that these particular remote-control protocols are intended to provide a generic desktop-video sink.

That means the current research does **not** indicate a public or obvious path here for:

- arbitrary Linux desktop screen ingest
- generic screen mirroring into SHIELD through the remote-control APIs
- SHIELD behaving like a general-purpose desktop streaming receiver through these control endpoints alone

If desktop-to-TV streaming is a goal, it is more likely to use a different stack such as:

- Google Cast / Chromecast behavior
- Sunshine / Moonlight
- a custom receiver application on Android TV

## 6. Practical interpretation

For the current project, the cleanest split is:

### Best path for the POC
- Android TV Remote v2
- enough for discovery, pairing, and basic control

### Best path for SHIELD-specific expansion
- `_nv_shield_remote._tcp`
- likely needed for richer NVIDIA-only features such as launcher or accessory-specific behavior

## 7. Recommended public-facing claim

A careful and accurate statement for this repository today would be:

> NVIDIA SHIELD TV exposes both a standard Android TV Remote v2 surface and a separate NVIDIA-specific TLS service on the local network. This repository already demonstrates discovery, probing, pairing, and basic Android TV key injection from Go, while documenting the likely existence of richer SHIELD-specific capabilities behind the proprietary NVIDIA service.
