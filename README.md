<p align="center">
  <img src="logo.png" alt="Ganoid" width="160" />
</p>

<h1 align="center">Ganoid</h1>

<p align="center">
  A Tailscale coordination server profile manager for Windows.
  <br />
  Switch between self-hosted coordination servers without touching config files.
</p>

<p align="center">
  <a href="https://github.com/yashau/ganoid/releases/latest"><img src="https://img.shields.io/github/v/release/yashau/ganoid?style=flat-square" alt="Latest Release" /></a>
  <img src="https://img.shields.io/badge/platform-Windows-blue?style=flat-square" alt="Platform: Windows" />
  <img src="https://img.shields.io/badge/go-1.26+-00ADD8?style=flat-square&logo=go" alt="Go Version" />
</p>

---

## Overview

Ganoid is a two-component tool for managing multiple Tailscale profiles — each pointing at a different coordination server — without manual reconfiguration.

| Component | Description |
|-----------|-------------|
| `ganoidd` | Privileged daemon. Runs as a Windows service. Manages Tailscale state, serves the web UI and REST API. |
| `ganoid` | System tray client. Monitors `ganoidd`, shows connection status, and lets you switch profiles directly from the tray or the web UI. |

The two components communicate over a local HTTP API authenticated with a per-session bearer token. `ganoid` self-recovers if `ganoidd` restarts — whichever starts first is fine.

## Installation

Run the following in PowerShell (elevation is handled automatically):

```powershell
irm https://raw.githubusercontent.com/yashau/ganoid/main/install.ps1 | iex
```

The installer will:

1. Download the latest `ganoidd.exe` and `ganoid.exe` from GitHub Releases
2. Install `ganoidd` as a Windows service (auto-start, LocalSystem)
3. Add a startup shortcut for `ganoid` to your user login
4. Create Start Menu shortcuts
5. Start everything immediately

## Usage

### System tray

`ganoid` sits in the system tray and shows the current connection state. Right-clicking the icon gives you:

- **Status** — current Tailscale backend state and active profile name
- **Switch Profile** — submenu listing all configured profiles; click one to switch immediately
- **Open Dashboard** — opens the web UI in your browser
- **Quit** — exits the tray app (ganoidd keeps running as a service)

### Web UI

Open the dashboard via the tray icon. It includes:

- **Dashboard** — active profile, Tailscale backend state, peer count, and one-click profile switching with live progress streamed in real time
- **Profiles** — add, edit, and delete profiles
- **Settings** — configure the listening port and other options

## How it works

### Profile switching

Each profile maps a friendly name to a coordination server URL. Switching runs an 8-step sequence:

1. `tailscale logout` from the current server (best-effort)
2. Stop the Tailscale service
3. Back up the current Tailscale state directory
4. Clear the active state directory
5. Restore the target profile's saved state (if any)
6. Write the new login server to the registry
7. Start the Tailscale service
8. Update the active profile in Ganoid's config

Switch progress is streamed live in the web UI. The tray also triggers a switch directly — no need to open the browser.

### First login

After switching to a profile for the first time, Tailscale will be in the `NeedsLogin` state. Open the dashboard and follow the Tailscale login link to authenticate with the new coordination server.

## Uninstall

```powershell
irm https://raw.githubusercontent.com/yashau/ganoid/main/uninstall.ps1 | iex
```

Configuration data in `%APPDATA%\ganoid` is left intact.

## Building from source

**Prerequisites:** Go 1.26+, pnpm, goversioninfo

```powershell
# Windows
.\build.ps1 -Version 0.1.0

# All platforms
.\build.ps1 -Version 0.1.0 -Target all
```

```bash
# Linux / macOS
./build.sh 0.1.0
./build.sh 0.1.0 all
```

The build script compiles the SvelteKit UI, generates Windows version resources, and produces `ganoidd.exe` + `ganoid.exe` with version metadata embedded.

## License

Copyright (c) 2026 Ibrahim Yashau. All rights reserved.
