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

## Fix — automatic (since 1.4.2, reworked in 1.5.6)

Before starting the UI the launcher runs an **OpenGL gate**. Its single rule:
*if the previous start reached the first frame, do not probe; otherwise probe.*
The gate keeps that fact in `bin/gl-state.json` — `phase: starting` is written
before GL initialisation, `phase: rendered` a few seconds after the window is
alive. Deleting that file is a full reset of the gate's decisions.

Mesa3D is **never installed without an explicit "Yes"** — not even in the
`win64-full` archive, which ships the DLLs in `mesa3d/` next to the exe.
Depending on what the probe sees, the gate shows one of these native dialogs
(GDI, so they work without OpenGL):

| Situation | Dialog | Effect of the answer |
|---|---|---|
| The probe did not finish within 10 s | Retry / Ignore / Abort | Retry runs the check again; Ignore starts on the system OpenGL untouched; Abort treats the machine as having no hardware OpenGL |
| No hardware OpenGL 2.1, no Mesa next to the exe | Yes / No | Yes installs Mesa3D (from `mesa3d/`, or ~24 MB download when that folder is absent) |
| Mesa is in use, the previous start died, hardware OpenGL works | Yes / No | Yes renames the Mesa DLLs to `.off` and starts on the GPU |
| The previous start died under Mesa and hardware OpenGL does not work either | OK | Informational; the launcher starts anyway and the window may stay blank |
| Mesa was installed but failed its verification probe | OK | Mesa is disabled again, the launcher starts on the system OpenGL |

On a machine that has no hardware OpenGL and a working Mesa next to the exe —
the ordinary RDP / GPU-less case Mesa exists for — the gate shows nothing at
all: it notes in the log that hardware OpenGL is unavailable, keeps Mesa and
starts. The "neither works" dialog appears only when a start under Mesa has
actually failed to reach a frame.

A timeout is **not** read as "no OpenGL any more": a probe that hangs is usually
a temporary driver hiccup, and treating it as a verdict was what installed Mesa
on machines with a perfectly good GPU.

### Why switching the renderer restarts the launcher

`OPENGL32.dll` sits in the exe's **import table**, so Windows maps it while the
process is being created — before a single line of the launcher's own code
runs. The renderer therefore cannot be changed inside a live process: renaming
the DLLs takes effect only for the *next* process. Whenever the gate or the
Diagnostics button switches the renderer, the launcher writes
`phase: restart` into `bin/gl-state.json`, says so in a dialog and restarts
itself. The new process starts straight into the chosen renderer without
probing again.

The same import is why the hardware probe has to step around Mesa: with Mesa
next to the exe, the `-gl-probe` child process would load Mesa too and report
it as if it were the GPU (the field report showed `D3D12 (NVIDIA GeForce
GT 440)` instead of the real `GeForce GT 440/PCIe/SSE2`). Before running the
probe the launcher therefore renames the local `opengl32.dll` to
`opengl32.dll.probe` for the duration of the child process and puts it back
afterwards — Windows allows renaming a mapped DLL, so the running Mesa is
unaffected. As a second line of defence the probe reads `GL_VENDOR`: a
"hardware" result whose vendor is Mesa is discarded rather than believed.

With `-tray` (start minimised) the gate shows **no dialogs at all** and changes
no files — every decision goes to the log instead.

### Verification and the pinned driver

The Mesa asset ships every Gallium driver, and on a machine with a GPU Mesa
picks `d3d12` by default — i.e. the very GPU stack that just failed. The
launcher therefore sets `GALLIUM_DRIVER=llvmpipe` before loading Mesa (only if
you have not set that variable yourself), and verifies the freshly installed
DLLs with a second probe that loads `opengl32.dll` from the launcher folder by
full path. The install is accepted only when that probe reports an `llvmpipe`
renderer; otherwise everything is rolled back automatically.

### Switching later

Every start that renders through Mesa writes a `WARN` line saying so. If
hardware OpenGL appears later, the launcher offers once per renderer to switch
back (answer "Later" and it stays quiet until the renderer string changes).

The **Diagnostics** tab has a single button that toggles the renderer while the
UI is up: *Disable Mesa3D (use hardware OpenGL)* or *Enable Mesa3D (software
rendering)*, depending on what is next to the exe. Confirming it shuts the
launcher down cleanly (the core is stopped first) and starts it again, because
the renderer can only change with the process. The button is hidden when there
is nothing to toggle.

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

- Preferred: the **Diagnostics** tab button, or answering "Yes" to the gate's
  offer to switch back. Both rename the Mesa DLLs to `opengl32.dll.off`,
  `libgallium_wgl.dll.off`, `dxil.dll.off` — renaming them back re-enables
  Mesa without any download.
- Manual: delete or rename those DLLs in the launcher folder. The `mesa3d/`
  folder is never touched by any of this.
- Delete `bin/gl-state.json` to make the gate forget every decision (including
  which renderer you already declined) and re-probe on the next start.
- `SINGBOX_LAUNCHER_NO_MESA=1` — skip the gate entirely.
- `GALLIUM_DRIVER=<driver>` — override the pinned `llvmpipe` (for example
  `zink` or `d3d12`). The launcher only sets the variable when it is empty.

Diagnostics:

- lines prefixed `gl:` in `logs/singbox-launcher.log` show what the probe saw
  and every decision the gate made;
- `logs/native-stderr.log` collects the output of the native code itself —
  Mesa, GPU drivers, GLFW. The Windows build is linked with `-H windowsgui`, so
  the process has no stderr and that output used to vanish without a trace;
  the launcher now redirects the handle to this file. `MESA_DEBUG=1` makes Mesa
  verbose in it.

## Performance

llvmpipe renders on the CPU. That is plenty for the launcher UI (<10 ms per
frame), but CPU usage with the window open is higher than with hardware GL.
Minimized to tray there is no difference.

## See also

- [WIN7_OPENGL.md](WIN7_OPENGL.md) — the same problem class on old Win7
  hardware (needs a 32-bit and older Mesa build — no auto-install there).
