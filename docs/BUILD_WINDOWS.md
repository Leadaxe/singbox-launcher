# Building on Windows

**🌐 Language**: English | [Русский](BUILD_WINDOWS.ru.md)

## 📋 Requirements

1. **Go 1.25 or newer**
   - Download from [https://go.dev/dl/](https://go.dev/dl/)
   - Install into the default folder `C:\Program Files\Go`
   - Verify the installation: `go version`

2. **A C compiler (GCC) — MANDATORY**
   - Fyne requires CGO, and CGO requires GCC
   - **Install one of these:**
     - [TDM-GCC](https://jmeubank.github.io/tdm-gcc/) — the simplest option
     - [MinGW-w64](https://www.mingw-w64.org/) — via MSYS2 or WinLibs
   - After installing, add GCC's `bin` folder to PATH
   - **Important:** restart the command prompt after installing!
   - Verify: `gcc --version`

3. **CGO** (enabled by default)
   - Fyne needs CGO to work
   - Make sure `CGO_ENABLED=1`

4. **Optional: rsrc** (for embedding the icon)
   ```bash
   go install github.com/akavel/rsrc@latest
   ```
   After installing, `rsrc.exe` lives in `%USERPROFILE%\go\bin\`

## 🔨 Building

### Option 1: use the script (recommended)

1. Open a command prompt (CMD) or PowerShell in the project folder
2. Run the build script:

```batch
build\build_windows.bat
```

Or from the project root:

```batch
.\build\build_windows.bat
```

The script automatically:
- Updates dependencies (`go mod tidy`)
- Embeds the icon (if rsrc is installed)
- Builds the project
- Produces `singbox-launcher.exe` in the project root

### Option 2: manual build

1. Open a command prompt in the project folder

2. Update dependencies:
```batch
go mod tidy
```

3. (Optional) Embed the icon:
```batch
rsrc -ico assets/app.ico -manifest app.manifest -o rsrc.syso
```

4. Build the project:
```batch
go build -ldflags="-H windowsgui -s -w" -o singbox-launcher.exe
```

Build flags:
- `-H windowsgui` — hides the console window (GUI application)
- `-s` — strips the symbol table
- `-w` — strips debug information

## ⚠️ Troubleshooting

### Error: "go: command not found"
- Make sure Go is installed
- Check PATH: `echo %PATH%` must contain `C:\Program Files\Go\bin`
- Restart the command prompt after installing Go

### Error: "build constraints exclude all Go files"
- This is normal for some Fyne dependencies on Windows
- Make sure `CGO_ENABLED=1`
- Try `set CGO_ENABLED=1` before building

### Error: "rsrc: command not found"
- Not critical — the icon simply won't be embedded
- Install it: `go install github.com/akavel/rsrc@latest`
- Make sure `%USERPROFILE%\go\bin` is in PATH

### Error: "gcc: executable file not found in %PATH%"

**Problem:** CGO requires a C compiler (GCC), which is not part of the standard Go installation on Windows.

**Fix — install one of these:**

#### Option 1: TDM-GCC (recommended for beginners)

1. Download the installer from [https://jmeubank.github.io/tdm-gcc/](https://jmeubank.github.io/tdm-gcc/)
2. Run it and choose:
   - **Architecture**: x86_64 (64-bit)
   - **Installation directory**: `C:\TDM-GCC-64` (the default)
   - **Add to PATH**: ✅ tick the box
3. Restart the command prompt
4. Verify the installation:
   ```batch
   gcc --version
   ```

#### Option 2: MinGW-w64 (via MSYS2)

1. Download MSYS2 from [https://www.msys2.org/](https://www.msys2.org/)
2. Install MSYS2
3. Open **MSYS2 MSYS** (not MinGW64!)
4. Update the packages:
   ```bash
   pacman -Syu
   ```
5. Install MinGW-w64:
   ```bash
   pacman -S mingw-w64-x86_64-gcc
   ```
6. Add to PATH: `C:\msys64\mingw64\bin`
7. Restart the command prompt

#### Option 3: MinGW-w64 (direct install)

1. Download the installer from [https://www.mingw-w64.org/downloads/](https://www.mingw-w64.org/downloads/)
2. Or use [WinLibs](https://winlibs.com/) — prebuilt packages
3. Extract into `C:\mingw64`
4. Add `C:\mingw64\bin` to PATH
5. Restart the command prompt

**After installing:**
- Restart the command prompt (important!)
- Verify: `gcc --version`
- Try the build again: `build\build_windows.bat`

## 📦 Result

After a successful build, the project root contains:
- `singbox-launcher.exe` — the application executable

The file is typically 15–25 MB (depending on the Go version and build flags).

## 🚀 Running

Just launch `singbox-launcher.exe` by double-clicking it, or from the command line:

```batch
.\singbox-launcher.exe
```

## 📝 Notes

- The first build may take a few minutes (downloading dependencies)
- Subsequent builds are faster
- For debugging, drop the `-H windowsgui` flag to see console logs
