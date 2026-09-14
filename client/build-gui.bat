@echo off
setlocal
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0..\scripts\build.ps1" -ClientMode Both -SkipServer -SkipSidecar %*
if errorlevel 1 echo GUI build failed. No CLI artifact was renamed as a GUI artifact.
exit /b %ERRORLEVEL%
