@echo off
REM Copyright 2026 Bert Shim <bertshim@gmail.com>
REM SPDX-License-Identifier: Apache-2.0
setlocal
cd /d "%~dp0"
go build -o termlink.exe ./cmd/termlink
if errorlevel 1 exit /b 1
echo Built: %~dp0termlink.exe
