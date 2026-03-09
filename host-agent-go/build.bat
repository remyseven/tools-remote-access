@echo off
:: Build Remotely.exe for Windows (run this on Windows with Go installed)
cd /d "%~dp0"

:: Find go.exe — check common install locations if not in PATH
where go >nul 2>&1
if %errorlevel% equ 0 (
    set GO=go
) else if exist "C:\Program Files\Go\bin\go.exe" (
    set GO=C:\Program Files\Go\bin\go.exe
) else if exist "C:\Go\bin\go.exe" (
    set GO=C:\Go\bin\go.exe
) else (
    echo [build] ERROR: go.exe not found. Install Go from https://go.dev/dl/
    pause
    exit /b 1
)

echo [build] Downloading dependencies...
"%GO%" mod tidy

echo [build] Building Remotely.exe...
set CGO_ENABLED=0
set GOOS=windows
set GOARCH=amd64
"%GO%" build -ldflags="-s -w" -o dist\Remotely.exe .

if %errorlevel% neq 0 (
    echo [build] FAILED
    pause
    exit /b 1
)

echo [build] Done: dist\Remotely.exe
dir dist\Remotely.exe
pause
