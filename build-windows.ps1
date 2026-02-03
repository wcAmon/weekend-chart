# Weekend Chart Agent - Windows Build Script
# Usage: Right-click -> Run with PowerShell
#        Or: powershell -ExecutionPolicy Bypass -File build-windows.ps1

$ErrorActionPreference = "Stop"

Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  Weekend Chart Agent Build Script" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""

# Check Go installation
Write-Host "[1/4] Checking Go installation..." -ForegroundColor Yellow
try {
    $goVersion = go version
    Write-Host "      Found: $goVersion" -ForegroundColor Green
} catch {
    Write-Host "      ERROR: Go is not installed!" -ForegroundColor Red
    Write-Host "      Download from: https://go.dev/dl/" -ForegroundColor Red
    Write-Host ""
    Read-Host "Press Enter to exit"
    exit 1
}

# Check GCC installation
Write-Host "[2/4] Checking GCC installation..." -ForegroundColor Yellow
try {
    $gccVersion = gcc --version | Select-Object -First 1
    Write-Host "      Found: $gccVersion" -ForegroundColor Green
} catch {
    Write-Host "      ERROR: GCC is not installed!" -ForegroundColor Red
    Write-Host "      Install MinGW-w64 or TDM-GCC:" -ForegroundColor Red
    Write-Host "        - MSYS2: https://www.msys2.org/" -ForegroundColor Red
    Write-Host "        - TDM-GCC: https://jmeubank.github.io/tdm-gcc/" -ForegroundColor Red
    Write-Host ""
    Read-Host "Press Enter to exit"
    exit 1
}

# Set environment
Write-Host "[3/4] Setting build environment..." -ForegroundColor Yellow
$env:CGO_ENABLED = "1"
$env:GOOS = "windows"
$env:GOARCH = "amd64"
Write-Host "      CGO_ENABLED=1, GOOS=windows, GOARCH=amd64" -ForegroundColor Green

# Create output directory
$outputDir = Join-Path $PSScriptRoot "agent\build"
if (-not (Test-Path $outputDir)) {
    New-Item -ItemType Directory -Path $outputDir -Force | Out-Null
}

# Build
Write-Host "[4/4] Building agent..." -ForegroundColor Yellow
$outputFile = Join-Path $outputDir "weekend-chart-agent.exe"

try {
    Push-Location $PSScriptRoot
    go build -ldflags "-H=windowsgui -s -w" -o $outputFile .\agent
    Pop-Location

    $fileInfo = Get-Item $outputFile
    $fileSizeMB = [math]::Round($fileInfo.Length / 1MB, 2)

    Write-Host ""
    Write-Host "========================================" -ForegroundColor Green
    Write-Host "  Build Successful!" -ForegroundColor Green
    Write-Host "========================================" -ForegroundColor Green
    Write-Host ""
    Write-Host "  Output: $outputFile" -ForegroundColor White
    Write-Host "  Size:   $fileSizeMB MB" -ForegroundColor White
    Write-Host ""
} catch {
    Write-Host ""
    Write-Host "========================================" -ForegroundColor Red
    Write-Host "  Build Failed!" -ForegroundColor Red
    Write-Host "========================================" -ForegroundColor Red
    Write-Host ""
    Write-Host "  Error: $_" -ForegroundColor Red
    Write-Host ""
}

Read-Host "Press Enter to exit"
