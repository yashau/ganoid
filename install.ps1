<#
.SYNOPSIS
    Ganoid installer — downloads binaries from GitHub releases and configures the system.

.EXAMPLE
    irm https://raw.githubusercontent.com/yashau/ganoid/main/install.ps1 | iex
#>

# ── Elevation ─────────────────────────────────────────────────────────────────
function Test-Elevated {
    [bool]([Security.Principal.WindowsIdentity]::GetCurrent().Groups -match 'S-1-5-32-544')
}

if (-not (Test-Elevated)) {
    Write-Host "Ganoid installer requires administrator privileges." -ForegroundColor Yellow
    Write-Host "Re-launching with elevation..." -ForegroundColor Yellow

    $rand = [System.IO.Path]::GetRandomFileName().Replace('.', '')
    $tmp  = "$env:TEMP\ganoid_install_$rand.ps1"
    Set-Content -Path $tmp -Value $MyInvocation.MyCommand.ScriptBlock -Encoding UTF8

    Start-Process powershell -Verb RunAs `
        -ArgumentList "-NoProfile -ExecutionPolicy Bypass -File `"$tmp`""
    exit
}

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# ── Config ────────────────────────────────────────────────────────────────────
$Repo               = 'yashau/ganoid'
$InstallDir         = "$env:ProgramFiles\Ganoid"
$ServiceName        = 'ganoidd'
$ServiceDisplayName = 'Ganoid Daemon'
$ServiceDesc        = 'Tailscale profile coordination daemon for Ganoid'

function Write-Step([string]$Msg) { Write-Host "`n==> $Msg" -ForegroundColor Cyan }
function Write-OK([string]$Msg)   { Write-Host "    OK  $Msg" -ForegroundColor Green }

# ── Fetch latest release ───────────────────────────────────────────────────────
Write-Step "Fetching latest release from github.com/$Repo"

$headers = @{ 'User-Agent' = 'ganoid-installer/1.0' }
try {
    $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -Headers $headers
} catch {
    Write-Error "Failed to fetch release info: $_"
    exit 1
}

$tag = $release.tag_name
Write-OK "Latest release: $tag"

function Get-AssetUrl([string]$Name) {
    $asset = $release.assets | Where-Object { $_.name -eq $Name } | Select-Object -First 1
    if (-not $asset) { Write-Error "Asset '$Name' not found in release $tag"; exit 1 }
    return $asset.browser_download_url
}

$ganoidd_url = Get-AssetUrl 'ganoidd.exe'
$ganoid_url  = Get-AssetUrl 'ganoid.exe'

# ── Create install directory ───────────────────────────────────────────────────
Write-Step "Installing to $InstallDir"
if (-not (Test-Path $InstallDir)) { New-Item -ItemType Directory -Path $InstallDir | Out-Null }

# ── Stop existing service and tray ────────────────────────────────────────────
$existingService = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
if ($existingService -and $existingService.Status -eq 'Running') {
    Write-Step "Stopping existing $ServiceName service"
    Stop-Service -Name $ServiceName -Force
    Start-Sleep -Seconds 2
    Write-OK "Service stopped"
}

Get-Process -Name 'ganoid' -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue

# ── Download binaries ──────────────────────────────────────────────────────────
Write-Step "Downloading ganoidd.exe"
Invoke-WebRequest -Uri $ganoidd_url -OutFile "$InstallDir\ganoidd.exe" -UseBasicParsing
Write-OK "ganoidd.exe saved"

Write-Step "Downloading ganoid.exe"
Invoke-WebRequest -Uri $ganoid_url -OutFile "$InstallDir\ganoid.exe" -UseBasicParsing
Write-OK "ganoid.exe saved"

# ── System PATH ───────────────────────────────────────────────────────────────
Write-Step "Updating system PATH"
$syspath = [Environment]::GetEnvironmentVariable('Path', 'Machine')
if ($syspath -notlike "*$InstallDir*") {
    [Environment]::SetEnvironmentVariable('Path', "$syspath;$InstallDir", 'Machine')
    Write-OK "Added $InstallDir to system PATH"
} else {
    Write-OK "Already in system PATH"
}

# ── Windows service ───────────────────────────────────────────────────────────
Write-Step "Configuring Windows service '$ServiceName'"
$binPath = "`"$InstallDir\ganoidd.exe`""
if ($existingService) {
    sc.exe config $ServiceName binPath= $binPath | Out-Null
    Write-OK "Updated existing service"
} else {
    sc.exe create $ServiceName binPath= $binPath DisplayName= $ServiceDisplayName start= delayed-auto obj= LocalSystem | Out-Null
    Write-OK "Service created"
}
sc.exe description $ServiceName $ServiceDesc | Out-Null
sc.exe failure $ServiceName reset= 3600 actions= restart/5000/restart/5000/restart/5000 | Out-Null
Write-OK "Failure recovery configured"

# ── Shortcuts ─────────────────────────────────────────────────────────────────
function New-Shortcut([string]$LnkPath, [string]$TargetPath, [string]$Arguments = '', [string]$Desc = '') {
    $wsh = New-Object -ComObject WScript.Shell
    $sc  = $wsh.CreateShortcut($LnkPath)
    $sc.TargetPath       = $TargetPath
    $sc.WorkingDirectory = Split-Path $TargetPath
    if ($Arguments) { $sc.Arguments   = $Arguments }
    if ($Desc)      { $sc.Description = $Desc }
    $sc.Save()
}

Write-Step "Creating shortcuts"
$startMenu = "$env:ProgramData\Microsoft\Windows\Start Menu\Programs\Ganoid"
if (-not (Test-Path $startMenu)) { New-Item -ItemType Directory -Path $startMenu | Out-Null }
New-Shortcut "$startMenu\Ganoid.lnk" "$InstallDir\ganoid.exe" '' 'Ganoid - Tailscale profile manager'
Write-OK "Start Menu shortcut created"

$startup = [Environment]::GetFolderPath('Startup')
New-Shortcut "$startup\Ganoid.lnk" "$InstallDir\ganoid.exe" '' 'Ganoid - Tailscale profile manager'
Write-OK "Startup shortcut created"

# ── Start service ─────────────────────────────────────────────────────────────
Write-Step "Starting $ServiceName service"
Start-Service -Name $ServiceName
Write-OK "Service started"

# ── Launch ganoid tray (de-elevated via explorer.exe) ─────────────────────────
Write-Step "Launching ganoid tray"
Start-Sleep -Seconds 2
Start-Process explorer.exe -ArgumentList "`"$InstallDir\ganoid.exe`""
Write-OK "ganoid launched"

Write-Host ""
Write-Host "  Ganoid $tag installed successfully." -ForegroundColor Green
Write-Host "  ganoidd runs as a Windows service (auto-start on boot)."
Write-Host "  ganoid    runs in the system tray (auto-start on login)."
Write-Host ""
