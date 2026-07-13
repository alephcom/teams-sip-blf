# SIP / CUCM Line State to Microsoft Teams Presence Sync

**teams-sip-blf** — Sync Microsoft Teams presence from your PBX using SIP BLF (FreePBX/Asterisk) or CUCM CTI (JTAPI sidecar), via Microsoft Graph.

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE) [![Go 1.21+](https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go)](https://go.dev/)

**Version:** 0.0.4

A small service that observes extension line state and updates each user's **Microsoft Teams presence** in **Microsoft Graph**. Supports:

- **SIP BLF** (`PROVIDER=sip`, default) — FreePBX, Asterisk, or any PBX with dialog-event BLF
- **CUCM CTI** (`PROVIDER=cucm`) — CUCM 15 JTAPI sidecar on Linux posts line-state events to this service

**Notice: This is a proof of concept.** It is not officially supported and may be unsuitable for production use. Use at your own risk.

## What it does

- **SIP BLF** — Subscribes to Busy Lamp Field (dialog event package) on your PBX and maps extension state to presence.
- **CUCM CTI** — Receives line-state events from a JTAPI sidecar (idle / ringing / busy) and maps them the same way.
- **Microsoft Teams presence** — Updates each user's availability (Available / Busy) in Teams via the Microsoft Graph API.
- **FreePBX, Asterisk, CUCM** — SIP path for Asterisk/FreePBX; CTI path for Cisco Unified CM 15.

## How it works

- **Provider interface**: `PROVIDER=sip` or `PROVIDER=cucm` both call the same handler → Graph `setPresence`.
- **SIP client**: Registers to the PBX and sends SUBSCRIBE (dialog event package) for each extension. Handles 401 digest auth.
- **CUCM ingress**: Listens on `CUCM_EVENT_LISTEN` for `POST /v1/line-state` from [`sidecar/cucm-jtapi`](sidecar/cucm-jtapi/README.md).
- **BLF / line state**: Maps state (idle / ringing / busy) to Graph availability (Available / Busy). HOLD is treated as busy.
- **Graph**: App-only auth (client credentials). Resolves email (UPN) to object ID, then `setPresence`.
- **STUN**: When `SIP_CONTACT_IP` is `auto`/`stun`/empty, discovers public IP:port for the Contact header (SIP provider only).

## Prerequisites

- Go 1.21+
- **SIP mode:** A SIP endpoint with BLF/dialog support and a SIP account. Dialplan hints must exist (e.g. `ext-local`).
- **CUCM mode:** CUCM 15, CTI Manager, Application User with Standard CTI Enabled + Controlled Devices, JTAPI Linux client jars, JDK 17+ for the sidecar. See [CUCM CTI setup](#cucm-cti-setup-providercucm).
- An Azure AD app registration with **Application** permissions `Presence.ReadWrite.All` and `User.ReadBasic.All`, with admin consent granted.

## Configuration

### 1. Extensions and emails

Edit `config/extensions.json` (or set `EXTENSIONS_JSON` to another path):

```json
[
  { "extension": "1001", "email": "user1@contoso.com" },
  { "extension": "1002", "email": "user2@contoso.com" }
]
```

If the JSON file does not exist, the app will try the same path with `.json` replaced by `.csv` (e.g. `config/extensions.csv`). The CSV format is two columns: `extension`, `email`. A header row `extension,email` is optional (case-insensitive) and will be skipped.

Each `email` is the user’s sign-in (userPrincipalName); the app resolves it to the Graph object ID (GUID) for setPresence.

**Alternatively**, set `VOICEMAIL_CONF` to the path of an Asterisk/FreePBX `voicemail.conf`. When set, the app loads extension and email from that file instead of `EXTENSIONS_JSON`. It parses context sections (e.g. `[default]`) for mailbox lines in the form `extension=password,name,email,...`; the third comma-separated field is used as email. If that field contains multiple addresses separated by `|`, the first is used. The `[general]` section is skipped. This is intended for deployments where the app is installed directly on the Asterisk/FreePBX server and can read the existing voicemail configuration.

### 2. Environment

Copy `.env.example` to `.env` and set:


| Variable              | Description                                                                                                                       |
| --------------------- | --------------------------------------------------------------------------------------------------------------------------------- |
| `PROVIDER`            | `sip` (default) or `cucm`                                                                                                         |
| `SIP_SERVER`          | PBX host:port (e.g. `192.168.1.1:5060`). Used when `PROVIDER=sip`.                                                                |
| `SIP_TRANSPORT`       | `udp` or `tcp`                                                                                                                    |
| `SIP_USERNAME`        | SIP username for REGISTER                                                                                                         |
| `SIP_PASSWORD`        | SIP password                                                                                                                      |
| `SIP_CONTACT_IP`      | Your host IP for the Contact header (must be reachable by the PBX). Use `auto` or `stun` to discover via STUN when behind NAT.    |
| `STUN_SERVERS`        | Comma-separated STUN servers for NAT discovery (default: Google STUN servers). Used when `SIP_CONTACT_IP` is `auto`/`stun`/empty. |
| `SIP_LISTEN`          | Address to bind for NOTIFY (default: `0.0.0.0:5060` when using STUN, else `SIP_CONTACT_IP:5060`)                                  |
| `CUCM_EVENT_LISTEN`   | HTTP listen address for JTAPI sidecar events when `PROVIDER=cucm` (default: `127.0.0.1:8090`)                                      |
| `CUCM_EVENT_TOKEN`    | Optional shared secret; require header `X-CUCM-Token` on POSTs                                                                    |
| `AZURE_TENANT_ID`     | Azure AD tenant ID                                                                                                                |
| `AZURE_CLIENT_ID`     | App (client) ID                                                                                                                   |
| `AZURE_CLIENT_SECRET` | Client secret                                                                                                                     |
| `EXTENSIONS_JSON`     | Path to extensions file (default: `config/extensions.json`). Ignored when `VOICEMAIL_CONF` is set.                                |
| `VOICEMAIL_CONF`      | Optional. Path to Asterisk voicemail.conf; when set, extension/email are read from it instead of JSON/CSV.                       |
| `PRESENCE_STATE_JSON` | Path to session ID state file (default: `config/presence-state.json`)                                                             |


### 3. Azure app registration

1. In [Microsoft Entra admin center](https://entra.microsoft.com/) → **App registrations** → **New registration**.
2. Add **Application** permissions: **Microsoft Graph** → **Presence.ReadWrite.All** and **User.ReadBasic.All**. User.ReadBasic.All is used to resolve email/UPN to user object ID (GUID) for setPresence. After assigning these permissions to the app, you must **grant admin consent** (e.g. in **API permissions** → **Grant admin consent for [your tenant]**).
3. Under **Certificates & secrets**, create a **Client secret** and use it as `AZURE_CLIENT_SECRET`.
4. Use **Overview** → Application (client) ID and Directory (tenant) ID for `AZURE_CLIENT_ID` and `AZURE_TENANT_ID`.

### 4. Behind NAT (STUN)

When the sync service runs behind NAT, set `SIP_CONTACT_IP=auto` (or `stun` or leave empty). The app will use the configured `STUN_SERVERS` to discover your public IP and port and put them in the SIP Contact header so the PBX can send NOTIFYs back. Ensure your router forwards UDP (and TCP if used) port 5060 to the host running the app. `SIP_LISTEN` defaults to `0.0.0.0:5060` in this case so the app binds on all interfaces.

### 5. FreePBX / Asterisk (BLF)

- Create a SIP device or extension that the sync service will use for REGISTER (e.g. `blf-client`).
- Ensure the PBX supports the **dialog** event package for BLF (RFC 4235). Many Asterisk/FreePBX setups use `dialog` for BLF.
- Allow the sync service’s IP to register and receive NOTIFY; open firewall for the port you use (e.g. 5060) if the PBX is remote.

**If SUBSCRIBE returns 404** for an extension, the PBX likely has no BLF/dialog target for that extension. On Asterisk (PJSIP): load `res_pjsip_pubsub`, `res_pjsip_dialog_info_body_generator`, and `res_pjsip_exten_state`; set `allow_subscribe=yes` on the endpoint; and define **dialplan hints** so the extension has a presence target (e.g. in `extensions.conf`: `exten => 500,hint,PJSIP/500` or the correct endpoint). Without a hint for that extension, SUBSCRIBE to `sip:500@pbx` returns 404. The sync app will log a warning and continue; other extensions may still work.

### 6. CUCM CTI setup (`PROVIDER=cucm`)

For Cisco Unified CM 15 on a Linux sync host, use CTI (JTAPI) instead of SIP BLF.

**CUCM admin**

1. Start **Cisco CTIManager** on the node(s) you will use (Serviceability → Control Center – Feature Services).
2. Create an **Application User** (e.g. `teams-presence-cti`) with role **Standard CTI Enabled**.
3. Associate **Controlled Devices** for each phone whose DN appears in `extensions.json`.
4. Allow the Linux host to reach CTI Manager on **TCP 2748** (non-secure; TLS 2749 is a follow-up).
5. Download **Cisco JTAPI Client for Linux** from Admin → **Application → Plugins** (`CiscoJTAPILinux.zip`) for your 15.x SU. Place `jtapi.jar` in `sidecar/cucm-jtapi/lib/` (see that directory’s README).

No separate Cisco “JTAPI” or Contact Center license is required for observe-only line state if phones are already licensed on the cluster.

**Run both processes**

```bash
# Go sync (ingress)
export PROVIDER=cucm
export CUCM_EVENT_LISTEN=127.0.0.1:8090
export CUCM_EVENT_TOKEN=changeme
# ... Azure + EXTENSIONS_JSON ...
./bin/sip-blf-sync

# JTAPI sidecar (see sidecar/cucm-jtapi/README.md)
export CUCM_HOSTS=cucm.example.com
export CUCM_USERNAME=teams-presence-cti
export CUCM_PASSWORD=secret
export CUCM_EXTENSIONS_FILE=config/extensions.json
export CUCM_EVENT_URL=http://127.0.0.1:8090/v1/line-state
export CUCM_EVENT_TOKEN=changeme
./sidecar/cucm-jtapi/run.sh
```

Event body: `{"extension":"1001","state":"idle|ringing|busy"}`.

## Build and run

Pre-built binaries for Linux (amd64) and Windows (amd64) are attached to each [release](https://github.com/alephcom/teams-sip-blf/releases) as `sip-blf-sync-linux-amd64` and `sip-blf-sync-windows-amd64.exe`.

To build from source:

```bash
go build -o bin/sip-blf-sync ./cmd/sip-blf-sync/
./bin/sip-blf-sync
```

Or:

```bash
go run ./cmd/sip-blf-sync/
```

With `PROVIDER=sip` (default) the service will:

1. Load extensions (and optional state file).
2. Register to the SIP server (with digest auth if challenged).
3. SUBSCRIBE to BLF (dialog) for each extension.
4. On each NOTIFY, map state and call Graph `setPresence`.

With `PROVIDER=cucm` it listens for sidecar POSTs and uses the same Graph path (no SIP).

## Project layout

- `cmd/sip-blf-sync/` – main entrypoint and config loading.
- `internal/provider/` – line-state provider interface and SIP adapter.
- `internal/sip/` – SIP registration and BLF SUBSCRIBE/NOTIFY (sipgo).
- `internal/cucm/` – HTTP ingress for JTAPI sidecar events.
- `internal/blf/` – state parsing/mapping and state → Graph availability.
- `internal/graph/` – Azure auth, state file, and Microsoft Graph `setPresence` / `setStatusMessage`.
- `sidecar/cucm-jtapi/` – Java JTAPI sidecar for CUCM 15 (Linux).
- `config/extensions.json` – extension → email mapping (or set `VOICEMAIL_CONF` to an Asterisk voicemail.conf path).
- `config/presence-state.json` – optional state file (used for persistence if needed).

## Versioning

This project uses [Semantic Versioning](https://semver.org/spec/v2.0.0.html) (SemVer): **MAJOR.MINOR.PATCH**.

- **MAJOR** – Incompatible API or behaviour changes (e.g. config format, breaking CLI or behaviour).
- **MINOR** – New features or behaviour added in a backward-compatible way.
- **PATCH** – Backward-compatible bug fixes and small improvements.

The current version is recorded in the [VERSION](VERSION) file and in the [CHANGELOG](CHANGELOG.md). Pre-1.0.0 versions (e.g. 0.0.x, 0.1.x) are considered initial development; the public API and behaviour may still change.

## Related

- [Microsoft Graph presence API](https://learn.microsoft.com/en-us/graph/api/resources/presence) — Set user presence from an application.
- [SIP dialog event package (RFC 4235)](https://www.rfc-editor.org/rfc/rfc4235) — BLF/dialog-info for line state.
- [Cisco Unified JTAPI Developers Guide (Release 15)](https://www.cisco.com/c/en/us/td/docs/voice_ip_comm/cucm/jtapi_dev/15/cucm_b_cisco-unified-jtapi-developers-guide-15.html) — JTAPI for CUCM.
- [sidecar/cucm-jtapi/README.md](sidecar/cucm-jtapi/README.md) — CUCM JTAPI sidecar setup.

## License

MIT License. See [LICENSE](LICENSE).
