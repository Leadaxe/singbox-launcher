# Уведомления о лицензиях сторонних компонентов

## WinTun.dll

Этот проект может включать `wintun.dll` - библиотеку для создания виртуальных сетевых адаптеров в Windows.

**Источник:** [https://www.wintun.net/](https://www.wintun.net/)  
**Лицензия:** MIT License  
**Авторское право:** Copyright (c) 2018-2021 WireGuard LLC. All Rights Reserved.

WinTun.dll распространяется под лицензией MIT, что позволяет:
- ✅ Свободно использовать
- ✅ Модифицировать
- ✅ Распространять (включая в релизы проекта)

**Официальный репозиторий:** [https://git.zx2c4.com/wintun/](https://git.zx2c4.com/wintun/)

## sing-box (форк: sing-box-lx)

**Источник (основное ядро):** [https://github.com/Leadaxe/sing-box-lx](https://github.com/Leadaxe/sing-box-lx)  
**Апстрим:** [https://github.com/SagerNet/sing-box](https://github.com/SagerNet/sing-box)  
**Лицензия:** GPL-3.0 (форк наследует лицензию апстрима)  

### Included third-party binaries

This release downloads/bundles a prebuilt `sing-box.exe` (Windows) / `sing-box` (macOS/Linux) from the **sing-box-lx** fork (upstream sing-box + сборочные теги `with_xhttp` и `with_awg`, версия `1.13.13-lx.3`):

**Репозиторий ядра:** [https://github.com/Leadaxe/sing-box-lx](https://github.com/Leadaxe/sing-box-lx)  
**Лицензия:** GPL-3.0

**Windows 7 (`GOOS=windows GOARCH=386`):** форк собирает и эту платформу (ассет `windows-386-legacy-windows-7`) — тоже `sing-box-lx`, отдельного апстрим-исключения больше нет.

**Примечание:** Если в релизе нет предсобранного бинарника, пользователи должны скачать его самостоятельно из [релизов sing-box-lx](https://github.com/Leadaxe/sing-box-lx/releases).

## Fyne

**Источник:** [https://github.com/fyne-io/fyne](https://github.com/fyne-io/fyne)  
**Лицензия:** BSD-3-Clause

---

**Примечание:** Этот проект (`singbox-launcher`) распространяется под **GNU General Public License v3.0**. Полный текст — [LICENSE](../LICENSE); коммерческая лицензия и dual licensing — [LICENSING.md](../LICENSING.md).

