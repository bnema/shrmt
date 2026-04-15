# Research index

This directory contains shareable research notes for the NVIDIA SHIELD TV control POC.

## Documents

- [`official-references.md`](./official-references.md)
  - official Android TV, Android platform, ADB, and AOSP references
- [`nvidia-shield-tv-apk-notes.md`](./nvidia-shield-tv-apk-notes.md)
  - findings from static inspection of the NVIDIA SHIELD TV Android app
- [`live-network-findings.md`](./live-network-findings.md)
  - sanitized results from local-network discovery and TLS probing
- [`poc-plan.md`](./poc-plan.md)
  - implementation phases and recommended order of work
- [`apk/README.md`](./apk/README.md)
  - guidance for handling APK artifacts safely in an open repository

## Scope

These notes focus on:

- discovery
- pairing
- remote input
- app launching
- SHIELD-specific services

## Out of scope

At this stage, the research does **not** attempt to:

- bypass security controls
- access cloud services
- redistribute proprietary application binaries
- publish private captures from a specific device or network
