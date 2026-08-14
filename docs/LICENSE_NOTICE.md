# Third-party license notices

**🌐 Language**: English | [Русский](LICENSE_NOTICE.ru.md)

## WinTun.dll

This project may include `wintun.dll` — a library for creating virtual network adapters on Windows.

**Source:** [https://www.wintun.net/](https://www.wintun.net/)  
**License:** MIT License  
**Copyright:** Copyright (c) 2018-2021 WireGuard LLC. All Rights Reserved.

WinTun.dll is distributed under the MIT license, which permits:
- ✅ Free use
- ✅ Modification
- ✅ Redistribution (including inside the project's releases)

**Official repository:** [https://git.zx2c4.com/wintun/](https://git.zx2c4.com/wintun/)

## sing-box (fork: sing-box-lx)

**Source (the primary core):** [https://github.com/Leadaxe/sing-box-lx](https://github.com/Leadaxe/sing-box-lx)  
**Upstream:** [https://github.com/SagerNet/sing-box](https://github.com/SagerNet/sing-box)  
**License:** GPL-3.0 (the fork inherits the upstream license)  

### Included third-party binaries

This release downloads/bundles a prebuilt `sing-box.exe` (Windows) / `sing-box` (macOS/Linux) from the **sing-box-lx** fork (upstream sing-box + the `with_xhttp` and `with_awg` build tags, at the version in `constants.RequiredCoreVersion` — currently `1.14.0-lx.26`):

**Core repository:** [https://github.com/Leadaxe/sing-box-lx](https://github.com/Leadaxe/sing-box-lx)  
**License:** GPL-3.0

**Windows 7 (`GOOS=windows GOARCH=386`):** the fork builds this platform too (the `windows-386-legacy-windows-7` asset) — also `sing-box-lx`, with no separate upstream exception anymore.

**Note:** if a release ships without a prebuilt binary, users must download it themselves from the [sing-box-lx releases](https://github.com/Leadaxe/sing-box-lx/releases).

## Fyne

**Source:** [https://github.com/fyne-io/fyne](https://github.com/fyne-io/fyne)  
**License:** BSD-3-Clause

---

**Note:** this project (`singbox-launcher`) is distributed under the **GNU General Public License v3.0**. Full text — [LICENSE](../LICENSE); commercial and dual licensing — [LICENSING.md](../LICENSING.md).
