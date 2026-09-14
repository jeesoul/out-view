@echo off
setlocal EnableExtensions

set "PROJECT_ROOT=%~dp0"
for /f "usebackq delims=" %%V in (`powershell.exe -NoProfile -Command "[xml]$p=Get-Content -Raw -Encoding UTF8 '%PROJECT_ROOT%pom.xml'; $p.project.version"`) do set "APP_VERSION=%%V"
if not defined APP_VERSION (
  echo ERROR: Cannot read the version from pom.xml.
  exit /b 1
)

if not defined OUTVIEW_SOURCE_ROOT set "OUTVIEW_SOURCE_ROOT=%PROJECT_ROOT%release\outview-%APP_VERSION%"
if not defined OUTVIEW_INSTALLER_OUTPUT_DIR set "OUTVIEW_INSTALLER_OUTPUT_DIR=%PROJECT_ROOT%release\installer"
if defined ISCC goto compiler_ready
set "ISCC=%ProgramFiles(x86)%\Inno Setup 6\ISCC.exe"
if not exist "%ISCC%" set "ISCC=%ProgramFiles%\Inno Setup 6\ISCC.exe"
:compiler_ready

if not exist "%ISCC%" (
  echo ERROR: Inno Setup compiler not found. Set ISCC to the full ISCC.exe path.
  exit /b 1
)
if not exist "%OUTVIEW_SOURCE_ROOT%\client\windows\outview-client-gui-windows-amd64.exe" (
  echo ERROR: Explicit GUI artifact is missing under %OUTVIEW_SOURCE_ROOT%.
  echo Build it with scripts\build.ps1 -Release -ClientMode Both.
  exit /b 1
)
if not exist "%OUTVIEW_SOURCE_ROOT%\webrtc-sidecar\windows\outview-sidecar-windows-amd64.exe" (
  echo ERROR: Sidecar artifact is missing under %OUTVIEW_SOURCE_ROOT%.
  exit /b 1
)
if not exist "%OUTVIEW_INSTALLER_OUTPUT_DIR%" mkdir "%OUTVIEW_INSTALLER_OUTPUT_DIR%"

echo Building outView v%APP_VERSION% Windows installer...
"%ISCC%" "/DAppVersion=%APP_VERSION%" "/DSourceRoot=%OUTVIEW_SOURCE_ROOT%" "/DOutputDir=%OUTVIEW_INSTALLER_OUTPUT_DIR%" "%PROJECT_ROOT%installer\windows\outview-setup.iss"
exit /b %ERRORLEVEL%
