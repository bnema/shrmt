# APK artifacts

This directory is for **documentation only**.

## Rules

- do not commit proprietary APK binaries here
- do not commit decompiled proprietary source trees here
- do not commit personal download paths or device-specific captures here
- if an APK is analyzed, publish only:
  - version information
  - high-level architecture notes
  - extracted string / protocol observations
  - interoperability-relevant conclusions

## Recommended workflow

1. analyze APKs locally outside the repository
2. extract only shareable findings
3. summarize those findings in markdown documents
4. redact device-specific identifiers before committing
