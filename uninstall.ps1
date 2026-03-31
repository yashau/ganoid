<#
.SYNOPSIS
    Ganoid uninstaller — removes the service, shortcuts, and binaries.

.EXAMPLE
    irm https://raw.githubusercontent.com/yashau/ganoid/main/uninstall.ps1 | iex
#>

# ── Elevation ─────────────────────────────────────────────────────────────────
function Test-Elevated {
    [bool]([Security.Principal.WindowsIdentity]::GetCurrent().Groups -match 'S-1-5-32-544')
}

if (-not (Test-Elevated)) {
    Write-Host "Ganoid uninstaller requires administrator privileges." -ForegroundColor Yellow
    Write-Host "Re-launching with elevation..." -ForegroundColor Yellow

    $rand = [System.IO.Path]::GetRandomFileName().Replace('.', '')
    $tmp  = "$env:TEMP\ganoid_uninstall_$rand.ps1"
    Set-Content -Path $tmp -Value $MyInvocation.MyCommand.ScriptBlock -Encoding UTF8

    Start-Process powershell -Verb RunAs `
        -ArgumentList "-NoProfile -ExecutionPolicy Bypass -File `"$tmp`""
    exit
}

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$InstallDir  = "$env:ProgramFiles\Ganoid"
$ServiceName = 'ganoidd'
$StartMenu   = "$env:ProgramData\Microsoft\Windows\Start Menu\Programs\Ganoid"
$Startup     = [Environment]::GetFolderPath('Startup')

function Write-Step([string]$Msg) { Write-Host "`n==> $Msg" -ForegroundColor Cyan }
function Write-OK([string]$Msg)   { Write-Host "    OK  $Msg" -ForegroundColor Green }

# Stop and remove service.
Write-Step "Removing Windows service '$ServiceName'"
$svc = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
if ($svc) {
    if ($svc.Status -eq 'Running') { Stop-Service -Name $ServiceName -Force }
    sc.exe delete $ServiceName | Out-Null
    Write-OK "Service removed"
} else {
    Write-OK "Service not found (already removed)"
}

# Kill tray process.
Write-Step "Stopping ganoid tray"
Get-Process -Name 'ganoid' -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
Write-OK "Done"

# Remove shortcuts.
Write-Step "Removing shortcuts"
if (Test-Path $StartMenu) { Remove-Item $StartMenu -Recurse -Force; Write-OK "Start Menu folder removed" }
$startupLnk = "$Startup\Ganoid.lnk"
if (Test-Path $startupLnk) { Remove-Item $startupLnk -Force; Write-OK "Startup shortcut removed" }

# Remove from PATH.
Write-Step "Cleaning system PATH"
$syspath = [Environment]::GetEnvironmentVariable('Path', 'Machine')
$newpath = ($syspath -split ';' | Where-Object { $_ -ne $InstallDir }) -join ';'
[Environment]::SetEnvironmentVariable('Path', $newpath, 'Machine')
Write-OK "PATH updated"

# Remove install directory.
Write-Step "Removing $InstallDir"
if (Test-Path $InstallDir) { Remove-Item $InstallDir -Recurse -Force; Write-OK "Directory removed" }
else { Write-OK "Directory not found" }

Write-Host ""
Write-Host "  Ganoid has been uninstalled." -ForegroundColor Green
Write-Host "  Config data in %APPDATA%\ganoid was left intact." -ForegroundColor Gray
Write-Host ""
