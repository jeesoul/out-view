@echo off
REM Build script for outView Client with GUI

setlocal EnableDelayedExpansion

set VERSION=1.0.0
set BUILD_DIR=build
set BINARY_NAME=outview-client

echo ====================================
echo outView Client Build Script (GUI)
echo ====================================
echo.

REM Clean build directory
if exist %BUILD_DIR% rmdir /s /q %BUILD_DIR%
mkdir %BUILD_DIR%

REM Get build timestamp
for /f "tokens=*" %%a in ('powershell -Command "Get-Date -Format 'yyyy-MM-dd HH:mm:ss'"') do set BUILD_DATE=%%a

echo Building GUI client for Windows AMD64...
set GOOS=windows
set GOARCH=amd64
set CGO_ENABLED=1
go build -ldflags "-s -w -H windowsgui -X main.Version=%VERSION% -X main.BuildDate=%BUILD_DATE%" -o %BUILD_DIR%/%BINARY_NAME%-gui-windows-amd64.exe ./cmd/outview-gui
if errorlevel 1 (
    echo Build failed for GUI client!
    echo Note: GUI build requires CGO. Make sure you have a C compiler installed.
    echo Falling back to CLI-only build...
    go build -ldflags "-s -w -X main.Version=%VERSION% -X main.BuildDate=%BUILD_DATE%" -o %BUILD_DIR%/%BINARY_NAME%-windows-amd64.exe ./cmd/outview-client
)
echo.

echo Building CLI client for Windows AMD64...
set CGO_ENABLED=0
go build -ldflags "-s -w -X main.Version=%VERSION% -X main.BuildDate=%BUILD_DATE%" -o %BUILD_DIR%/%BINARY_NAME%-windows-amd64.exe ./cmd/outview-client
if errorlevel 1 (
    echo Build failed for CLI client!
    exit /b 1
)
echo   - %BINARY_NAME%-windows-amd64.exe (CLI)

echo.
echo ====================================
echo Build completed!
echo Output directory: %BUILD_DIR%
echo ====================================

dir /b %BUILD_DIR%

echo.
echo Usage:
echo   GUI: %BUILD_DIR%\%BINARY_NAME%-gui-windows-amd64.exe
echo   CLI: %BUILD_DIR%\%BINARY_NAME%-windows-amd64.exe -host SERVER -device-id ID -token TOKEN