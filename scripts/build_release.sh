#!/bin/sh
set -e

# Script to build ACORDE binaries
# Usage: ./scripts/build_release.sh

echo "🏗️  Building ACORDE..."

# Check for Go
if ! command -v go >/dev/null 2>&1; then
    echo "❌ Error: Go is not installed."
    exit 1
fi

# Check for C compiler (required for SQLite)
if ! command -v gcc >/dev/null 2>&1; then
    echo "❌ Error: GCC is not installed. Required for SQLite dependency."
    echo "   Ubuntu/Debian: sudo apt install build-essential"
    echo "   Fedora: sudo dnf groupinstall \"Development Tools\""
    echo "   macOS: xcode-select --install"
    exit 1
fi

echo "✅ Found Go and GCC."

# Create output dir
mkdir -p build

# Build
echo "🚀 Compiling..."
go build -ldflags="-s -w" -o build/acorde ./cmd/acorde

echo "✅ Build success! Binary is at: build/acorde"
echo "   Run with: ./build/acorde daemon"
