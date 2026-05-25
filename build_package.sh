#!/usr/bin/env bash

# Exit immediately if a command exits with a non-zero status
set -e

# Define directories
WORKSPACE_DIR="$(pwd)"
BUILD_DIR="${WORKSPACE_DIR}/build"
DIST_DIR="${WORKSPACE_DIR}/dist"
APP_NAME="gps-simulator"

echo "=================================================="
echo "    GPS Signal Simulator Cross-Platform Packager"
echo "=================================================="

# Function to clean old builds (preserving cached repository clones)
clean() {
    echo "🧹 Cleaning old build and dist directories..."
    rm -rf "${BUILD_DIR}/linux_pkg"
    rm -rf "${BUILD_DIR}/windows_pkg"
    rm -rf "${DIST_DIR}"
    mkdir -p "${DIST_DIR}"
}

# Helper to compile B1I C simulator (beidou-sdr-sim) and B1C Rust simulator (gnss-signal-simulator)
compile_simulators() {
    TARGET_OS=$1
    DEST_DIR=$2
    
    echo "⚙️  Preparing C & Rust GNSS simulators for ${TARGET_OS}..."
    
    # 1. Compile B1I C Simulator (beidou-sdr-sim)
    echo "   [Sim] Compiling B1I C simulator (beidou-sdr-sim)..."
    C_BUILD_DIR="${BUILD_DIR}/beidou-sdr-sim-build"
    if [ ! -d "${C_BUILD_DIR}" ]; then
        echo "   [Sim] Cloning beidou-sdr-sim..."
        mkdir -p "${BUILD_DIR}"
        git clone --depth 1 https://github.com/yangfan852219770/beidou-sdr-sim.git "${C_BUILD_DIR}"
    fi
    
    C_OUT="${DEST_DIR}/beidou-sdr-sim"
    [ "${TARGET_OS}" = "windows" ] && C_OUT="${DEST_DIR}/beidou-sdr-sim.exe"
    
    COMPILE_C_SUCCESS=false
    if [ "${TARGET_OS}" = "linux" ]; then
        if [ "$(uname)" = "Linux" ]; then
            gcc "${C_BUILD_DIR}/beidou-sdr-sim.c" -O3 -lm -o "${C_OUT}" && COMPILE_C_SUCCESS=true
        else
            if command -v x86_64-linux-gnu-gcc >/dev/null 2>&1; then
                x86_64-linux-gnu-gcc "${C_BUILD_DIR}/beidou-sdr-sim.c" -O3 -lm -o "${C_OUT}" && COMPILE_C_SUCCESS=true
            else
                echo "   ⚠️ Warning: x86_64-linux-gnu-gcc not found. Skipping B1I C simulator cross-compilation."
            fi
        fi
    elif [ "${TARGET_OS}" = "windows" ]; then
        if [ "$(uname)" = "Darwin" ] || [ "$(uname)" = "Linux" ]; then
            if command -v x86_64-w64-mingw32-gcc >/dev/null 2>&1; then
                x86_64-w64-mingw32-gcc "${C_BUILD_DIR}/beidou-sdr-sim.c" -O3 -lm -o "${C_OUT}" && COMPILE_C_SUCCESS=true
            else
                echo "   ⚠️ Warning: x86_64-w64-mingw32-gcc not found. Skipping B1I C simulator cross-compilation."
            fi
        else
            gcc "${C_BUILD_DIR}/beidou-sdr-sim.c" -O3 -lm -o "${C_OUT}" && COMPILE_C_SUCCESS=true
        fi
    fi
    
    if [ "${COMPILE_C_SUCCESS}" = "true" ]; then
        echo "   ✅ B1I C simulator compiled successfully!"
    else
        echo "   ⚠️ Warning: B1I C simulator compilation skipped or failed."
    fi
    
    # 2. Compile B1C Rust Simulator (gnss-signal-simulator)
    echo "   [Sim] Compiling B1C Rust simulator (gnss-signal-simulator)..."
    RUST_BUILD_DIR="${BUILD_DIR}/gnss-signal-simulator-rs-build"
    if [ ! -d "${RUST_BUILD_DIR}" ]; then
        echo "   [Sim] Cloning gnss-signal-simulator-rs..."
        mkdir -p "${BUILD_DIR}"
        git clone --depth 1 https://github.com/danusha2345/gnss-signal-simulator-rs.git "${RUST_BUILD_DIR}"
    fi
    
    RUST_OUT="${DEST_DIR}/beidou-sdr-sim-b1c"
    [ "${TARGET_OS}" = "windows" ] && RUST_OUT="${DEST_DIR}/beidou-sdr-sim-b1c.exe"
    
    COMPILE_RUST_SUCCESS=false
    if command -v cargo >/dev/null 2>&1; then
        cd "${RUST_BUILD_DIR}"
        if [ "${TARGET_OS}" = "linux" ]; then
            if [ "$(uname)" = "Linux" ]; then
                cargo build --release --bin gnss_rust && \
                cp "target/release/gnss_rust" "${RUST_OUT}" && COMPILE_RUST_SUCCESS=true
            else
                if rustup target list | grep "x86_64-unknown-linux-gnu (installed)" >/dev/null 2>&1 || rustup target add x86_64-unknown-linux-gnu >/dev/null 2>&1; then
                    if command -v x86_64-linux-gnu-gcc >/dev/null 2>&1; then
                        CARGO_TARGET_X86_64_UNKNOWN_LINUX_GNU_LINKER=x86_64-linux-gnu-gcc cargo build --release --bin gnss_rust --target x86_64-unknown-linux-gnu && \
                        cp "target/x86_64-unknown-linux-gnu/release/gnss_rust" "${RUST_OUT}" && COMPILE_RUST_SUCCESS=true
                    else
                        cargo build --release --bin gnss_rust --target x86_64-unknown-linux-gnu && \
                        cp "target/x86_64-unknown-linux-gnu/release/gnss_rust" "${RUST_OUT}" && COMPILE_RUST_SUCCESS=true
                    fi
                fi
            fi
        elif [ "${TARGET_OS}" = "windows" ]; then
            if [ "$(uname)" = "Darwin" ] || [ "$(uname)" = "Linux" ]; then
                if rustup target list | grep "x86_64-pc-windows-gnu (installed)" >/dev/null 2>&1 || rustup target add x86_64-pc-windows-gnu >/dev/null 2>&1; then
                    if command -v x86_64-w64-mingw32-gcc >/dev/null 2>&1; then
                        CARGO_TARGET_X86_64_PC_WINDOWS_GNU_LINKER=x86_64-w64-mingw32-gcc cargo build --release --bin gnss_rust --target x86_64-pc-windows-gnu && \
                        cp "target/x86_64-pc-windows-gnu/release/gnss_rust.exe" "${RUST_OUT}" && COMPILE_RUST_SUCCESS=true
                    else
                        cargo build --release --bin gnss_rust --target x86_64-pc-windows-gnu && \
                        cp "target/x86_64-pc-windows-gnu/release/gnss_rust.exe" "${RUST_OUT}" && COMPILE_RUST_SUCCESS=true
                    fi
                fi
            else
                cargo build --release --bin gnss_rust && \
                cp "target/release/gnss_rust.exe" "${RUST_OUT}" && COMPILE_RUST_SUCCESS=true
            fi
        fi
        cd "${WORKSPACE_DIR}"
    else
        echo "   ⚠️ Warning: 'cargo' not found in PATH. Skipping Rust B1C simulator compilation."
    fi
    
    if [ "${COMPILE_RUST_SUCCESS}" = "true" ]; then
        echo "   ✅ B1C Rust simulator compiled successfully!"
    else
        echo "   ⚠️ Warning: B1C Rust simulator compilation skipped or failed."
    fi
}

# Function to package Linux build
build_linux() {
    echo "🐧 Building and packaging for Linux (amd64)..."
    
    # 1. Compile
    echo "   [1/3] Compiling Go binary..."
    LINUX_BUILD_PATH="${BUILD_DIR}/linux_pkg"
    mkdir -p "${LINUX_BUILD_PATH}"
    GOOS=linux GOARCH=amd64 go build -o "${LINUX_BUILD_PATH}/${APP_NAME}" "${WORKSPACE_DIR}"
    
    # 2. Prepare Assets
    echo "   [2/3] Preparing assets..."
    cp -r "${WORKSPACE_DIR}/data" "${LINUX_BUILD_PATH}/"
    cp "${WORKSPACE_DIR}/README.md" "${LINUX_BUILD_PATH}/"
    
    # Clean temporary files from data folder in package
    rm -f "${LINUX_BUILD_PATH}/data/gps.bin"
    
    # Compile C and Rust simulators
    compile_simulators "linux" "${LINUX_BUILD_PATH}"
    
    # 3. Compress
    echo "   [3/3] Archiving as .tar.gz..."
    cd "${LINUX_BUILD_PATH}"
    tar -czf "${DIST_DIR}/${APP_NAME}-linux-amd64.tar.gz" *
    cd "${WORKSPACE_DIR}"
    
    echo "✅ Linux package completed: dist/${APP_NAME}-linux-amd64.tar.gz"
}

# Function to package Windows build
build_windows() {
    echo "🏁 Building and packaging for Windows (amd64)..."
    
    # 1. Compile
    echo "   [1/3] Compiling Go binary..."
    WINDOWS_BUILD_PATH="${BUILD_DIR}/windows_pkg"
    mkdir -p "${WINDOWS_BUILD_PATH}"
    GOOS=windows GOARCH=amd64 go build -o "${WINDOWS_BUILD_PATH}/${APP_NAME}.exe" "${WORKSPACE_DIR}"
    
    # 2. Prepare Assets
    echo "   [2/3] Preparing assets..."
    cp -r "${WORKSPACE_DIR}/data" "${WINDOWS_BUILD_PATH}/"
    cp "${WORKSPACE_DIR}/README.md" "${WINDOWS_BUILD_PATH}/"
    cp "${WORKSPACE_DIR}/README_Windows.md" "${WINDOWS_BUILD_PATH}/"
    
    # Clean temporary files from data folder in package
    rm -f "${WINDOWS_BUILD_PATH}/data/gps.bin"
    
    # Compile C and Rust simulators
    compile_simulators "windows" "${WINDOWS_BUILD_PATH}"
    
    # 3. Compress
    echo "   [3/3] Archiving as .zip..."
    cd "${WINDOWS_BUILD_PATH}"
    
    # Check if zip is available, if not fallback to python or display error
    if command -v zip >/dev/null 2>&1; then
        zip -r "${DIST_DIR}/${APP_NAME}-windows-amd64.zip" *
    elif command -v python3 >/dev/null 2>&1; then
        python3 -c "import shutil; shutil.make_archive('${DIST_DIR}/${APP_NAME}-windows-amd64', 'zip', '${WINDOWS_BUILD_PATH}')"
    else
        echo "❌ Error: 'zip' command not found. Cannot create zip archive."
        exit 1
    fi
    
    cd "${WORKSPACE_DIR}"
    echo "✅ Windows package completed: dist/${APP_NAME}-windows-amd64.zip"
}

# Parse argument
TARGET="${1:-all}"

clean

case "${TARGET}" in
    "linux")
        build_linux
        ;;
    "windows")
        build_windows
        ;;
    "all")
        build_linux
        build_windows
        ;;
    *)
        echo "❌ Unknown build target: ${TARGET}"
        echo "Usage: $0 [linux|windows|all]"
        exit 1
        ;;
esac

echo "=================================================="
echo "🎉 Packaging Process Finished Successfully!"
echo "=================================================="
