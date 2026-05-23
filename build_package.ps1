# GPS Signal Simulator PowerShell Packaging Script
# Requires Go to be installed and in PATH.

$ErrorActionPreference = "Stop"

$WorkspaceDir = Get-Location
$BuildDir = Join-Path $WorkspaceDir "build"
$DistDir = Join-Path $WorkspaceDir "dist"
$AppName = "gps-simulator"

Write-Host "==================================================" -ForegroundColor Cyan
Write-Host "    GPS Signal Simulator Windows Packager" -ForegroundColor Cyan
Write-Host "==================================================" -ForegroundColor Cyan

# 1. Clean Directories
Write-Host "🧹 Cleaning old build and dist directories..." -ForegroundColor Yellow
if (Test-Path $BuildDir) { Remove-Item -Path $BuildDir -Recurse -Force }
if (Test-Path $DistDir) { Remove-Item -Path $DistDir -Recurse -Force }
New-Item -Path $DistDir -ItemType Directory -Force | Out-Null

# 2. Build and Package Windows
Write-Host "`n🏁 Building and packaging for Windows (amd64)..." -ForegroundColor Yellow
$WindowsBuildPath = Join-Path $BuildDir "windows_pkg"
New-Item -Path $WindowsBuildPath -ItemType Directory -Force | Out-Null

Write-Host "   [1/3] Compiling Go binary..."
$env:GOOS = "windows"
$env:GOARCH = "amd64"
go build -o (Join-Path $WindowsBuildPath "$AppName.exe") main.go

Write-Host "   [2/3] Preparing assets..."
Copy-Item -Path (Join-Path $WorkspaceDir "data") -Destination $WindowsBuildPath -Recurse -Force
Copy-Item -Path (Join-Path $WorkspaceDir "README.md") -Destination $WindowsBuildPath -Force
Copy-Item -Path (Join-Path $WorkspaceDir "README_Windows.md") -Destination $WindowsBuildPath -Force

# Clean any temporary files
$TempBin = Join-Path $WindowsBuildPath "data/gps.bin"
if (Test-Path $TempBin) { Remove-Item -Path $TempBin -Force }

Write-Host "   [3/3] Archiving as .zip..."
$ZipOutput = Join-Path $DistDir "$AppName-windows-amd64.zip"
# Compress-Archive has a bug if file already exists or with path matching, so we select children
Compress-Archive -Path (Join-Path $WindowsBuildPath "*") -DestinationPath $ZipOutput -Force
Write-Host "✅ Windows package completed: dist/$AppName-windows-amd64.zip" -ForegroundColor Green

# 3. Build and Package Linux
Write-Host "`n🐧 Building and packaging for Linux (amd64)..." -ForegroundColor Yellow
$LinuxBuildPath = Join-Path $BuildDir "linux_pkg"
New-Item -Path $LinuxBuildPath -ItemType Directory -Force | Out-Null

Write-Host "   [1/3] Compiling Go binary..."
$env:GOOS = "linux"
$env:GOARCH = "amd64"
go build -o (Join-Path $LinuxBuildPath $AppName) main.go

Write-Host "   [2/3] Preparing assets..."
Copy-Item -Path (Join-Path $WorkspaceDir "data") -Destination $LinuxBuildPath -Recurse -Force
Copy-Item -Path (Join-Path $WorkspaceDir "README.md") -Destination $LinuxBuildPath -Force

# Clean any temporary files
$TempBinLinux = Join-Path $LinuxBuildPath "data/gps.bin"
if (Test-Path $TempBinLinux) { Remove-Item -Path $TempBinLinux -Force }

Write-Host "   [3/3] Archiving as .tar.gz..."
$TarOutput = Join-Path $DistDir "$AppName-linux-amd64.tar.gz"

# Modern Windows 10/11 includes bsdtar as tar.exe by default
if (Get-Command "tar" -ErrorAction SilentlyContinue) {
    # Run tar.exe inside the build folder to avoid wrapping directory in archive
    Push-Location $LinuxBuildPath
    tar.exe -czf $TarOutput *
    Pop-Location
    Write-Host "✅ Linux package completed: dist/$AppName-linux-amd64.tar.gz" -ForegroundColor Green
} else {
    Write-Host "⚠️ Warning: 'tar' utility not found. Creating a Linux ZIP package instead..." -ForegroundColor DarkYellow
    $LinuxZipOutput = Join-Path $DistDir "$AppName-linux-amd64.zip"
    Compress-Archive -Path (Join-Path $LinuxBuildPath "*") -DestinationPath $LinuxZipOutput -Force
    Write-Host "✅ Linux package completed (as ZIP): dist/$AppName-linux-amd64.zip" -ForegroundColor Green
}

# Reset environment variables to default
$env:GOOS = ""
$env:GOARCH = ""

Write-Host "`n==================================================" -ForegroundColor Cyan
Write-Host "🎉 Packaging Process Finished Successfully!" -ForegroundColor Cyan
Write-Host "==================================================" -ForegroundColor Cyan
