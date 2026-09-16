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

   **Fedora:**
   ```bash
   sudo dnf install -y \
     mesa-libGL-devel libXcursor-devel libXrandr-devel libXi-devel \
     libXinerama-devel libXft-devel libxkbcommon-x11-devel \
     libXxf86vm-devel wayland-devel
   ```

   **RHEL 9 and compatible distributions:**

   The `libxkbcommon-x11-devel` and `libXxf86vm-devel` packages are provided by CodeReady Builder (CRB). Enable it first, then install the Fedora package list above.

   On RHEL with an active subscription:
   ```bash
   sudo subscription-manager repos \
     --enable="codeready-builder-for-rhel-9-$(arch)-rpms"
   ```

   On Rocky Linux 9 or AlmaLinux 9:
   ```bash
   sudo dnf install -y dnf-plugins-core
   sudo dnf config-manager --set-enabled crb
   ```

   **openSUSE (Leap / Tumbleweed):**
   ```bash
   sudo zypper install -y \
     gcc gcc-c++ make pkg-config Mesa-libGL-devel Mesa-libEGL-devel \
     libXcursor-devel libXrandr-devel libXi-devel libXinerama-devel \
     libXft-devel \
     libxkbcommon-x11-devel libXxf86vm-devel wayland-devel
   ```

3. **CGO** — must be enabled (`CGO_ENABLED=1` by default).

## Building

### Option 1: the script (recommended)

The script checks for the dependencies and prints the install commands when they are missing. It enables both X11 and native Wayland when their development files are available; otherwise it builds the X11 backend, which also works in Wayland sessions through XWayland.

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
export CGO_CFLAGS="$(pkg-config --cflags-only-I wayland-client wayland-cursor wayland-egl xkbcommon)"
GOOS=linux GOARCH=amd64 go build -buildvcs=false -ldflags="-s -w" -o singbox-launcher
```

## Troubleshooting

### Package gl was not found / pkg-config

- Install `pkg-config` and the OpenGL packages: on Debian/Ubuntu that's `libgl1-mesa-dev`; on openSUSE, `Mesa-libGL-devel`. See the "System packages" block above.

### X11/Xcursor/Xcursor.h: No such file or directory

- X11 headers are missing. On Debian/Ubuntu: `libxcursor-dev`; on openSUSE: `libXcursor-devel` and the rest of the list above (`libXrandr-devel`, `libXi-devel`, and so on).

### wayland-client-core.h: No such file or directory

- GLFW enables its Wayland backend on Linux. On openSUSE, Wayland headers live under `/usr/include/wayland`; `build_linux.sh` automatically adds this path from `pkg-config`. For a manual build, use the `CGO_CFLAGS` command shown above.
- Verify the setup with `pkg-config --cflags wayland-client`; on openSUSE it should include `-I/usr/include/wayland`.

### EGL/egl.h: No such file or directory

- EGL headers are needed for GLFW's native Wayland backend. They are installed transitively by the package lists tested on Debian 12, Ubuntu 24.04, Fedora 44, and Rocky Linux 9. If they are missing on another release, install `libegl1-mesa-dev` on Debian/Ubuntu, `mesa-libEGL-devel` on Fedora/RHEL, or `Mesa-libEGL-devel` on openSUSE. `build_linux.sh` falls back to the X11 backend when they are unavailable.

### Docker build: COPY failed / no such file

- Run `docker build` **from the repository root** (where `go.mod` and `go.sum` live), with `.` as the context and `-f build/Dockerfile.linux`.

## Running

```bash
./singbox-launcher
```

If `sing-box` is installed from a distro package and available on `PATH`, the launcher uses it; otherwise put the binary in `bin/sing-box` next to the launcher, or download it from the **Local** tab.

For TUN setup, see the main README (the section on Linux capabilities and `setcap`).
