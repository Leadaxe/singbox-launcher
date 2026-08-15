# Win7 — OpenGL 2.1 troubleshooting

**🌐 Language**: English | [Русский](WIN7_OPENGL.ru.md)

## Symptom

On Windows 7 (32-bit, legacy build `singbox-launcher-<version>-win7-32.zip`):

- launcher tray icon appears
- main window does not open, or opens as an empty frame (no widgets, no text)
- other Fyne-free apps (Throne, Happ, regular Win32 apps) work fine on the same machine

## Cause

The launcher UI is built on Fyne, which uses GLFW + OpenGL for rendering. **GLFW requires OpenGL 2.1 or newer.** On older hardware with integrated graphics (Intel HD Graphics 1xxx-2xxx, some ATI/AMD chipsets), Win7 ships with OpenGL 2.0 only — the GPU driver doesn't expose 2.1.

You can confirm the OpenGL version with [GPU-Z](https://www.techpowerup.com/gpuz/), [GLview](https://www.realtech-vr.com/home/glview), or any tool that reads `glGetString(GL_VERSION)`.

## Fix — drop in Mesa3D software OpenGL

[Mesa3D](https://www.mesa3d.org/) provides a software OpenGL implementation. The pre-built `mesa-dist-win` package ships ready-to-use DLLs that override the system OpenGL with a 3.x-capable software renderer, transparent to the application.

**Steps:**

1. Download the latest **32-bit (x86)** release of `mesa-dist-win`:

   <https://github.com/pal1000/mesa-dist-win/releases>

   Pick `mesa3d-<version>-release-msvc.7z` (or `.exe` self-extracting variant).

2. Extract the archive. Inside, navigate to the `x86/` subfolder.

3. Copy **`opengl32.dll`** and **`libglapi.dll`** from `x86/` into the same folder where `singbox-launcher-win7-32.exe` lives.

4. Restart the launcher.

The Windows DLL search order will pick up the local `opengl32.dll` before the system one, and Fyne / GLFW will see an OpenGL 3.x context — the window will render normally.

## Verification

- Tray icon → click → main window opens with the Local / Remote / Settings tabs.
- If it still doesn't render, double-check that both DLLs were copied from the `x86/` subfolder (not `x64/`) and sit next to the `.exe`, not in a subdirectory.

## Performance note

Mesa software rendering is slower than a hardware GPU driver. For a small launcher UI like this it's still smooth (<10 ms per frame on a typical Win7 machine), but expect higher CPU usage than the Windows 10/11 build with native OpenGL.

## Why isn't this bundled?

Mesa3D ships under a permissive license (MIT-style), so technically we could ship the DLLs in the Win7 release archive. We don't, because:

- It would inflate the Win7 release by ~5 MB just for legacy hardware.
- Most Win7 32-bit users have OpenGL 2.1+ available and don't need it.
- The launcher would have to detect missing OpenGL 2.1 at startup and lazy-extract — extra complexity for an edge case.

If demand grows, we may revisit and either bundle or auto-download Mesa on first failed startup.

Update: since 1.4.2 the **Win64** build does exactly that — probes OpenGL at startup and offers to auto-download Mesa (see [RDP_OPENGL.md](RDP_OPENGL.md), issue #105). The Win7 32-bit build still requires the manual steps above: modern Mesa builds need Windows 10+, so the auto-installed version would not run there. On Win7 the launcher shows a native dialog pointing to this guide instead.

## Credit

The case and the fix were reported by a user from the Telegram channel — thanks for the report.
