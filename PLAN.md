# Ganoid — Project Plan

> A cross-platform Tailscale coordination server profile manager.  
> Daemon + embedded web UI. Switch between Headscale instances and official Tailscale seamlessly.

---

## Name

| Artifact | Name |
|---|---|
| Project | Ganoid |
| Binary | `ganoid` |
| Daemon | `ganoidd` |
| Config dir (Windows) | `%APPDATA%\ganoid\` |
| Config dir (Linux/macOS) | `~/.config/ganoid\` |

---

## Problem Statement

Tailscale ties its node key to the coordination server. Switching between a self-hosted Headscale instance and the official Tailscale control plane — or between multiple Headscale servers — requires manually stopping the daemon, clearing state, editing registry/config, and restarting. Ganoid automates and wraps this into a profile-based UI accessible from the browser.

---

## Architecture Overview

```
┌─────────────────────────────────────────────┐
│                ganoid binary                │
│                                             │
│  ┌──────────────┐    ┌─────────────────┐   │
│  │  REST API    │    │  Embedded       │   │
│  │  (chi)       │◄───│  SvelteKit UI   │   │
│  │  :57400      │    │  (go:embed)     │   │
│  └──────┬───────┘    └─────────────────┘   │
│         │                                   │
│  ┌──────▼───────┐                           │
│  │   Manager    │                           │
│  │   Core       │                           │
│  └──────┬───────┘                           │
│         │                                   │
│  ┌──────▼───────┐    ┌─────────────────┐   │
│  │  Platform    │    │  Profile Store  │   │
│  │  Interface   │    │  (JSON file)    │   │
│  └──────┬───────┘    └─────────────────┘   │
│         │                                   │
│  ┌──────┴──────────────────────────┐        │
│  │  windows.go / linux.go /        │        │
│  │  darwin.go                      │        │
│  └─────────────────────────────────┘        │
└─────────────────────────────────────────────┘
         │
         ▼
   Tailscale daemon + CLI
```

Ganoid runs as a background process. On Windows it registers as a system tray app; on Linux/macOS it can be managed via systemd/launchd or run manually. The UI is served on `localhost:57400` and opened automatically on launch or via tray icon click.

---

## Repository Structure

```
ganoid/
├── cmd/
│   └── ganoid/
│       └── main.go                  # entrypoint
├── internal/
│   ├── api/
│   │   └── api.go                   # chi router, all HTTP handlers
│   ├── config/
│   │   └── config.go                # profile storage, R/W JSON config file
│   ├── manager/
│   │   └── manager.go               # switch sequence, orchestration logic
│   ├── platform/
│   │   ├── platform.go              # Platform interface definition
│   │   ├── windows.go               # registry, Windows service control
│   │   ├── linux.go                 # systemd unit control
│   │   └── darwin.go                # launchd control
│   └── tray/
│       └── tray.go                  # systray icon (open UI, quit)
├── ui/                              # SvelteKit app
│   ├── src/
│   │   ├── lib/
│   │   │   └── api.ts               # typed fetch wrappers for REST API
│   │   └── routes/
│   │       ├── +page.svelte         # dashboard (active profile, TS status)
│   │       ├── profiles/
│   │       │   └── +page.svelte     # profile list, add/edit/delete
│   │       └── settings/
│   │           └── +page.svelte     # port, startup, behaviour
│   ├── package.json
│   └── vite.config.ts
├── Makefile
├── go.mod
├── go.sum
└── README.md
```

---

## Platform Interface

```go
type Platform interface {
    StopService() error
    StartService() error
    ServiceStatus() (ServiceState, error)

    StateDirPath() string
    ProfileStateDirPath(profileID string) string

    SetLoginServer(url string) error
    GetLoginServer() (string, error)
    ClearLoginServer() error

    TailscaleBinaryPath() string
}
```

### Platform implementation notes

| Platform | Login server location | State dir | Service control |
|---|---|---|---|
| Windows | `HKLM\SOFTWARE\Tailscale IPN` → `LoginServer` | `%ProgramData%\Tailscale` | SCM / `golang.org/x/sys/windows/svc` |
| Linux | `/etc/default/tailscaled` or systemd drop-in | `/var/lib/tailscaled` | `systemctl` |
| macOS | `/Library/Preferences/io.tailscale.ipn.macos.plist` | `/Library/Tailscale` | `launchctl` |

> **Elevation note:** Windows registry writes and service control require admin privileges. Ganoid must either run elevated or use a privileged helper. Run-as-administrator on first launch is the simplest v1 approach.

---

## Profile Data Model

```json
{
  "active_profile_id": "headscale-home",
  "profiles": [
    {
      "id": "official",
      "name": "Tailscale Official",
      "login_server": "",
      "created_at": "2026-01-01T00:00:00Z",
      "last_used": "2026-03-10T12:00:00Z"
    },
    {
      "id": "headscale-home",
      "name": "Home Headscale",
      "login_server": "https://headscale.example.com",
      "created_at": "2026-01-15T00:00:00Z",
      "last_used": "2026-03-28T08:00:00Z"
    }
  ]
}
```

- `login_server` empty string = official Tailscale (`controlplane.tailscale.com`)
- Stored at platform config dir, not inside Tailscale's state dir
- Profile state backups stored at `<config_dir>/states/<profile_id>/`

---

## Profile Switch Sequence

```
1.  tailscale logout          → gracefully deregisters node from current server
2.  Stop Tailscale daemon
3.  Copy active state dir     → backup to states/<current_profile_id>/
4.  Clear active state dir
5.  Restore states/<target_profile_id>/ → active state dir (if exists)
6.  Write login server        → registry / config file
7.  Start Tailscale daemon
8.  Update active_profile_id  → config JSON
```

- If target profile has no saved state, step 5 is skipped → daemon starts fresh → new node key → user hits browser auth flow once
- Step 1 (`tailscale logout`) is best-effort; failure is non-fatal (daemon may already be down)

---

## REST API

Base: `http://localhost:57400/api`

| Method | Path | Description |
|---|---|---|
| `GET` | `/status` | Active profile, Tailscale connection state, version |
| `GET` | `/profiles` | List all profiles |
| `POST` | `/profiles` | Create profile |
| `PUT` | `/profiles/:id` | Update profile (name, login_server) |
| `DELETE` | `/profiles/:id` | Delete profile (cannot delete active) |
| `POST` | `/profiles/:id/switch` | Activate profile — triggers full switch sequence |
| `GET` | `/tailscale/status` | Raw `tailscale status --json` passthrough |

All responses: `Content-Type: application/json`. Switch endpoint streams progress via SSE or returns immediately with a job ID (TBD in implementation).

---

## UI Screens

### 1. Dashboard (`/`)
- Active profile badge with login server URL
- Tailscale connection state (Connected / Needs Login / Stopped)
- Peer count
- One-click switch buttons for all other profiles
- Link to open Tailscale auth URL if needed

### 2. Profiles (`/profiles`)
- Table: name, login server, last used, actions
- Add/edit modal: name field + login server URL (blank = official)
- Delete with confirmation (blocked if profile is active)

### 3. Settings (`/settings`)
- HTTP port (default 57400)
- Open browser on start toggle
- Start with system toggle (writes autostart entry)

---

## Build Pipeline

```makefile
ui:
	cd ui && npm ci && npm run build      # outputs to ui/dist/

build: ui
	go build -o ganoid ./cmd/ganoid

build-windows: ui
	GOOS=windows GOARCH=amd64 go build -o ganoid.exe ./cmd/ganoid

build-linux: ui
	GOOS=linux GOARCH=amd64 go build -o ganoid-linux ./cmd/ganoid

build-darwin: ui
	GOOS=darwin GOARCH=arm64 go build -o ganoid-darwin ./cmd/ganoid
```

SvelteKit `dist/` embedded via:
```go
//go:embed ui/dist
var uiFiles embed.FS
```

---

## Systray Menu Structure

```
Ganoid
├── Status: Connected (Home Headscale)   [disabled label, updates dynamically]
├── ─────────────────────────────────
├── Switch Profile ▶
│   ├── ✓ Home Headscale                 [active profile, checkmark]
│   ├──   Tailscale Official
│   └──   Work Headscale
├── ─────────────────────────────────
├── Open Dashboard
└── Quit
```

### Implementation notes

- Submenu entries are built at startup by ranging over loaded profiles
- The active profile gets a checkmark prefix in its label (`✓ Name` vs `  Name`)
- The status label at the top reflects live Tailscale connection state, polled on an interval
- **Menu rebuild on profile change:** `getlantern/systray` does not support removing submenu items after creation. The cleanest approach is to restart the tray goroutine whenever the profile list changes (add/delete/rename). Since profile changes are infrequent this is acceptable. The API server signals the tray via an internal channel when a mutation occurs.
- Clicking a submenu item triggers the same switch sequence as `POST /api/profiles/:id/switch`
- The active profile cannot be re-switched (item shown disabled or skipped)

---

## Dependencies

### Go
| Package | Purpose |
|---|---|
| `github.com/go-chi/chi/v5` | HTTP router |
| `github.com/getlantern/systray` | Cross-platform systray |
| `golang.org/x/sys` | Windows service + registry access |

### UI (SvelteKit)
| Package | Purpose |
|---|---|
| `@sveltejs/kit` | Framework |
| `vite` | Build tool |
| No component library | Keep the bundle lean; hand-rolled UI |

---

## Phased Delivery

| Phase | Scope | Notes |
|---|---|---|
| **1** | Go module init, Platform interface, Windows implementation (registry R/W, service stop/start, state dir paths) | Windows-first, get the hard part done |
| **2** | Profile store (JSON R/W), switch sequence logic, `tailscale` CLI shelling | Core functionality |
| **3** | REST API (chi), serve static placeholder at `/` | Backend complete, testable via curl |
| **4** | SvelteKit UI — dashboard + profiles screens | Frontend |
| **5** | Embed UI into binary, single-binary build, Makefile | Packaging |
| **6** | Linux platform implementation (systemd) | Cross-platform |
| **7** | macOS platform implementation (launchd) | Cross-platform |
| **8** | Systray icon (Windows + Linux + macOS) | Polish |
| **9** | Settings screen, autostart, installer/packaging | Distribution |

Start with Phase 1–5 for a working Windows build. Phases 6–9 can follow.

---

## Known Constraints & Gotchas

- **Node key is server-bound.** There is no way to reuse a node key across coordination servers. Switching always means a new identity on the target server unless state was previously saved.
- **State dir permissions.** On Windows, `%ProgramData%\Tailscale` is owned by SYSTEM. Ganoid needs to run elevated or use a service helper to copy/clear it.
- **Logout is graceful, not required.** If the daemon is already stopped or unreachable, skipping logout is safe. The old node will expire on the coordination server eventually.
- **Official Tailscale vs Headscale feature parity.** Some Tailscale features (e.g. MagicDNS, certain ACL features) behave differently on Headscale. Ganoid is coordination-server-agnostic and makes no assumptions about features.
- **Tailscale Desktop app vs daemon.** On Windows, Tailscale ships both a GUI app and `tailscaled`. If the user has the GUI app installed, the service name and state path are the same — no special handling needed, but the GUI app may conflict if open during a switch.
- **Port 57400.** Chosen to avoid common conflicts. Should be configurable.
