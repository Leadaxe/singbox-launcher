#!/bin/bash

set -e

# Linux generic test runner filtering out GUI packages
NO_PAUSE=0
if [ "$1" = "nopause" ] || [ "$1" = "silent" ]; then
    NO_PAUSE=1
    shift
fi

cd "$(dirname "$0")/.."

TEST_OUTPUT_DIR="temp/linux"
mkdir -p "$TEST_OUTPUT_DIR/tmp" "$TEST_OUTPUT_DIR/cache"

export CGO_ENABLED=1
export GOOS=linux

export GOTMPDIR="$(pwd)/$TEST_OUTPUT_DIR/tmp"
export GOCACHE="$(pwd)/$TEST_OUTPUT_DIR/cache"

TEST_PACKAGE="./..."
TEST_FLAGS="-v -race -coverprofile=coverage.out"

# Compute package list excluding UI packages and fyne imports
PKGS=$(go list $TEST_PACKAGE 2>/dev/null | grep -v '/ui/' | grep -v 'fyne.io' || true)
if [ -z "$PKGS" ]; then
    echo "No packages to test after filtering. Exiting."
    exit 0
fi

echo "Packages to be tested:"
echo "$PKGS"

go test $TEST_FLAGS -count=1 $PKGS 2>&1 | tee "$TEST_OUTPUT_DIR/test_output.log"
EXIT_CODE=${PIPESTATUS[0]}

exit $EXIT_CODE
