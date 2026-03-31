# Ganoid Build Script (PowerShell)
#
# Usage:
#   .\build.ps1 -Version 0.1.0
#   .\build.ps1 -Version 0.1.0 -Target all
#   .\build.ps1 -Version 0.1.0 -Target linux

param(
    [Parameter(Mandatory=$true)]
    [string]$Version,
    [string]$Target = "windows"
)

$ErrorActionPreference = "Stop"

# ── Metadata ──────────────────────────────────────────────────────────────────
$BuildTime = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
$GitCommit = try { git rev-parse --short HEAD 2>$null } catch { "unknown" }
if (-not $GitCommit) { $GitCommit = "unknown" }

$VersionParts  = $Version.Split('.')
$Major = if ($VersionParts.Length -gt 0) { [int]$VersionParts[0] } else { 0 }
$Minor = if ($VersionParts.Length -gt 1) { [int]$VersionParts[1] } else { 1 }
$Patch = if ($VersionParts.Length -gt 2) { [int]$VersionParts[2] } else { 0 }
$VersionString = "$Major.$Minor.$Patch.0"

$LdBase   = "-s -w"
$LdVars   = "-X main.version=$Version -X main.buildTime=$BuildTime -X main.gitCommit=$GitCommit"
$LdFlags  = "$LdBase $LdVars"

Write-Host "`nBuilding Ganoid" -ForegroundColor Cyan
Write-Host "  Version:    $Version"
Write-Host "  Build time: $BuildTime"
Write-Host "  Git commit: $GitCommit"
Write-Host "  Target:     $Target"
Write-Host ""

Push-Location $PSScriptRoot

try {
    # ── Helper: generate versioninfo.json + resource.syso for one binary ──────
    function New-WinResource([string]$CmdDir, [string]$BinName, [string]$Description, [string]$OrigName) {
        $Utf8NoBom = New-Object System.Text.UTF8Encoding $false
        $Info = [ordered]@{
            FixedFileInfo = [ordered]@{
                FileVersion    = [ordered]@{ Major=$Major; Minor=$Minor; Patch=$Patch; Build=0 }
                ProductVersion = [ordered]@{ Major=$Major; Minor=$Minor; Patch=$Patch; Build=0 }
                FileFlagsMask  = "3f"; FileFlags = "00"; FileOS = "040004"
                FileType       = "01"; FileSubType = "00"
            }
            StringFileInfo = [ordered]@{
                Comments         = ""
                CompanyName      = "Ibrahim Yashau"
                FileDescription  = $Description
                FileVersion      = $VersionString
                InternalName     = $BinName
                LegalCopyright   = "Copyright (c) 2026 Ibrahim Yashau. All rights reserved."
                LegalTrademarks  = ""
                OriginalFilename = $OrigName
                PrivateBuild     = ""
                ProductName      = "Ganoid"
                ProductVersion   = $VersionString
                SpecialBuild     = ""
            }
            VarFileInfo = [ordered]@{
                Translation = [ordered]@{ LangID = "0409"; CharsetID = "04B0" }
            }
            IconPath     = "../../internal/tray/icon.ico"
            ManifestPath = ""
        }
        $Json = ($Info | ConvertTo-Json -Depth 10) -replace "`r`n", "`n"
        $JsonPath = Join-Path $CmdDir "versioninfo.json"
        [System.IO.File]::WriteAllText($JsonPath, $Json, $Utf8NoBom)

        Push-Location $CmdDir
        try {
            & goversioninfo -64 -o resource.syso versioninfo.json
            if ($LASTEXITCODE -ne 0) { Write-Host "  WARN: goversioninfo failed for $BinName" -ForegroundColor Yellow }
            else { Write-Host "  OK  resource.syso ($BinName)" -ForegroundColor Green }
        } finally {
            Pop-Location
        }
    }

    # ── Generate Windows resources ────────────────────────────────────────────
    if ($Target -eq "windows" -or $Target -eq "all") {
        Write-Host "Generating Windows resources..." -ForegroundColor Yellow

        New-WinResource `
            (Join-Path $PSScriptRoot "cmd\ganoidd") `
            "ganoidd" `
            "Ganoid Daemon — Tailscale profile coordination server" `
            "ganoidd.exe"

        New-WinResource `
            (Join-Path $PSScriptRoot "cmd\ganoid") `
            "ganoid" `
            "Ganoid — Tailscale profile manager tray application" `
            "ganoid.exe"
    }

    # ── Build ─────────────────────────────────────────────────────────────────
    function Invoke-GoBuild([string]$OS, [string]$Arch, [string]$OutD, [string]$OutG) {
        $env:GOOS       = $OS
        $env:GOARCH     = $Arch
        $env:CGO_ENABLED = "0"

        Write-Host "Building ganoidd ($OS/$Arch)..." -ForegroundColor Yellow
        & go build -ldflags $LdFlags -o $OutD ./cmd/ganoidd
        if ($LASTEXITCODE -ne 0) { throw "go build ganoidd failed" }
        Write-Host "  OK  $OutD" -ForegroundColor Green

        Write-Host "Building ganoid ($OS/$Arch)..." -ForegroundColor Yellow
        & go build -ldflags $LdFlags -o $OutG ./cmd/ganoid
        if ($LASTEXITCODE -ne 0) { throw "go build ganoid failed" }
        Write-Host "  OK  $OutG" -ForegroundColor Green
    }

    switch ($Target) {
        "windows" { Invoke-GoBuild "windows" "amd64" "ganoidd.exe"    "ganoid.exe"    }
        "linux"   { Invoke-GoBuild "linux"   "amd64" "ganoidd-linux"  "ganoid-linux"  }
        "darwin"  { Invoke-GoBuild "darwin"  "arm64" "ganoidd-darwin" "ganoid-darwin" }
        "all" {
            Invoke-GoBuild "windows" "amd64" "ganoidd.exe"    "ganoid.exe"
            Invoke-GoBuild "linux"   "amd64" "ganoidd-linux"  "ganoid-linux"
            Invoke-GoBuild "darwin"  "arm64" "ganoidd-darwin" "ganoid-darwin"
        }
        default { throw "Unknown target '$Target'. Use: windows, linux, darwin, all" }
    }

    # ── Checksums ─────────────────────────────────────────────────────────────
    Write-Host "`nGenerating checksums..." -ForegroundColor Yellow
    $Bins = @()
    switch ($Target) {
        "windows" { $Bins = @("ganoidd.exe",    "ganoid.exe")    }
        "linux"   { $Bins = @("ganoidd-linux",  "ganoid-linux")  }
        "darwin"  { $Bins = @("ganoidd-darwin", "ganoid-darwin") }
        "all"     { $Bins = @("ganoidd.exe","ganoid.exe","ganoidd-linux","ganoid-linux","ganoidd-darwin","ganoid-darwin") }
    }
    $Lines = @()
    foreach ($b in $Bins) {
        if (Test-Path $b) {
            $hash = (Get-FileHash -Path $b -Algorithm SHA256).Hash.ToLower()
            $size = [math]::Round((Get-Item $b).Length / 1MB, 2)
            $Lines += "$hash  $b"
            Write-Host "  $b ($size MB)" -ForegroundColor Green
        }
    }
    if ($Lines.Count -gt 0) {
        $Utf8NoBom = New-Object System.Text.UTF8Encoding $false
        [System.IO.File]::WriteAllLines((Join-Path $PSScriptRoot "checksums.txt"), $Lines, $Utf8NoBom)
        Write-Host "  Checksums written to checksums.txt" -ForegroundColor Green
    }

    Write-Host "`nBuild successful! Ganoid v$Version" -ForegroundColor Green

} finally {
    Pop-Location
    Remove-Item Env:GOOS        -ErrorAction SilentlyContinue
    Remove-Item Env:GOARCH      -ErrorAction SilentlyContinue
    Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue
}
