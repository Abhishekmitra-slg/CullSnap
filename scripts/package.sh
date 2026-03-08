#!/bin/bash
# scripts/package.sh
# A unified unified build script that seamlessly figures out your OS and CPU architecture
# and compiles a production-ready Wails bundle for you.

set -e

echo "📸 Starting Unified CullSnap Build Pipeline"

# 1. Ensure Wails is installed
if ! command -v wails &> /dev/null; then
    echo "⚠️ Wails CLI not found. Installing github.com/wailsapp/wails/v2/cmd/wails@latest..."
    go install github.com/wailsapp/wails/v2/cmd/wails@latest
else
    echo "✅ Wails CLI detected."
fi

# 2. Determine OS via uname
OS="$(uname -s)"
ARCH="$(uname -m)"
PLATFORM=""

case "${OS}" in
    Linux*)     
        # Ensure GTK dependencies are satisfied on Linux before proceeding
        echo "🐧 Detected Linux"
        PLATFORM="linux"
        ;;
    Darwin*)    
        echo "🍎 Detected macOS"
        PLATFORM="darwin"
        ;;
    CYGWIN*|MINGW*|MINGW32*|MSYS*) 
        echo "🪟 Detected Windows"
        PLATFORM="windows"
        ;;
    *)          
        echo "❌ Unknown Operating System: ${OS}"
        exit 1
        ;;
esac

# 3. Determine Architecture
case "${ARCH}" in
    x86_64)  ARCH="amd64" ;;
    arm64)   ARCH="arm64" ;;
    aarch64) ARCH="arm64" ;;
    *)       
        echo "⚠️ Unknown Architecture: ${ARCH}. Defaulting to amd64."
        ARCH="amd64"
        ;;
esac

TARGET="${PLATFORM}/${ARCH}"
if [ "${PLATFORM}" = "darwin" ]; then
    # Usually macOS builds are shipped as universal binaries to support M1/M2 and Intel Macs simultaneously
    TARGET="darwin/universal"
fi

echo "🔨 Selected Target: ${TARGET}"

# 4. Trigger Wails Build
echo "🚀 Compiling..."
wails build -platform "${TARGET}" -clean -m

echo "✅ Build Complete! Your application is ready in the ./build/bin folder."
