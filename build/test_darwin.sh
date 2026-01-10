#!/bin/bash

# Test script for macOS (Darwin)

set -e

# Check for nopause/silent parameter
NO_PAUSE=0
if [ "$1" = "nopause" ] || [ "$1" = "silent" ]; then
    NO_PAUSE=1
    shift
fi

cd "$(dirname "$0")/.."

echo ""
echo "========================================"
echo "  Running Tests for Sing-Box Launcher"
echo "========================================"
echo ""

# ========================================
# Cleanup temporary directories AT START
# ========================================
echo "=== Cleaning temporary directories ==="

# Clean project temp directory
TEST_OUTPUT_DIR="temp/darwin"
if [ -d "$TEST_OUTPUT_DIR" ]; then
    echo "Removing project temp directory: $TEST_OUTPUT_DIR..."
    rm -rf "$TEST_OUTPUT_DIR"
fi

# Create clean directory for tests
mkdir -p "$TEST_OUTPUT_DIR/tmp"
mkdir -p "$TEST_OUTPUT_DIR/cache"
echo "Test output directory: $(pwd)/$TEST_OUTPUT_DIR"
echo "Cleanup completed. You can inspect $TEST_OUTPUT_DIR after tests run."
echo ""

# Check if Go is available
echo "=== Checking Go installation ==="
if ! command -v go &> /dev/null; then
    echo "!!! Go not found in PATH !!!"
    echo "Please install Go from https://go.dev/dl/"
    if [ $NO_PAUSE -eq 0 ]; then
        read -p "Press Enter to exit..."
    fi
    exit 1
fi

GOROOT=$(go env GOROOT)
echo "GOROOT=$GOROOT"
echo "Go version: $(go version)"
echo ""

# Set environment for tests
echo "=== Setting environment ==="
export CGO_ENABLED=1
export GOOS=darwin

# Set temporary directory for Go in project folder
export GOTMPDIR="$(pwd)/$TEST_OUTPUT_DIR/tmp"
export GOCACHE="$(pwd)/$TEST_OUTPUT_DIR/cache"

# Check for C compiler for CGO
if [ "$CGO_ENABLED" = "1" ]; then
    if ! command -v clang &> /dev/null && ! command -v gcc &> /dev/null; then
        echo "!!! WARNING: C compiler not found in PATH !!!"
        echo "CGO requires a C compiler. Please install Xcode Command Line Tools:"
        echo "  xcode-select --install"
        echo ""
        echo "!!! CGO tests may fail !!!"
    else
        if command -v clang &> /dev/null; then
            echo "C compiler found (clang):"
            clang --version | head -n 1
        else
            echo "C compiler found (gcc):"
            gcc --version | head -n 1
        fi
    fi
    echo ""
fi

# Parse test parameters
TEST_PACKAGE="./..."
TEST_FLAGS="-v"
TEST_RUN=""

# Check for 'short' mode
if [ "$1" = "short" ]; then
    TEST_FLAGS="-v -short"
    shift
    if [ -n "$1" ]; then
        TEST_PACKAGE="$1"
        shift
    fi
fi

# Check for 'run' mode
if [ "$1" = "run" ]; then
    if [ -z "$2" ]; then
        echo "!!! Error: 'run' requires test name pattern !!!"
        echo "Usage: $0 run TestName [package]"
        if [ $NO_PAUSE -eq 0 ]; then
            read -p "Press Enter to exit..."
        fi
        exit 1
    fi
    TEST_FLAGS="-v -run $2"
    shift 2
    if [ -n "$1" ]; then
        TEST_PACKAGE="$1"
    else
        TEST_PACKAGE="./..."
    fi
fi

# If there's still an argument, it's the package
if [ -n "$1" ] && [ "$1" != "nopause" ] && [ "$1" != "silent" ]; then
    TEST_PACKAGE="$1"
fi

# Run tests
echo ""
echo "========================================"
echo "  Running Tests"
echo "========================================"
echo ""

# Show list of packages that will be tested
echo "=== Packages to test ==="
go list $TEST_PACKAGE 2>/dev/null | while read -r pkg; do
    echo "  - $pkg"
done
echo ""

echo "CGO_ENABLED=$CGO_ENABLED"
echo "GOROOT=$GOROOT"
echo "GOTMPDIR=$GOTMPDIR"
echo "GOCACHE=$GOCACHE"
echo "Test package: $TEST_PACKAGE"
echo "Test flags: $TEST_FLAGS"
echo "Test binaries will be saved to: $(pwd)/$TEST_OUTPUT_DIR"
echo ""

# Show start time and create log file
TEST_LOG="$(pwd)/$TEST_OUTPUT_DIR/test_output.log"
echo "Test started at: $(date)"
echo "Test log will be saved to: $TEST_LOG"
echo ""
echo "Starting tests..."
echo ""

# Compute package list excluding UI packages and fyne imports
PKGS=$(go list $TEST_PACKAGE 2>/dev/null | grep -v '/ui/' | grep -v 'fyne.io' || true)
if [ -z "$PKGS" ]; then
    echo "No packages to test after filtering. Exiting."
    exit 0
fi

# Run tests with output to both file and screen
echo "Packages to be tested:" 
echo "$PKGS"
go test $TEST_FLAGS -count=1 $PKGS 2>&1 | tee "$TEST_LOG"
TEST_EXIT_CODE=${PIPESTATUS[0]}

# Show finish time
echo ""
echo "========================================"
echo "Test finished at: $(date)"
echo "Full test log: $TEST_LOG"
echo "========================================"

# After tests compile binaries for inspection
echo ""
echo "=== Compiling test binaries for inspection ==="
go list $TEST_PACKAGE 2>/dev/null | while read -r pkg; do
    # Convert package path to filename
    PKG_NAME=$(echo "$pkg" | sed 's|singbox-launcher/||g' | sed 's|/|_|g' | sed 's| |_|g')
    if [ -z "$PKG_NAME" ]; then
        PKG_NAME="main"
    fi
    OUTPUT_FILE="$TEST_OUTPUT_DIR/${PKG_NAME}.test"
    echo "Compiling $pkg..."
    if go test -c -o "$OUTPUT_FILE" "$pkg" 2>/dev/null; then
        echo "  Saved: $OUTPUT_FILE"
    else
        echo "  Failed to compile: $pkg"
    fi
done
echo ""

echo ""
echo "========================================"
if [ $TEST_EXIT_CODE -eq 0 ]; then
    echo "  All tests passed!"
else
    echo "  Some tests failed (exit code: $TEST_EXIT_CODE)"
fi
echo "========================================"
echo ""
echo "Test binaries saved to: $TEST_OUTPUT_DIR"
echo "You can inspect them manually before next run."
echo ""

if [ $NO_PAUSE -eq 0 ]; then
    read -p "Press Enter to exit..."
fi

exit $TEST_EXIT_CODE
