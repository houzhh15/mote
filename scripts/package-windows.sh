#!/bin/bash
# Windows 打包脚本 (PowerShell/Bash hybrid)
# 将 Wails 构建产物打包为可发布的 .zip

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
BUILD_DIR="$PROJECT_ROOT/build"
GUI_BUILD_DIR="$PROJECT_ROOT/gui/build/bin"
APP_NAME="Mote"
VERSION="${VERSION:-1.0.0}"

echo "=== Mote Windows Package Script ==="
echo "Version: $VERSION"

# Create output directory
OUTPUT_DIR="$BUILD_DIR/windows"
mkdir -p "$OUTPUT_DIR"

# Check if running on Windows (Git Bash/MSYS) or cross-compiling
if [[ "$OSTYPE" == "msys" ]] || [[ "$OSTYPE" == "cygwin" ]]; then
    IS_WINDOWS=true
else
    IS_WINDOWS=false
fi

# Find Windows executables
EXE_SUFFIX=".exe"
GUI_EXE=$(find "$GUI_BUILD_DIR" -name "*$EXE_SUFFIX" 2>/dev/null | head -1)

if [ -n "$GUI_EXE" ]; then
    cp "$GUI_EXE" "$OUTPUT_DIR/$APP_NAME.exe"
    echo "Copied GUI executable"
fi

# Copy CLI and tray if they exist
if [ -f "$BUILD_DIR/mote$EXE_SUFFIX" ]; then
    cp "$BUILD_DIR/mote$EXE_SUFFIX" "$OUTPUT_DIR/"
    echo "Copied mote CLI"
fi

if [ -f "$BUILD_DIR/mote-tray$EXE_SUFFIX" ]; then
    cp "$BUILD_DIR/mote-tray$EXE_SUFFIX" "$OUTPUT_DIR/"
    echo "Copied mote-tray"
fi

# Create launcher batch file
cat > "$OUTPUT_DIR/mote-launcher.bat" << 'LAUNCHER'
@echo off
setlocal

set "SCRIPT_DIR=%~dp0"
set "MOTE_GUI=%SCRIPT_DIR%Mote.exe"
set "MOTE_TRAY=%SCRIPT_DIR%mote-tray.exe"

:: Start GUI
if exist "%MOTE_GUI%" (
    start "" "%MOTE_GUI%"
) else (
    echo Mote GUI not found.
)

:: Start tray (optional)
if exist "%MOTE_TRAY%" (
    start "" "%MOTE_TRAY%"
)

endlocal
LAUNCHER
echo "Created launcher batch file"

# Create PATH installation script
cat > "$OUTPUT_DIR/install-path.bat" << 'INSTALL'
@echo off
setlocal EnableDelayedExpansion

echo === Mote PATH Installation ===
echo.

set "SCRIPT_DIR=%~dp0"
set "SCRIPT_DIR=%SCRIPT_DIR:~0,-1%"

:: Check admin privileges
net session >nul 2>&1
if %errorLevel% neq 0 (
    echo This script requires administrator privileges.
    echo Please right-click and select "Run as administrator".
    pause
    exit /b 1
)

:: Add to system PATH
echo Adding Mote to system PATH...
for /f "tokens=2*" %%a in ('reg query "HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\Environment" /v Path 2^>nul') do set "CURRENT_PATH=%%b"

echo !CURRENT_PATH! | findstr /i "%SCRIPT_DIR%" >nul
if %errorLevel% equ 0 (
    echo Mote is already in PATH.
) else (
    setx PATH "!CURRENT_PATH!;%SCRIPT_DIR%" /M >nul 2>&1
    if %errorLevel% equ 0 (
        echo Successfully added to PATH.
        echo Please restart your terminal for changes to take effect.
    ) else (
        echo Failed to add to PATH. Please add manually:
        echo %SCRIPT_DIR%
    )
)

echo.
pause
INSTALL
echo "Created PATH installation script"

# Create uninstall script
cat > "$OUTPUT_DIR/uninstall.bat" << 'UNINSTALL'
@echo off
echo === Mote Uninstallation ===
echo.
echo This will remove Mote files from this directory.
echo It will NOT remove Mote from your system PATH.
echo.
set /p confirm="Are you sure? (y/n): "
if /i not "%confirm%"=="y" exit /b 0

set "SCRIPT_DIR=%~dp0"
del /q "%SCRIPT_DIR%Mote.exe" 2>nul
del /q "%SCRIPT_DIR%mote.exe" 2>nul
del /q "%SCRIPT_DIR%mote-tray.exe" 2>nul
del /q "%SCRIPT_DIR%mote-launcher.bat" 2>nul
del /q "%SCRIPT_DIR%install-path.bat" 2>nul

echo.
echo Uninstallation complete.
echo To remove from PATH, edit system environment variables manually.
pause
UNINSTALL
echo "Created uninstall script"

# Create README
cat > "$OUTPUT_DIR/README.txt" << 'README'
Mote - AI Agent Runtime
=======================

Quick Start:
1. Double-click Mote.exe to launch the GUI
2. Or double-click mote-launcher.bat to launch both GUI and tray

Command Line:
1. Run install-path.bat as Administrator to add mote to PATH
2. Open a new terminal and use: mote --help

Files:
- Mote.exe        : Main GUI application
- mote.exe        : Command-line interface
- mote-tray.exe   : System tray application
- mote-launcher.bat : Launches GUI and tray together

Configuration:
Config files are stored in: %USERPROFILE%\.mote\

For more information, visit: https://github.com/openclaw/mote
README

# Create ZIP archive
ZIP_NAME="Mote-$VERSION-Windows-amd64.zip"
echo "Creating ZIP: $ZIP_NAME"
cd "$OUTPUT_DIR"

if command -v zip &> /dev/null; then
    zip -r "$ZIP_NAME" *.exe *.bat *.txt 2>/dev/null || true
elif $IS_WINDOWS; then
    # Use PowerShell on Windows
    powershell -Command "Compress-Archive -Path '*.exe','*.bat','*.txt' -DestinationPath '$ZIP_NAME' -Force" 2>/dev/null || true
fi

echo ""
echo "=== Package Complete ==="
echo "Output: $OUTPUT_DIR"
ls -la "$OUTPUT_DIR" 2>/dev/null || dir "$OUTPUT_DIR"
