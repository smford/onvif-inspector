# onvif-inspector

> High-performance, cross-platform ONVIF IP camera discovery and real-time event diagnostic utility in Go.

`onvif-inspector` discovers ONVIF cameras on local subnets via WS-Discovery or direct unicast addressing, establishes long-polling WS-BaseNotification / PullPoint event subscriptions, compensates for clock drift, and provides real-time event diagnostics through an interactive Terminal User Interface (TUI), an embedded web dashboard with Server-Sent Events (SSE), and a JSON/NDJSON headless pipeline.

---

## Key Features

- **Zero Runtime Dependencies:** Pure Go implementation without CGO or Node.js/NPM dependencies. Self-contained static binary.
- **Robust WS-Security & Clock Skew Compensation:** Queries `GetSystemDateAndTime` without authentication to calculate camera time drift and applies offset to all WS-Security headers, eliminating clock drift authentication failures.
- **Defensive Firmware Handling:** Automatically rewrites internal camera URLs, normalizes XML responses, handles malformed namespaces, and uses exponential backoff with jitter on dropped sockets.
- **Multi-NIC WS-Discovery:** Broadcasts multicast probes across all active network interfaces simultaneously or binds to an explicit interface (`-i`), with unicast target fallback (`-t`).
- **Graceful Unsubscribe:** Intercepts `SIGINT`/`SIGTERM` to immediately send `Unsubscribe` SOAP calls to all active subscriptions, preventing camera connection limit exhaustion.
- **Decoupled Architecture & Bounded Ring Buffer:** Workers stream into a central pub/sub hub with a 1,000-event bounded circular ring buffer retaining raw XML envelopes for diagnostics.
- **Push Notification Fallback (Webhook):** Automatically detects cameras with defective `PullMessages` implementations (e.g., TP-Link Tapo C720) that drop TCP sockets with EOF. Seamlessly falls back to WS-BaseNotification push subscriptions (`wsnt:Subscribe`), spinning up an embedded HTTP webhook receiver to capture incoming `<wsnt:Notify>` payloads.
- **Interactive TUI:** 3-pane responsive terminal layout built with Bubble Tea and Lip Gloss.
- **Embedded Web Dashboard:** Single-page interface bundled via `//go:embed` with real-time SSE streaming (`text/event-stream`), camera statuses, and raw SOAP payload inspection.
- **Headless Shell Pipeline:** Outputs structured JSON Lines (`ndjson`) or formatted text for piping into `jq` or log collectors.

---

## Installation & Makefile Targets

### Build with Make
```bash
# Build the onvif-inspector binary
make build

# Run all unit tests
make test

# View all available make targets
make help
```

The compiled binary will be placed at `./onvif-inspector`.

### Cross-Compilation
```bash
make cross-compile
```
Outputs static binaries for Linux (`amd64`/`arm64`), macOS (`amd64`/`arm64`), and Windows (`amd64`) in `./bin/`.

---

## Quick Start

### 1. Interactive Terminal UI (TUI)
Discover cameras and launch the interactive 3-pane interface:
```bash
# Using config.yaml for credentials:
./onvif-inspector

# Or providing credentials via CLI:
./onvif-inspector -u admin -p MySecretPassword
```

### 2. Configuration File & CLI Management

Credentials and camera targets can be managed automatically in `config.yaml` using either the **Web UI** or the **CLI**.

#### CLI Config Commands
```bash
# Set default credentials for auto-discovered cameras:
./onvif-inspector config set-default -u admin -p SecretPassword

# Set credentials and friendly name for a specific camera (by IP or IP:port):
./onvif-inspector config set -t 192.168.1.100:80 -n "Front Door" -u camuser -p CamSecret

# List all configured cameras, names, and default credentials:
./onvif-inspector config list

# Remove a camera from configuration:
./onvif-inspector config remove -t 192.168.1.100:80

# Specify a custom config file path with -c / --config:
./onvif-inspector config list -c /path/to/cameras.yaml
```

You can also run the inspector directly with credentials and save them automatically:
```bash
./onvif-inspector -t 192.168.1.100:80 -u admin -p SecretPassword --save-config
```

#### Web UI Settings
When running with `--web`, click the **⚙️ Settings** button in the header or the **⚙️** icon on any camera card to view and edit camera names and credentials. Saving updates `config.yaml` immediately and triggers a live camera re-authentication without restarting the daemon. Camera names appear prominently across camera cards and the live event feed.

#### Manual `config.yaml` Structure
You can also manually edit `config.yaml` (see `config.yaml.example`):
```yaml
# Default credentials for discovered cameras
default:
  username: "admin"
  password: "DefaultPassword"

# Per-camera specific names and credentials (by IP, IP:port, or endpoint UUID)
cameras:
  - name: "Front Door"
    ip: "192.168.1.100"
    username: "admin"
    password: "FrontDoorPassword"
  - name: "Backyard"
    target: "192.168.1.105:8000"
    username: "operator"
    password: "BackyardPassword"
```

### 3. Direct Unicast Target (Bypass Multicast Discovery)
Connect directly to a camera by IP and port:
```bash
./onvif-inspector -t 192.168.1.100:80 -u admin -p MySecretPassword
```

### 4. TUI with Embedded Web Dashboard
Run the TUI while concurrently serving the embedded web UI on port 8080:
```bash
./onvif-inspector --web --web-port 8080
```
Open [http://localhost:8080](http://localhost:8080) in your browser.

### 5. Headless Web Server (No TUI)
Run as a background web monitoring service:
```bash
./onvif-inspector --headless --web --web-port 8080
```

### 6. Headless JSON Streaming (Pipeline to `jq`)
Stream normalized real-time events as JSON lines to stdout:
```bash
./onvif-inspector --headless --format json | jq .
```

---

## CLI Flags

```text
  -c, --config string        Path to YAML configuration file (default: config.yaml if present)
  -i, --interface string     Network interface for WS-Discovery (e.g. eth0, en0)
  -t, --target string        Direct camera address bypass discovery (e.g. 192.168.1.100:80)
  -u, --username string      ONVIF username (default: "admin")
  -p, --password string      ONVIF password
      --web                  Enable embedded web dashboard concurrently with TUI
      --web-port int         Port for embedded web dashboard (default: 8080)
      --webhook-port int     Local port for ONVIF push webhook fallback (0 = automatic free port)
      --headless             Run without TUI (stdout streaming or web-only mode)
      --format string        Output format for headless stdout: text, json (default: "text")
      --debug                Log raw SOAP requests and responses to stderr
  -v                         Print version and exit
```

---

## TUI Keyboard Shortcuts

| Key | Action |
| --- | --- |
| `Tab` | Cycle active pane focus (Devices → Live Feed → Inspector) |
| `Shift+Tab` | Cycle active pane backwards |
| `↑` / `k` | Move selection up in active pane |
| `↓` / `j` | Move selection down in active pane |
| `c` / `e` | Open camera settings dialog for highlighted camera |
| `a` | Add new camera target |
| `d` | Configure default camera credentials |
| `Enter` | Filter Live Feed to selected camera (toggle off in Devices pane) |
| `/` | Open search/filter bar (filter by topic, IP, or category) |
| `Esc` | Clear camera/search filter or close settings modal |
| `Backspace` / `A` | Clear camera filter to show all cameras (in Devices pane) |
| `Space` | Pause / resume live event stream |
| `x` | Toggle raw SOAP XML inspection mode in Inspector |
| `Home` / `g` | Jump to first item |
| `End` / `G` | Jump to latest item |
| `q` / `Ctrl+C` | Gracefully unsubscribe and exit |

---

## REST & SSE API Endpoints

When `--web` or `--web-port` is enabled, the embedded HTTP server provides:

- `GET /`: Embedded single-page dashboard.
- `GET /api/devices`: Returns JSON array of discovered cameras, latency, and subscription states.
- `GET /api/events`: Returns historical buffered events (up to 1,000 items) with raw SOAP envelopes.
- `GET /api/events/stream`: Server-Sent Events (SSE) streaming `event: onvif-event` and `event: device-update`.

---

## Project Layout

```text
.
├── cmd/
│   └── onvif-inspector/    # Application entrypoint & worker coordination
│       └── main.go
├── internal/
│   ├── config/             # YAML configuration persistence & CLI subcommands
│   │   ├── config.go
│   │   └── config_test.go
│   ├── discovery/          # Multi-NIC UDP WS-Discovery & unicast fallback
│   │   ├── probe.go
│   │   ├── probe_test.go
│   │   └── types.go
│   ├── onvif/              # ONVIF SOAP client, WS-Security, PullPoint & Push Fallback
│   │   ├── client.go
│   │   ├── client_test.go
│   │   ├── clock.go
│   │   ├── clock_test.go
│   │   ├── events.go
│   │   ├── events_test.go
│   │   ├── push.go
│   │   ├── push_test.go
│   │   ├── security.go
│   │   ├── security_test.go
│   │   └── types.go
│   ├── hub/                # Decoupled pub/sub event bus & 1000-item ring buffer
│   │   ├── bus.go
│   │   ├── bus_test.go
│   │   └── ringbuffer.go
│   ├── tui/                # Charmbracelet Bubble Tea / Lip Gloss 3-pane TUI
│   │   ├── model.go
│   │   ├── styles.go
│   │   ├── tui_test.go
│   │   ├── update.go
│   │   └── view.go
│   └── web/                # HTTP server & Server-Sent Events broker
│       ├── server.go
│       ├── server_test.go
│       └── sse.go
├── web/                    # Static frontend embedded via //go:embed
│   ├── assets.go
│   ├── app.js
│   ├── index.html
│   └── style.css
├── Makefile
├── goreleaser.yaml
├── go.mod
└── go.sum
```