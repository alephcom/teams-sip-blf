# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.0.3] - 2025-02-28

### Fixed

- BLF dialog-info parsing: use RFC 4235 namespace (`urn:ietf:params:xml:ns:dialog-info`) and parse dialog state from the child `<state>` element instead of an attribute. Fixes presence always showing Available when the PBX sends standard dialog-info XML (e.g. answered call and hangup now correctly show Busy then Available).

## [0.0.2] - 2025-02-28

### Added

- Extensions CSV fallback: if `extensions.json` does not exist, the app tries the same path with `.csv` (e.g. `config/extensions.csv`). CSV format: two columns `extension`, `email`; optional header row is detected and skipped.

## [0.0.1] - 2025-02-28

### Added

- SIP client registration to PBX with digest auth
- BLF (Busy Lamp Field) subscription via dialog event package for configured extensions
- NOTIFY handling with dialog-info XML parsing and state mapping (idle / ringing / busy → Graph availability)
- Microsoft Graph app-only auth (client credentials) and `setPresence` / optional `setStatusMessage`
- Extension → email (UPN) mapping with user object ID resolution and caching
- STUN-based public IP discovery when running behind NAT (`SIP_CONTACT_IP=auto`/`stun`)
- Configurable extensions list (`config/extensions.json`) and optional presence state persistence
