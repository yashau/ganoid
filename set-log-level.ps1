# set-log-level.ps1 — reinstalls ganoidd service with a custom log level.
# Must be run as Administrator.
#
# Usage:
#   .\set-log-level.ps1 debug
#   .\set-log-level.ps1 info      (default)
#   .\set-log-level.ps1 warn
#   .\set-log-level.ps1 error

param(
    [ValidateSet("debug","info","warn","error")]
    [string]$Level = "info"
)

$ErrorActionPreference = "Stop"

$ServiceName = "ganoidd"
$BinPath = (Get-WmiObject Win32_Service -Filter "Name='$ServiceName'").PathName

if (-not $BinPath) {
    Write-Error "Service '$ServiceName' not found. Is Ganoid installed?"
    exit 1
}

# Strip any existing -log-level flag from the binary path.
$BinPath = $BinPath -replace '\s+-log-level\s+\S+', ''
$BinPath = $BinPath.Trim()

$NewBinPath = "$BinPath -log-level $Level"

Write-Host "Stopping $ServiceName..."
Stop-Service $ServiceName -Force

Write-Host "Setting log level to '$Level'..."
sc.exe config $ServiceName binPath= $NewBinPath | Out-Null

Write-Host "Starting $ServiceName..."
Start-Service $ServiceName

Write-Host ""
Write-Host "Done. Log level is now '$Level'."
Write-Host "Log file: $env:ProgramData\Ganoid\ganoidd.log"
