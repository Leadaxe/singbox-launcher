#!/bin/bash

# Build script for macOS (Darwin)

set -e

cd "$(dirname "$0")/.."

# Parse args first so --help exits without go mod tidy / full checks
BUILD_TYPE="universal"
COPY_TO_APPLICATIONS=false
BUILD_TYPE_SET=false

print_usage() {
    echo "Usage: $0 [options] [universal|intel|arm64|catalina]"
    echo ""
    echo "Build types:"
    echo "  universal - Universal binary (arm64 + amd64), slower; default if omitted"
    echo "  intel     - amd64 only (macOS 11.0+)"
    echo "  arm64     - Apple Silicon only (macOS 11.0+), fast on M-series Macs"
    echo "  catalina  - amd64 only, minimum macOS 10.15"
    echo ""
    echo "Options:"
    echo "  -i                                  Install/update in /Applications: if the app already"
    echo "                                      exists, only the executable is replaced (bin/, logs/ kept);"
    echo "                                      otherwise the full .app bundle is copied (first install)."
    echo "                                      The built .app in the repo directory is removed afterward."
    echo "  -h, --help                          Show this help"
}

for arg in "$@"; do
    case "$arg" in
        -h|--help)
            print_usage
            exit 0
            ;;
        -i)
            COPY_TO_APPLICATIONS=true
            ;;
        universal|intel|arm64|catalina)
            if [ "$BUILD_TYPE_SET" = true ]; then
                echo "ERROR: build type specified more than once (already: $BUILD_TYPE)"
                exit 1
            fi
            BUILD_TYPE="$arg"
            BUILD_TYPE_SET=true
            ;;
        *)
            echo "ERROR: unknown argument: $arg"
            echo ""
            print_usage
            exit 1
            ;;
    esac
done

echo ""
echo "========================================"
echo "  Building Sing-Box Launcher (macOS)"
echo "========================================"
echo ""

echo "=== Checking build tools ==="
# Check for Xcode or Command Line Tools
if ! command -v xcrun &> /dev/null; then
    echo "ERROR: xcrun not found. Please install Xcode or Command Line Tools:"
    echo "  xcode-select --install"
    exit 1
fi

# Get SDK path and version
SDK_PATH=$(xcrun --show-sdk-path 2>/dev/null || echo "")
if [ -z "$SDK_PATH" ]; then
    echo "ERROR: Cannot find macOS SDK. Please install Xcode or Command Line Tools:"
    echo "  xcode-select --install"
    exit 1
fi

SDK_VERSION=$(xcrun --show-sdk-version 2>/dev/null || echo "unknown")
echo "SDK Path: $SDK_PATH"
echo "SDK Version: $SDK_VERSION"

echo ""
if [[ -n "${GITHUB_ACTIONS:-}" ]]; then
  echo "=== Skipping 'go mod tidy' in CI (GITHUB_ACTIONS=true) ==="
else
  echo "=== Tidying Go modules ==="
  go mod tidy
fi

echo ""
echo "=== Setting build environment ==="
export CGO_ENABLED=1
export GOOS=darwin

if [ "$BUILD_TYPE" = "universal" ]; then
    # Check for lipo (required for universal binary)
    if ! command -v lipo &> /dev/null; then
        echo "ERROR: lipo not found. Please install Xcode or Command Line Tools:"
        echo "  xcode-select --install"
        exit 1
    fi
    echo "Building universal binary for both architectures (arm64 + amd64)..."
    MIN_MACOS_VERSION="11.0"
elif [ "$BUILD_TYPE" = "intel" ]; then
    echo "Building Intel-only binary (amd64)..."
    MIN_MACOS_VERSION="11.0"
elif [ "$BUILD_TYPE" = "arm64" ]; then
    echo "Building Apple Silicon-only binary (arm64)..."
    MIN_MACOS_VERSION="11.0"
else
    echo "Building Intel-only Catalina binary (amd64) targeting macOS 10.15..."
    MIN_MACOS_VERSION="10.15"
fi

# Check if full Xcode is required (Command Line Tools have incomplete SDK)
UTCORETYPES_H="$SDK_PATH/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Headers/UTCoreTypes.h"
CURRENT_DEVELOPER_DIR=$(xcode-select -p 2>/dev/null || echo "")

# Check if UTCoreTypes.h exists (only in full Xcode SDK)
if [ ! -f "$UTCORETYPES_H" ]; then
    echo ""
    echo "❌ ERROR: Full Xcode is required for building this project!"
    echo "   Command Line Tools have incomplete SDK headers (missing UTCoreTypes.h)."
    echo ""
    
    # Check if Xcode is installed but not selected
    if [ -d "/Applications/Xcode.app" ]; then
        if [[ "$CURRENT_DEVELOPER_DIR" != *"Xcode.app"* ]]; then
            echo "   💡 Full Xcode detected at /Applications/Xcode.app but not selected."
            echo "   Switch to Xcode with:"
            echo "   sudo xcode-select --switch /Applications/Xcode.app"
            echo ""
            echo "   Or accept the Xcode license first:"
            echo "   sudo xcodebuild -license accept"
            exit 1
        fi
    else
        echo "   Please install Xcode from the App Store:"
        echo "   https://apps.apple.com/app/xcode/id497799835"
        echo ""
        echo "   After installation, accept the license:"
        echo "   sudo xcodebuild -license accept"
        echo ""
        echo "   Then switch to Xcode:"
        echo "   sudo xcode-select --switch /Applications/Xcode.app"
        exit 1
    fi
fi

# Set SDK path for CGO compiler
export SDKROOT="$SDK_PATH"

# Set minimum macOS version for CGO compiler
export CGO_CFLAGS="-mmacosx-version-min=$MIN_MACOS_VERSION"
export CGO_LDFLAGS="-mmacosx-version-min=$MIN_MACOS_VERSION"

# Determine output filename
BASE_NAME="singbox-launcher"
EXTENSION=""
OUTPUT_FILENAME="${BASE_NAME}${EXTENSION}"
COUNTER=0

while [ -f "$OUTPUT_FILENAME" ]; do
    COUNTER=$((COUNTER + 1))
    OUTPUT_FILENAME="${BASE_NAME}-${COUNTER}${EXTENSION}"
done

echo "Using output file: $OUTPUT_FILENAME"

echo ""
echo "=== Getting version ==="
if [ -n "${APP_VERSION:-}" ]; then
    VERSION="$APP_VERSION"
    echo "Version (from APP_VERSION): $VERSION"
else
    VERSION=$(git describe --tags --always --dirty --exclude='*-prerelease' 2>/dev/null || echo "")
    if [ -n "$VERSION" ]; then
        echo "Version (from git describe): $VERSION"
    else
        VERSION="unnamed-dev"
        echo "Version (default): $VERSION"
    fi
fi
echo "Minimum macOS version: $MIN_MACOS_VERSION"

# RequiredTemplateRef is pinned to the exact commit being built. Each release
# binary thus fetches wizard_template.json from that frozen commit, never the
# branch HEAD — see SPEC 046.
TEMPLATE_REF=$(git rev-parse HEAD 2>/dev/null || echo "")
if [ -z "$TEMPLATE_REF" ]; then
    echo "!!! Could not resolve git HEAD for RequiredTemplateRef; aborting !!!"
    exit 1
fi
echo "Template ref: $TEMPLATE_REF"

LDFLAGS="-s -w -X singbox-launcher/internal/constants.AppVersion=$VERSION -X singbox-launcher/internal/constants.RequiredTemplateRef=$TEMPLATE_REF"

if [ "$BUILD_TYPE" = "universal" ]; then
    echo ""
    echo "=== Building for arm64 (Apple Silicon) ==="
    TEMP_ARM64="${BASE_NAME}_arm64"
    GOARCH=arm64 go build -buildvcs=false -ldflags="$LDFLAGS" -o "$TEMP_ARM64"

    if [ $? -ne 0 ]; then
        echo "!!! Build failed for arm64 !!!"
        exit 1
    fi

    echo ""
    echo "=== Building for amd64 (Intel) ==="
    TEMP_AMD64="${BASE_NAME}_amd64"
    GOARCH=amd64 go build -buildvcs=false -ldflags="$LDFLAGS" -o "$TEMP_AMD64"

    if [ $? -ne 0 ]; then
        echo "!!! Build failed for amd64 !!!"
        rm -f "$TEMP_ARM64"
        exit 1
    fi

    echo ""
    echo "=== Creating universal binary ==="
    lipo -create -output "$OUTPUT_FILENAME" "$TEMP_ARM64" "$TEMP_AMD64"
    LIPO_STATUS=$?

    # Clean up temporary binaries
    rm -f "$TEMP_ARM64" "$TEMP_AMD64"

    if [ $LIPO_STATUS -ne 0 ]; then
        echo "!!! Failed to create universal binary !!!"
        exit 1
    fi

    echo "Universal binary created: $OUTPUT_FILENAME"
    # Verify the binary contains both architectures
    echo "Binary architectures:"
    lipo -info "$OUTPUT_FILENAME"
elif [ "$BUILD_TYPE" = "arm64" ]; then
    echo ""
    echo "=== Building for arm64 (Apple Silicon) ==="
    GOARCH=arm64 go build -buildvcs=false -ldflags="$LDFLAGS" -o "$OUTPUT_FILENAME"

    if [ $? -ne 0 ]; then
        echo "!!! Build failed for arm64 !!!"
        exit 1
    fi

    echo "Apple Silicon binary created: $OUTPUT_FILENAME"
    echo "Binary architecture:"
    file "$OUTPUT_FILENAME"
else
    echo ""
    echo "=== Building for amd64 (Intel) ==="
    GOARCH=amd64 go build -buildvcs=false -ldflags="$LDFLAGS" -o "$OUTPUT_FILENAME"

    if [ $? -ne 0 ]; then
        echo "!!! Build failed for amd64 !!!"
        exit 1
    fi

    echo "Intel binary created: $OUTPUT_FILENAME"
    echo "Binary architecture:"
    file "$OUTPUT_FILENAME"
fi

# Note: For the 'catalina' build type the script runs the same amd64 build
# but sets the minimum supported macOS version to 10.15 earlier, so the
# produced binary and .app will target macOS 10.15 (Catalina). If you need
# to sign or notarize the binary on Catalina, run the corresponding steps
# on a machine with Xcode and proper certificates.

echo ""
echo "=== Creating .app bundle ==="

# Create .app bundle structure
APP_NAME="${BASE_NAME}.app"
APP_CONTENTS="$APP_NAME/Contents"
APP_MACOS="$APP_CONTENTS/MacOS"
APP_RESOURCES="$APP_CONTENTS/Resources"

# Remove old bundle if exists
rm -rf "$APP_NAME"

# Create directory structure
mkdir -p "$APP_MACOS"
mkdir -p "$APP_RESOURCES"

# Move binary to MacOS directory
mv "$OUTPUT_FILENAME" "$APP_MACOS/$BASE_NAME"
chmod +x "$APP_MACOS/$BASE_NAME"

# Handle application icon
ICON_FILE="assets/app.icns"
ICON_NAME="app"
HAS_ICON=false

if [ -f "$ICON_FILE" ]; then
    echo "=== Copying application icon ==="
    cp "$ICON_FILE" "$APP_RESOURCES/${ICON_NAME}.icns"
    HAS_ICON=true
    echo "Icon copied: $ICON_FILE -> $APP_RESOURCES/${ICON_NAME}.icns"
else
    echo "=== Icon not found: $ICON_FILE (skipping icon setup) ==="
fi

# Create Info.plist with optional icon
{
    echo '<?xml version="1.0" encoding="UTF-8"?>'
    echo '<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">'
    echo '<plist version="1.0">'
    echo '<dict>'
    echo '    <key>CFBundleExecutable</key>'
    echo "    <string>$BASE_NAME</string>"
    echo '    <key>CFBundleIdentifier</key>'
    echo '    <string>com.singbox.launcher</string>'
    echo '    <key>CFBundleName</key>'
    echo '    <string>Sing-Box Launcher</string>'
    echo '    <key>CFBundlePackageType</key>'
    echo '    <string>APPL</string>'
    if [ "$HAS_ICON" = true ]; then
        echo '    <key>CFBundleIconFile</key>'
        echo "    <string>$ICON_NAME</string>"
    fi
    echo '    <key>CFBundleShortVersionString</key>'
    echo "    <string>$VERSION</string>"
    echo '    <key>CFBundleVersion</key>'
    echo "    <string>$VERSION</string>"
    echo '    <key>LSMinimumSystemVersion</key>'
    echo "    <string>$MIN_MACOS_VERSION</string>"
    if [ "$BUILD_TYPE" = "universal" ]; then
        echo '    <key>LSArchitecturePriority</key>'
        echo '    <array>'
        echo '        <string>arm64</string>'
        echo '        <string>x86_64</string>'
        echo '    </array>'
    elif [ "$BUILD_TYPE" = "arm64" ]; then
        echo '    <key>LSArchitecturePriority</key>'
        echo '    <array>'
        echo '        <string>arm64</string>'
        echo '    </array>'
    fi
    echo '    <key>NSHighResolutionCapable</key>'
    echo '    <true/>'
    echo '    <key>LSUIElement</key>'
    echo '    <false/>'
    echo '</dict>'
    echo '</plist>'
} > "$APP_CONTENTS/Info.plist"

echo "Created .app bundle: $APP_NAME"
echo ""
echo "========================================"
echo "  Build completed successfully!"
if [ "$BUILD_TYPE" = "universal" ]; then
    echo "  Output: $APP_NAME (universal binary: arm64 + amd64)"
    echo "  Minimum macOS: $MIN_MACOS_VERSION (Big Sur+)"
    echo "  Supports: Apple Silicon and Intel Macs"
elif [ "$BUILD_TYPE" = "arm64" ]; then
    echo "  Output: $APP_NAME (Apple Silicon-only: arm64)"
    echo "  Minimum macOS: $MIN_MACOS_VERSION (Big Sur+)"
    echo "  Supports: Apple Silicon Macs only"
else
    echo "  Output: $APP_NAME (Intel-only binary: amd64)"
    if [ "$BUILD_TYPE" = "catalina" ]; then
        echo "  Minimum macOS: $MIN_MACOS_VERSION (Catalina+)"
    else
        echo "  Minimum macOS: $MIN_MACOS_VERSION (Big Sur+)"
    fi
    echo "  Supports: Intel Macs only"
fi
if [ "$COPY_TO_APPLICATIONS" = true ]; then
    echo "  Run with: open /Applications/$APP_NAME"
else
    echo "  Run with: open $APP_NAME"
fi
echo ""

if [ "$COPY_TO_APPLICATIONS" = true ]; then
    echo "=== Installing to /Applications ==="
    DEST_APP="/Applications/$APP_NAME"
    DEST_BIN="$DEST_APP/Contents/MacOS/$BASE_NAME"
    SRC_BIN="$(pwd)/$APP_NAME/Contents/MacOS/$BASE_NAME"

    if [ -d "$DEST_APP" ]; then
        echo "Existing $APP_NAME found — updating executable only (Contents/MacOS/bin/, logs/ unchanged)."
        cp "$SRC_BIN" "$DEST_BIN"
        chmod +x "$DEST_BIN"
        echo "Updated: $DEST_BIN"
    else
        echo "No $APP_NAME in /Applications — copying full bundle (first install)."
        cp -R "$APP_NAME" /Applications/
        echo "Installed: $DEST_APP"
    fi
    rm -rf "$APP_NAME"
    echo "Removed local bundle: $(pwd)/$APP_NAME"
    echo "========================================"
else
    echo "To install or update in /Applications:"
    echo "  $0 -i $BUILD_TYPE   # updates binary only if the app is already there; else full .app"
    echo ""
    echo "Manual full copy (replaces entire app, wipes data in Contents/MacOS/bin/):"
    echo "  cp -R \"$(pwd)/$APP_NAME\" /Applications/"
    echo "========================================"
fi

