@echo off
:: set-log-level.cmd — runs set-log-level.ps1 bypassing execution policy.
:: Must be run as Administrator.
::
:: Usage:
::   set-log-level.cmd debug
::   set-log-level.cmd info
::   set-log-level.cmd warn
::   set-log-level.cmd error

if "%~1"=="" (
    echo Usage: set-log-level.cmd [debug^|info^|warn^|error]
    exit /b 1
)

powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0set-log-level.ps1" %1
