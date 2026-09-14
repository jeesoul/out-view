@echo off
setlocal
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0..\scripts\build.ps1" -ClientMode Cli -SkipServer -SkipSidecar %*
exit /b %ERRORLEVEL%
