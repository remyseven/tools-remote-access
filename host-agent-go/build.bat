@echo off
:: Build Remotely.exe for Windows (run this on Windows with Go installed)
cd /d "%~dp0"

echo [build] Downloading dependencies...
go mod tidy

echo [build] Building Remotely.exe...
set CGO_ENABLED=0
set GOOS=windows
set GOARCH=amd64
go build -ldflags="-s -w" -o dist\Remotely.exe .

if %errorlevel% neq 0 (
    echo [build] FAILED
    pause
    exit /b 1
)

echo [build] Done: dist\Remotely.exe
dir dist\Remotely.exe
pause
