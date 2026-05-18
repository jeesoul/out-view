@echo off
REM Build Windows installer for outView v1.2.0

echo Building outView v1.2.0 Windows Installer...

REM Check if Inno Setup is installed
set ISCC="C:\Program Files (x86)\Inno Setup 6\ISCC.exe"
if not exist %ISCC% (
    echo ERROR: Inno Setup not found at %ISCC%
    echo Please install Inno Setup 6 from https://jrsoftware.org/isdl.php
    pause
    exit /b 1
)

REM Build the installer
echo Compiling installer...
%ISCC% installer\windows\outview-setup.iss

if %ERRORLEVEL% EQU 0 (
    echo.
    echo ========================================
    echo SUCCESS! Installer created:
    echo installer\windows\Output\outview-1.2.0-setup.exe
    echo ========================================
) else (
    echo.
    echo ERROR: Installer build failed!
    pause
    exit /b 1
)

pause
