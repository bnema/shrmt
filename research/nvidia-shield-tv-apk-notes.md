# NVIDIA SHIELD TV APK reverse-engineering notes

## Sample analyzed

A recent public release of the NVIDIA SHIELD TV Android app was inspected locally.

Shareable metadata from that sample:

- package: `com.nvidia.shield.remote`
- app label: `SHIELD TV`
- main activity: `com.nvidia.shield.remote.MainActivity`
- min SDK observed: `21`
- target SDK observed: `31`

Local filesystem paths used during analysis are intentionally omitted.

## High-level architecture

The app is a **Flutter** application.

Interesting embedded package names found in the compiled app payload include:

- `package:ShieldRemote/...`
- `package:beyonder_backend/...`
- `package:mdns/mdns.dart`
- `package:beyonder_backend/tcp/TcpSocket.dart`
- `package:beyonder_backend/certificate/CertificateManager.dart`

### Interpretation

Most of the SHIELD-specific logic appears to live in compiled Dart AOT code inside `libapp.so`, not in a large Java/Kotlin application layer.

## Manifest-level observations

Permissions observed include:

- `android.permission.INTERNET`
- `android.permission.ACCESS_NETWORK_STATE`
- `android.permission.ACCESS_WIFI_STATE`
- `android.permission.CHANGE_WIFI_MULTICAST_STATE`
- `android.permission.WAKE_LOCK`
- `android.permission.VIBRATE`
- `android.permission.RECORD_AUDIO`
- `android.permission.ACCESS_COARSE_LOCATION`
- `android.permission.ACCESS_FINE_LOCATION`
- `android.permission.BLUETOOTH`
- `android.permission.BLUETOOTH_ADMIN`
- `android.permission.BLUETOOTH_SCAN`
- `android.permission.BLUETOOTH_CONNECT`

### Interpretation

This aligns with an app that needs:

- local-network discovery
- multicast / mDNS access
- Bluetooth-assisted discovery or accessory features
- voice / audio features
- active network communication with a SHIELD device

## Discovery findings

The most important discovery-related string found in the app is:

- `_nv_shield_remote._tcp`

Related API / channel strings include:

- `remote.shield.nvidia.com/mdns/api`
- `remote.shield.nvidia.com/mdns/discovery`
- `remote.shield.nvidia.com/mdns/resolution`
- `remote.shield.nvidia.com/mdns/state`

### Interpretation

The NVIDIA mobile app does **not** appear to rely only on the standard Android TV / Google TV discovery path.
It also appears to use a **NVIDIA-specific mDNS service**:

- `_nv_shield_remote._tcp`

For interoperability work, a client should likely scan at least:

- `_nv_shield_remote._tcp`
- `_androidtvremote._tcp`
- `_androidtvremote2._tcp`

## Pairing / security findings

Strings strongly suggesting certificate-based authenticated pairing include:

- `CertificateManager`
- `PairingPinCode`
- `encodeAuthenticationPinRequest`
- `encodeAuthenticationPinCodeSend`
- `authenticationPairingResult`
- `onAuthenticationPinChallenge`
- `onAuthenticationPairingResult`
- `COMMAND_PAIRING_RESULT`
- `TlsException`
- `Socket_GetRemotePeer`
- `nativeGetRemotePeer`
- `pairedHosts.json`

### Interpretation

A likely pairing flow is:

1. discover the SHIELD over mDNS
2. connect over TCP
3. establish or upgrade to TLS
4. request a pairing challenge
5. display or consume a PIN challenge
6. send the PIN response back to the device
7. persist pairing / trust state locally

## Protocol findings

The app contains a protobuf-driven backend under `beyonder_backend`.

Interesting message / service names found include:

- `NvBeyonderHandshakeMsg`
- `NvBeyonderAuthenticationMsg`
- `NvBeyonderHostInfoMsg`
- `NvBeyonderVirtualInputMsg`
- `VolumeControlPayload`
- `MediaSessionPayload`
- `FileTransferServicePayload`
- `AccessoryLocatorRequest`
- `RemoteLauncherServicePayload`

### Interpretation

This looks like a structured proprietary protocol stack, not just a thin wrapper around ADB.

## Features confirmed from strings and assets

### Remote input

Observed strings / assets indicate support for:

- D-pad navigation
- select / center
- mouse / touchpad input
- mouse wheel
- keyboard input
- volume handling

### Pairing UX

Observed strings include:

- `Pair a new device`
- `Pair using IP`
- `No SHIELD devices found. Try pairing via IP.`
- `Cannot pair to SHIELD TV`

### App launching

Observed strings include:

- `REQUEST_APP_LIST`
- `launchApp`
- `App launched succeeded`
- `Failed to launch app`
- `launchDb.json`
- `SERVICEID_REMOTE_LAUNCHER`
- `RemoteLauncherServicePayload`

### SHIELD-specific accessory features

Observed strings include:

- `Find my remote`
- `Play sound on your SHIELD remote`
- `AccessoryLocatorRequest`
- `ACCESSORY_REMOTE`
- `ACCESSORY_TOUCHPAD`
- `ACCESSORY_KEYBOARD`
- `ACCESSORY_MOUSE`

### Extra capabilities hinted by strings

- voice search
- audio input / output
- file transfer
- private listening

## Current conclusion

The NVIDIA SHIELD mobile app appears to use a **custom NVIDIA protocol stack** with:

- mDNS discovery over `_nv_shield_remote._tcp`
- protobuf messages
- TCP transport
- TLS / certificate management
- PIN-based pairing
- services for virtual input, host info, launcher, volume/media, and accessory locating

This is encouraging for a Go-based interoperability client because it suggests there is a reusable network protocol surface to target.

## Recommended next reverse-engineering steps

1. characterize the mDNS advertisements exposed by a SHIELD device
2. keep discovery code focused on both standard Android TV and NVIDIA-specific services
3. inspect handshake and authentication framing more deeply
4. compare the proprietary path with Android TV Remote v2 before implementing more of the vendor-specific flow
