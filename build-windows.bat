@echo off
chcp 65001 >nul
title Weekend Chart Agent Builder

echo.
echo ========================================
echo   Weekend Chart Agent Build Script
echo ========================================
echo.

:: Check Go
echo [1/4] Checking Go installation...
where go >nul 2>&1
if %errorlevel% neq 0 (
    echo       ERROR: Go is not installed!
    echo       Download from: https://go.dev/dl/
    goto :error
)
for /f "tokens=*" %%i in ('go version') do echo       Found: %%i

:: Check GCC
echo [2/4] Checking GCC installation...
where gcc >nul 2>&1
if %errorlevel% neq 0 (
    echo       ERROR: GCC is not installed!
    echo       Install MinGW-w64 or TDM-GCC:
    echo         - MSYS2: https://www.msys2.org/
    echo         - TDM-GCC: https://jmeubank.github.io/tdm-gcc/
    goto :error
)
for /f "tokens=*" %%i in ('gcc --version ^| findstr gcc') do echo       Found: %%i

:: Set environment
echo [3/4] Setting build environment...
set CGO_ENABLED=1
set GOOS=windows
set GOARCH=amd64
echo       CGO_ENABLED=1, GOOS=windows, GOARCH=amd64

:: Create output directory
if not exist "%~dp0agent\build" mkdir "%~dp0agent\build"

:: Build
echo [4/4] Building agent...
cd /d "%~dp0"
go build -ldflags "-H=windowsgui -s -w" -o agent\build\weekend-chart-agent.exe .\agent
if %errorlevel% neq 0 goto :error

echo.
echo ========================================
echo   Build Successful!
echo ========================================
echo.
echo   Output: %~dp0agent\build\weekend-chart-agent.exe
echo.
goto :end

:error
echo.
echo ========================================
echo   Build Failed!
echo ========================================
echo.

:end
pause
