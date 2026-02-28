# teams_freepbx

**Version:** 0.0.2

SIP BLF → Microsoft Teams presence sync: a small service that registers to a SIP endpoint (e.g. FreePBX/Asterisk), subscribes to BLF (Busy Lamp Field) for a list of extensions, and updates each user's Teams presence in Microsoft Graph when their line state changes.

**Notice: This is a proof of concept.** It is not officially supported and may be unsuitable for production use. Use at your own risk.

## Overview

- **SIP client**: Registers to the PBX (From header uses SIP username and server host so the PBX can match the peer) and sends SUBSCRIBE (dialog event package) for each extension in config. Handles 401 digest auth on SUBSCRIBE.
- **BLF**: On NOTIFY, parses dialog-info XML and maps state (idle / ringing / busy) to Graph availability (Available / Busy).
- **Graph**: Uses app-only auth (client credentials). Resolves each extension’s email (UPN) to the user’s object ID (GUID) via `GET /users/{upn}` (cached), then calls `setPresence` with the application ID as `sessionId`. Optionally `setStatusMessage`.
- **STUN**: When `SIP_CONTACT_IP` is `auto`/`stun`/empty, uses a simple STUN binding request to discover the public IP:port for the Contact header.

## Prerequisites

- Go 1.21+
- A SIP endpoint (FreePBX, Asterisk, etc.) with BLF/dialog event support and a SIP account for the client. The subscribing endpoint’s context must match the dialplan hints (e.g. `ext-local`).
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

### 2. Environment

Copy `.env.example` to `.env` and set:


| Variable              | Description                                                                                                                       |
| --------------------- | --------------------------------------------------------------------------------------------------------------------------------- |
| `SIP_SERVER`          | PBX host:port (e.g. `192.168.1.1:5060`)                                                                                           |
| `SIP_TRANSPORT`       | `udp` or `tcp`                                                                                                                    |
| `SIP_USERNAME`        | SIP username for REGISTER                                                                                                         |
| `SIP_PASSWORD`        | SIP password                                                                                                                      |
| `SIP_CONTACT_IP`      | Your host IP for the Contact header (must be reachable by the PBX). Use `auto` or `stun` to discover via STUN when behind NAT.    |
| `STUN_SERVERS`        | Comma-separated STUN servers for NAT discovery (default: Google STUN servers). Used when `SIP_CONTACT_IP` is `auto`/`stun`/empty. |
| `AZURE_TENANT_ID`     | Azure AD tenant ID                                                                                                                |
| `AZURE_CLIENT_ID`     | App (client) ID                                                                                                                   |
| `AZURE_CLIENT_SECRET` | Client secret                                                                                                                     |
| `EXTENSIONS_JSON`     | Path to extensions file (default: `config/extensions.json`)                                                                       |
| `PRESENCE_STATE_JSON` | Path to session ID state file (default: `config/presence-state.json`)                                                             |
| `SIP_LISTEN`          | Address to bind for NOTIFY (default: `0.0.0.0:5060` when using STUN, else `SIP_CONTACT_IP:5060`)                                  |


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

## Build and run

```bash
go build -o bin/sip-blf-sync ./cmd/sip-blf-sync/
./bin/sip-blf-sync
```

Or:

```bash
go run ./cmd/sip-blf-sync/
```

The service will:

1. Load extensions (and optional state file).
2. Register to the SIP server (with digest auth if challenged).
3. SUBSCRIBE to BLF (dialog) for each extension (with digest auth if the PBX challenges SUBSCRIBE).
4. Listen for NOTIFY; on each NOTIFY, parse state, resolve the user’s email to object ID if needed, and call Graph `setPresence` for that user. The application ID is used as `sessionId` for app-only presence.

## Project layout

- `cmd/sip-blf-sync/` – main entrypoint and config loading.
- `internal/sip/` – SIP registration and BLF SUBSCRIBE/NOTIFY (sipgo).
- `internal/blf/` – BLF NOTIFY body parsing (dialog-info) and state → Graph availability mapping.
- `internal/graph/` – Azure auth, state file, and Microsoft Graph `setPresence` / `setStatusMessage`.
- `config/extensions.json` – extension → email mapping.
- `config/presence-state.json` – optional state file (used for persistence if needed).

## Versioning

This project uses [Semantic Versioning](https://semver.org/spec/v2.0.0.html) (SemVer): **MAJOR.MINOR.PATCH**.

- **MAJOR** – Incompatible API or behaviour changes (e.g. config format, breaking CLI or behaviour).
- **MINOR** – New features or behaviour added in a backward-compatible way.
- **PATCH** – Backward-compatible bug fixes and small improvements.

The current version is recorded in the [VERSION](VERSION) file and in the [CHANGELOG](CHANGELOG.md). Pre-1.0.0 versions (e.g. 0.0.x, 0.1.x) are considered initial development; the public API and behaviour may still change.

## License

MIT License. See [LICENSE](LICENSE).