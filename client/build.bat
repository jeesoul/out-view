@echo off
REM Build script for outView Client

setlocal EnableDelayedExpansion

set VERSION=1.0.2-SNAPSHOT
set BUILD_DIR=build
set BINARY_NAME=outview-client

echo ====================================
echo outView Client Build Script
echo ====================================
echo.

REM Clean build directory
if exist %BUILD_DIR% rmdir /s /q %BUILD_DIR%
mkdir %BUILD_DIR%

REM Get build timestamp
for /f "tokens=*" %%a in ('powershell -Command "Get-Date -Format 'yyyy-MM-dd HH:mm:ss'"') do set BUILD_DATE=%%a

echo Building for Windows AMD64...
set GOOS=windows
set GOARCH=amd64
go build -ldflags "-s -w -X main.Version=%VERSION% -X main.BuildDate=%BUILD_DATE%" -o %BUILD_DIR%/%BINARY_NAME%-windows-amd64.exe ./cmd/outview-client
if errorlevel 1 (
    echo Build failed for Windows AMD64!
    exit /b 1
)
echo   - %BINARY_NAME%-windows-amd64.exe

echo Building for Windows 386...
set GOOS=windows
set GOARCH=386
go build -ldflags "-s -w -X main.Version=%VERSION% -X main.BuildDate=%BUILD_DATE%" -o %BUILD_DIR%/%BINARY_NAME%-windows-386.exe ./cmd/outview-client
if errorlevel 1 (
    echo Build failed for Windows 386!
    exit /b 1
)
echo   - %BINARY_NAME%-windows-386.exe

echo Building for Linux AMD64...
set GOOS=linux
set GOARCH=amd64
go build -ldflags "-s -w -X main.Version=%VERSION% -X main.BuildDate=%BUILD_DATE%" -o %BUILD_DIR%/%BINARY_NAME%-linux-amd64 ./cmd/outview-client
if errorlevel 1 (
    echo Build failed for Linux AMD64!
    exit /b 1
)
echo   - %BINARY_NAME%-linux-amd64

echo Building for Linux ARM64...
set GOOS=linux
set GOARCH=arm64
go build -ldflags "-s -w -X main.Version=%VERSION% -X main.BuildDate=%BUILD_DATE%" -o %BUILD_DIR%/%BINARY_NAME%-linux-arm64 ./cmd/outview-client
if errorlevel 1 (
    echo Build failed for Linux ARM64!
    exit /b 1
)
echo   - %BINARY_NAME%-linux-arm64

echo Building for macOS AMD64...
set GOOS=darwin
set GOARCH=amd64
go build -ldflags "-s -w -X main.Version=%VERSION% -X main.BuildDate=%BUILD_DATE%" -o %BUILD_DIR%/%BINARY_NAME%-darwin-amd64 ./cmd/outview-client
if errorlevel 1 (
    echo Build failed for macOS AMD64!
    exit /b 1
)
echo   - %BINARY_NAME%-darwin-amd64

echo Building for macOS ARM64...
set GOOS=darwin
set GOARCH=arm64
go build -ldflags "-s -w -X main.Version=%VERSION% -X main.BuildDate=%BUILD_DATE%" -o %BUILD_DIR%/%BINARY_NAME%-darwin-arm64 ./cmd/outview-client
if errorlevel 1 (
    echo Build failed for macOS ARM64!
    exit /b 1
)
echo   - %BINARY_NAME%-darwin-arm64

echo.
echo ====================================
echo Build completed!
echo Output directory: %BUILD_DIR%
echo ====================================

dir /b %BUILD_DIR%