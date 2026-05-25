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

# 1. Clean Directories (preserving cached repository clones)
Write-Host "🧹 Cleaning old build and dist directories..." -ForegroundColor Yellow
$WindowsBuildPath = Join-Path $BuildDir "windows_pkg"
$LinuxBuildPath = Join-Path $BuildDir "linux_pkg"
if (Test-Path $WindowsBuildPath) { Remove-Item -Path $WindowsBuildPath -Recurse -Force }
if (Test-Path $LinuxBuildPath) { Remove-Item -Path $LinuxBuildPath -Recurse -Force }
if (Test-Path $DistDir) { Remove-Item -Path $DistDir -Recurse -Force }
New-Item -Path $DistDir -ItemType Directory -Force | Out-Null

# Helper function to compile B1I C simulator (beidou-sdr-sim) and B1C Rust simulator (gnss-signal-simulator)
function Compile-Simulators($TargetOS, $DestDir) {
    Write-Host "`n⚙️  Preparing C & Rust GNSS simulators for $TargetOS..." -ForegroundColor Cyan
    
    if (!(Test-Path $BuildDir)) { New-Item -Path $BuildDir -ItemType Directory -Force | Out-Null }
    
    # 1. Compile B1I C Simulator (beidou-sdr-sim)
    Write-Host "   [Sim] Compiling B1I C simulator (beidou-sdr-sim)..."
    $CBuildDir = Join-Path $BuildDir "beidou-sdr-sim-build"
    if (!(Test-Path $CBuildDir)) {
        Write-Host "   [Sim] Cloning beidou-sdr-sim..."
        git clone --depth 1 https://github.com/yangfan852219770/beidou-sdr-sim.git $CBuildDir
    }
    
    $COut = Join-Path $DestDir "beidou-sdr-sim"
    if ($TargetOS -eq "windows") { $COut += ".exe" }
    
    $CompileCSuccess = $false
    try {
        if ($TargetOS -eq "windows") {
            if (Get-Command "gcc" -ErrorAction SilentlyContinue) {
                gcc (Join-Path $CBuildDir "beidou-sdr-sim.c") -O3 -lm -o $COut
                $CompileCSuccess = $true
            } else {
                Write-Host "   ⚠️ Warning: 'gcc' not found in PATH. Skipping B1I C simulator compilation." -ForegroundColor Yellow
            }
        } elseif ($TargetOS -eq "linux") {
            if (Get-Command "x86_64-linux-gnu-gcc" -ErrorAction SilentlyContinue) {
                x86_64-linux-gnu-gcc (Join-Path $CBuildDir "beidou-sdr-sim.c") -O3 -lm -o $COut
                $CompileCSuccess = $true
            } else {
                Write-Host "   ⚠️ Warning: 'x86_64-linux-gnu-gcc' not found. Skipping B1I C simulator cross-compilation." -ForegroundColor Yellow
            }
        }
    } catch {
        Write-Host "   ⚠️ Warning: B1I C simulator compilation failed." -ForegroundColor Yellow
    }
    
    if ($CompileCSuccess) {
        Write-Host "   ✅ B1I C simulator compiled successfully!" -ForegroundColor Green
    }
    
    # 2. Compile B1C Rust Simulator (gnss-signal-simulator)
    Write-Host "   [Sim] Compiling B1C Rust simulator (gnss-signal-simulator)..."
    $RustBuildDir = Join-Path $BuildDir "gnss-signal-simulator-rs-build"
    if (!(Test-Path $RustBuildDir)) {
        Write-Host "   [Sim] Cloning gnss-signal-simulator-rs..."
        git clone --depth 1 https://github.com/danusha2345/gnss-signal-simulator-rs.git $RustBuildDir
    }
    
    $RustOut = Join-Path $DestDir "beidou-sdr-sim-b1c"
    if ($TargetOS -eq "windows") { $RustOut += ".exe" }
    
    $CompileRustSuccess = $false
    if (Get-Command "cargo" -ErrorAction SilentlyContinue) {
        Push-Location $RustBuildDir
        try {
            if ($TargetOS -eq "windows") {
                cargo build --release --bin gnss_rust
                Copy-Item "target/release/gnss_rust.exe" -Destination $RustOut -Force
                $CompileRustSuccess = $true
            } elseif ($TargetOS -eq "linux") {
                if (rustup target list | Select-String "x86_64-unknown-linux-gnu (installed)") {
                    cargo build --release --bin gnss_rust --target x86_64-unknown-linux-gnu
                    Copy-Item "target/x86_64-unknown-linux-gnu/release/gnss_rust" -Destination $RustOut -Force
                    $CompileRustSuccess = $true
                } else {
                    Write-Host "   ⚙️ Installing rust target: x86_64-unknown-linux-gnu..." -ForegroundColor DarkCyan
                    rustup target add x86_64-unknown-linux-gnu
                    cargo build --release --bin gnss_rust --target x86_64-unknown-linux-gnu
                    Copy-Item "target/x86_64-unknown-linux-gnu/release/gnss_rust" -Destination $RustOut -Force
                    $CompileRustSuccess = $true
                }
            }
        } catch {
            Write-Host "   ⚠️ Warning: B1C Rust simulator compilation failed." -ForegroundColor Yellow
        }
        Pop-Location
    } else {
        Write-Host "   ⚠️ Warning: 'cargo' not found in PATH. Skipping Rust B1C simulator compilation." -ForegroundColor Yellow
    }
    
    if ($CompileRustSuccess) {
        Write-Host "   ✅ B1C Rust simulator compiled successfully!" -ForegroundColor Green
    }
}

# 2. Build and Package Windows
Write-Host "`n🏁 Building and packaging for Windows (amd64)..." -ForegroundColor Yellow
New-Item -Path $WindowsBuildPath -ItemType Directory -Force | Out-Null

Write-Host "   [1/3] Compiling Go binary..."
$env:GOOS = "windows"
$env:GOARCH = "amd64"
go build -o (Join-Path $WindowsBuildPath "$AppName.exe") .

Write-Host "   [2/3] Preparing assets..."
Copy-Item -Path (Join-Path $WorkspaceDir "data") -Destination $WindowsBuildPath -Recurse -Force
Copy-Item -Path (Join-Path $WorkspaceDir "README.md") -Destination $WindowsBuildPath -Force
Copy-Item -Path (Join-Path $WorkspaceDir "README_Windows.md") -Destination $WindowsBuildPath -Force

# Clean any temporary files
$TempBin = Join-Path $WindowsBuildPath "data/gps.bin"
if (Test-Path $TempBin) { Remove-Item -Path $TempBin -Force }

# Compile C and Rust simulators
Compile-Simulators "windows" $WindowsBuildPath

Write-Host "   [3/3] Archiving as .zip..."
$ZipOutput = Join-Path $DistDir "$AppName-windows-amd64.zip"
Compress-Archive -Path (Join-Path $WindowsBuildPath "*") -DestinationPath $ZipOutput -Force
Write-Host "✅ Windows package completed: dist/$AppName-windows-amd64.zip" -ForegroundColor Green

# 3. Build and Package Linux
Write-Host "`n🐧 Building and packaging for Linux (amd64)..." -ForegroundColor Yellow
New-Item -Path $LinuxBuildPath -ItemType Directory -Force | Out-Null

Write-Host "   [1/3] Compiling Go binary..."
$env:GOOS = "linux"
$env:GOARCH = "amd64"
go build -o (Join-Path $LinuxBuildPath $AppName) .

Write-Host "   [2/3] Preparing assets..."
Copy-Item -Path (Join-Path $WorkspaceDir "data") -Destination $LinuxBuildPath -Recurse -Force
Copy-Item -Path (Join-Path $WorkspaceDir "README.md") -Destination $LinuxBuildPath -Force

# Clean any temporary files
$TempBinLinux = Join-Path $LinuxBuildPath "data/gps.bin"
if (Test-Path $TempBinLinux) { Remove-Item -Path $TempBinLinux -Force }

# Compile C and Rust simulators
Compile-Simulators "linux" $LinuxBuildPath

Write-Host "   [3/3] Archiving as .tar.gz..."
$TarOutput = Join-Path $DistDir "$AppName-linux-amd64.tar.gz"

if (Get-Command "tar" -ErrorAction SilentlyContinue) {
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
