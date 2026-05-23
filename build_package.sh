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

# Function to clean old builds
clean() {
    echo "🧹 Cleaning old build and dist directories..."
    rm -rf "${BUILD_DIR}"
    rm -rf "${DIST_DIR}"
    mkdir -p "${DIST_DIR}"
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
