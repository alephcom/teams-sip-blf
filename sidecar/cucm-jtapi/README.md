# CUCM JTAPI sidecar

Linux sidecar that observes CUCM 15 line state via **JTAPI** and POSTs changes to the Go sync service (`PROVIDER=cucm`).

```
CUCM CTI Manager (TCP 2748)
        ↓ JTAPI
  this sidecar
        ↓ POST /v1/line-state
  sip-blf-sync (Go) → Microsoft Graph setPresence
```

## Prerequisites

- JDK 17+
- Maven 3.8+
- CUCM 15 Application User with **Standard CTI Enabled** and Controlled Devices set
- Network: this host → CUCM CTI Manager **TCP 2748** (non-secure v1)
- Cisco JTAPI Client for Linux matching your CUCM 15 SU

## Install JTAPI jars

1. CUCM Admin → **Application → Plugins** → download **Cisco JTAPI Client for Linux** (`CiscoJTAPILinux.zip`).
2. Unpack and copy `jtapi.jar` (plus any other jars from the zip) into [`lib/`](lib/).
3. See [`lib/README.md`](lib/README.md).

## Configuration (environment)

| Variable | Description | Default |
|---|---|---|
| `CUCM_HOSTS` | Comma-separated CTI Manager hosts | required (`CUCM_HOST` also accepted) |
| `CUCM_USERNAME` | Application User userid | required |
| `CUCM_PASSWORD` | Application User password | required |
| `CUCM_EXTENSIONS` | Comma-separated DNs to watch | empty = all provider addresses |
| `CUCM_EXTENSIONS_FILE` | Path to `extensions.json`, `.txt`, or `.csv` | optional |
| `CUCM_EVENT_URL` | Go ingress URL | `http://127.0.0.1:8090/v1/line-state` |
| `CUCM_EVENT_TOKEN` | Shared secret (`X-CUCM-Token`) | empty |
| `CUCM_RECONNECT_MS` | Delay between reconnect attempts | `5000` |

Prefer setting `CUCM_EXTENSIONS` or `CUCM_EXTENSIONS_FILE` to the same DNs as the Go `extensions.json`.

## Run

```bash
# Terminal 1 — Go sync
export PROVIDER=cucm
export CUCM_EVENT_LISTEN=127.0.0.1:8090
export CUCM_EVENT_TOKEN=changeme
# ... Azure + EXTENSIONS_JSON ...
./bin/sip-blf-sync

# Terminal 2 — sidecar
export CUCM_HOSTS=cucm.example.com
export CUCM_USERNAME=teams-presence-cti
export CUCM_PASSWORD=secret
export CUCM_EXTENSIONS_FILE=../../config/extensions.json
export CUCM_EVENT_URL=http://127.0.0.1:8090/v1/line-state
export CUCM_EVENT_TOKEN=changeme
chmod +x run.sh
./run.sh
```

## State mapping

| Line condition | Posted `state` | Teams (via Go) |
|---|---|---|
| Idle | `idle` | Available |
| Ringing / alerting | `ringing` | Busy + InACall |
| Talking / held | `busy` | Busy + InACall |

## Secure CTI (follow-up)

v1 uses non-secure port **2748**. TLS (**2749**) needs mixed mode, CAPF on the app user, and **Standard CTI Secure Connection** — not configured by this sidecar yet.
