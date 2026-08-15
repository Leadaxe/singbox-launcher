# RDP / GPU-less server — window does not appear (OpenGL)

**🌐 Language**: English | [Русский](RDP_OPENGL.ru.md)

## Symptom

On Windows Server (or a VM/server without a GPU), typically over RDP:

- the process starts, the tray icon appears
- the main window never opens; tray "Open" does nothing
- reproduces in both `mstsc.exe` and Windows App (issue [#105](https://github.com/Leadaxe/singbox-launcher/issues/105))

## Cause

The launcher UI is built on Fyne (GLFW + OpenGL), which requires **OpenGL 2.1+**.
In an RDP session on a machine without a GPU (or without a driver that exposes
GL into the session), Windows only provides the "GDI Generic" software
implementation of **OpenGL 1.1** — the context cannot be created and the window
silently fails to render.

## Fix — automatic (since 1.4.2)

The launcher probes the OpenGL version before starting the UI. When hardware
OpenGL 2.1 is missing, a native dialog offers to download **Mesa3D (llvmpipe)**,
a software OpenGL renderer (~24 MB). On consent the DLLs are extracted next to
`singbox-launcher.exe` and the window opens immediately, no restart needed.

The download comes from this repository's
[`mesa3d-26.2.0`](https://github.com/Leadaxe/singbox-launcher/releases/tag/mesa3d-26.2.0)
release (a mirror of [mesa-dist-win](https://github.com/pal1000/mesa-dist-win)),
falling back to a ghproxy mirror when GitHub is unreachable.

## Fix — manual

If auto-install is not an option (no internet on the server), copy the files
yourself:

1. Download `mesa3d-26.2.0-win64.zip` from the
   [`mesa3d-26.2.0`](https://github.com/Leadaxe/singbox-launcher/releases/tag/mesa3d-26.2.0)
   release (or grab `x64/opengl32.dll`, `x64/libgallium_wgl.dll`, `x64/dxil.dll`
   from `mesa3d-<version>-release-msvc.7z` on
   [mesa-dist-win](https://github.com/pal1000/mesa-dist-win/releases)).
2. Extract all DLLs into the folder next to `singbox-launcher.exe`.
3. Start the launcher.

The Windows DLL search order picks the local `opengl32.dll` before the system
one — Fyne sees OpenGL 4.x (llvmpipe) and the window renders.

## Rollback / environment switches

- Return to hardware OpenGL: delete `opengl32.dll`, `libgallium_wgl.dll`,
  `dxil.dll` from the launcher folder.
- `SINGBOX_LAUNCHER_NO_MESA=1` — disable the probe and the Mesa offer entirely.
- `SINGBOX_LAUNCHER_FORCE_MESA=1` — force the Mesa offer even when hardware
  OpenGL is present (debugging aid).

Diagnostics: lines prefixed `gl:` in `logs/main.log` show the version and
renderer the probe saw.

## Performance

llvmpipe renders on the CPU. That is plenty for the launcher UI (<10 ms per
frame), but CPU usage with the window open is higher than with hardware GL.
Minimized to tray there is no difference.

## See also

- [WIN7_OPENGL.md](WIN7_OPENGL.md) — the same problem class on old Win7
  hardware (needs a 32-bit and older Mesa build — no auto-install there).
