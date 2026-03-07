@echo off
cd /d "%~dp0"
node.exe agent.js %*
pause
