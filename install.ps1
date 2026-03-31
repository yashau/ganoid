#Requires -RunAsAdministrator
<#
.SYNOPSIS
    Ganoid installer — downloads binaries from GitHub releases and configures the system.

.DESCRIPTION
    Downloads ganoidd.exe and ganoid.exe from the latest GitHub release, installs
    ganoidd as a Windows service (auto-start, LocalSystem), and sets ganoid up to
    run at user login via the Startup folder.

.EXAMPLE
    irm https://raw.githubusercontent.com/yashau/ganoid/main/install.ps1 | iex
#>

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# ── Config ────────────────────────────────────────────────────────────────────
$Repo       = 'yashau/ganoid'
$InstallDir = "$env:ProgramFiles\Ganoid"
$ServiceName = 'ganoidd'
$ServiceDisplayName = 'Ganoid Daemon'
$ServiceDesc = 'Tailscale profile coordination daemon for Ganoid'

# ── Helpers ───────────────────────────────────────────────────────────────────
function Write-Step([string]$Msg) {
    Write-Host "`n==> $Msg" -ForegroundColor Cyan
}

function Write-OK([string]$Msg) {
    Write-Host "    OK  $Msg" -ForegroundColor Green
}

function Write-Warn([string]$Msg) {
    Write-Host "    WARN $Msg" -ForegroundColor Yellow
}

# ── Fetch latest release ───────────────────────────────────────────────────────
Write-Step "Fetching latest release from github.com/$Repo"

$apiUrl = "https://api.github.com/repos/$Repo/releases/latest"
$headers = @{ 'User-Agent' = 'ganoid-installer/1.0' }

try {
    $release = Invoke-RestMethod -Uri $apiUrl -Headers $headers
} catch {
    Write-Error "Failed to fetch release info: $_"
    exit 1
}

$tag = $release.tag_name
Write-OK "Latest release: $tag"

function Get-AssetUrl([string]$Name) {
    $asset = $release.assets | Where-Object { $_.name -eq $Name } | Select-Object -First 1
    if (-not $asset) {
        Write-Error "Asset '$Name' not found in release $tag"
        exit 1
    }
    return $asset.browser_download_url
}

$ganoidd_url = Get-AssetUrl 'ganoidd.exe'
$ganoid_url  = Get-AssetUrl 'ganoid.exe'

# ── Create install directory ───────────────────────────────────────────────────
Write-Step "Installing to $InstallDir"

if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir | Out-Null
}

# ── Stop existing service before replacing binaries ───────────────────────────
$existingService = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
if ($existingService -and $existingService.Status -eq 'Running') {
    Write-Step "Stopping existing $ServiceName service"
    Stop-Service -Name $ServiceName -Force
    Start-Sleep -Seconds 2
    Write-OK "Service stopped"
}

# ── Kill existing ganoid.exe tray process ──────────────────────────────────────
$trayProc = Get-Process -Name 'ganoid' -ErrorAction SilentlyContinue
if ($trayProc) {
    Write-Step "Stopping ganoid tray process"
    $trayProc | Stop-Process -Force
    Start-Sleep -Seconds 1
    Write-OK "Process stopped"
}

# ── Download binaries ──────────────────────────────────────────────────────────
Write-Step "Downloading ganoidd.exe"
Invoke-WebRequest -Uri $ganoidd_url -OutFile "$InstallDir\ganoidd.exe" -UseBasicParsing
Write-OK "ganoidd.exe saved"

Write-Step "Downloading ganoid.exe"
Invoke-WebRequest -Uri $ganoid_url -OutFile "$InstallDir\ganoid.exe" -UseBasicParsing
Write-OK "ganoid.exe saved"

# ── Add install dir to system PATH (idempotent) ────────────────────────────────
Write-Step "Updating system PATH"
$syspath = [Environment]::GetEnvironmentVariable('Path', 'Machine')
if ($syspath -notlike "*$InstallDir*") {
    [Environment]::SetEnvironmentVariable('Path', "$syspath;$InstallDir", 'Machine')
    Write-OK "Added $InstallDir to system PATH"
} else {
    Write-OK "Already in system PATH"
}

# ── Install / update Windows service ──────────────────────────────────────────
Write-Step "Configuring Windows service '$ServiceName'"

$binPath = "`"$InstallDir\ganoidd.exe`""

if ($existingService) {
    # Update existing service binary path in case it changed.
    sc.exe config $ServiceName binPath= $binPath | Out-Null
    Write-OK "Updated existing service"
} else {
    sc.exe create $ServiceName `
        binPath= $binPath `
        DisplayName= $ServiceDisplayName `
        start= delayed-auto `
        obj= LocalSystem | Out-Null
    Write-OK "Service created"
}

# Set description.
sc.exe description $ServiceName $ServiceDesc | Out-Null

# Configure failure recovery: restart after 5 s, 3 times, then reset after 1 h.
sc.exe failure $ServiceName reset= 3600 actions= restart/5000/restart/5000/restart/5000 | Out-Null
Write-OK "Failure recovery configured (restart every 5 s, up to 3 times per hour)"

# ── Create shortcuts ───────────────────────────────────────────────────────────
function New-Shortcut([string]$LnkPath, [string]$TargetPath, [string]$Args = '', [string]$Desc = '') {
    $wsh = New-Object -ComObject WScript.Shell
    $sc  = $wsh.CreateShortcut($LnkPath)
    $sc.TargetPath = $TargetPath
    if ($Args)  { $sc.Arguments    = $Args }
    if ($Desc)  { $sc.Description  = $Desc }
    $sc.WorkingDirectory = Split-Path $TargetPath
    $sc.Save()
}

# Start Menu (all users).
Write-Step "Creating Start Menu shortcuts"
$startMenu = "$env:ProgramData\Microsoft\Windows\Start Menu\Programs\Ganoid"
if (-not (Test-Path $startMenu)) { New-Item -ItemType Directory -Path $startMenu | Out-Null }

New-Shortcut `
    "$startMenu\Ganoid.lnk" `
    "$InstallDir\ganoid.exe" `
    '' `
    'Ganoid — Tailscale profile manager tray app'

New-Shortcut `
    "$startMenu\Ganoid (no browser).lnk" `
    "$InstallDir\ganoid.exe" `
    '-no-browser' `
    'Ganoid — start tray without opening browser'

Write-OK "Start Menu shortcuts created at $startMenu"

# Per-user Startup folder (so ganoid auto-starts when the current user logs in).
Write-Step "Adding ganoid to current user Startup"
$startup = [Environment]::GetFolderPath('Startup')
New-Shortcut `
    "$startup\Ganoid.lnk" `
    "$InstallDir\ganoid.exe" `
    '' `
    'Ganoid — Tailscale profile manager'
Write-OK "Startup shortcut created at $startup\Ganoid.lnk"

# ── Start the service ──────────────────────────────────────────────────────────
Write-Step "Starting $ServiceName service"
Start-Service -Name $ServiceName
Write-OK "Service started"

# ── Launch ganoid tray for the current user (non-elevated child process) ───────
Write-Step "Launching ganoid tray"

# Run ganoid as the current interactive user (not elevated) so the tray appears
# in the right session.  We use the Explorer shell-exec trick for this.
$ganoidExe = "$InstallDir\ganoid.exe"
Start-Process -FilePath $ganoidExe -WindowStyle Hidden

Write-OK "ganoid launched"

# ── Done ───────────────────────────────────────────────────────────────────────
Write-Host ""
Write-Host "  Ganoid $tag installed successfully." -ForegroundColor Green
Write-Host ""
Write-Host "  ganoidd runs as a Windows service (auto-start on boot)."
Write-Host "  ganoid    runs in the system tray (auto-start on login)."
Write-Host ""
Write-Host "  To open the dashboard:  ganoid opens it automatically on first run."
Write-Host "  To uninstall:           run uninstall.ps1 (or remove via Services + Programs)."
Write-Host ""
