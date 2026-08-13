# Building on Linux

**🌐 Language**: English | [Русский](BUILD_LINUX.ru.md)

## Requirements

1. **Go 1.25** (or whatever `go.mod` says)
   - Install: [https://go.dev/dl/](https://go.dev/dl/) or your distro's package
   - Verify: `go version`

2. **System packages for CGO and Fyne (OpenGL + X11/GLFW)**

   Without them the build fails with errors like `Package gl was not found` or `X11/Xcursor/Xcursor.h: No such file or directory`.

   **Debian / Ubuntu:**
   ```bash
   sudo apt-get update && sudo apt-get install -y \
     build-essential pkg-config libgl1-mesa-dev libxcursor-dev \
     libxrandr-dev libxi-dev libxinerama-dev libxft-dev \
     libxkbcommon-x11-dev libxxf86vm-dev libwayland-dev
   ```

   **Fedora / RHEL:**
   ```bash
   sudo dnf install -y \
     mesa-libGL-devel libXcursor-devel libXrandr-devel libXi-devel \
     libXinerama-devel libXft-devel libxkbcommon-x11-devel \
     libXxf86vm-devel libwayland-devel
   ```

3. **CGO** — must be enabled (`CGO_ENABLED=1` by default).

## Building

### Option 1: the script (recommended)

The script checks for the dependencies and prints the install commands when they are missing.

```bash
cd /path/to/singbox-launcher
./build/build_linux.sh
```

Result: a `singbox-launcher` binary (or `singbox-launcher-1`, …) in the repository root.

### Option 2: build in Docker

If you would rather not install the system packages, build inside a container. Run it **from the repository root**:

```bash
docker build -f build/Dockerfile.linux --target export -o type=local,dest=. .
chmod +x singbox-launcher
```

The binary appears in the current directory.

### Option 3: manual build

Once the dependencies are installed:

```bash
export CGO_ENABLED=1
GOOS=linux GOARCH=amd64 go build -buildvcs=false -ldflags="-s -w" -o singbox-launcher
```

## Troubleshooting

### Package gl was not found / pkg-config

- Install `pkg-config` and the OpenGL packages: on Debian/Ubuntu that's `libgl1-mesa-dev`, see the "System packages" block above.

### X11/Xcursor/Xcursor.h: No such file or directory

- X11 headers are missing. On Debian/Ubuntu: `libxcursor-dev` and the rest of the list above (libxrandr-dev, libxi-dev and so on).

### Docker build: COPY failed / no such file

- Run `docker build` **from the repository root** (where `go.mod` and `go.sum` live), with `.` as the context and `-f build/Dockerfile.linux`.

## Running

```bash
./singbox-launcher
```

If `sing-box` is installed from a distro package and available on `PATH`, the launcher uses it; otherwise put the binary in `bin/sing-box` next to the launcher, or download it from the **Local** tab.

For TUN setup, see the main README (the section on Linux capabilities and `setcap`).
