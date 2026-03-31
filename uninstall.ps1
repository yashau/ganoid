#Requires -RunAsAdministrator
<#
.SYNOPSIS
    Ganoid uninstaller — removes the service, shortcuts, and binaries.

.EXAMPLE
    irm https://raw.githubusercontent.com/yashau/ganoid/main/uninstall.ps1 | iex
#>

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
$sc = "$Startup\Ganoid.lnk"
if (Test-Path $sc) { Remove-Item $sc -Force; Write-OK "Startup shortcut removed" }

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
