# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

**Не добавлять** сюда мелкие правки **только UI** (порядок виджетов, выравнивание, стиль кнопок без смены действия и т.п.). Писать **новое поведение**: данные, форматы, сохранение, заметные для пользователя возможности.

## EN
### Highlights

- **Window now opens over RDP / on GPU-less servers** (issue #105). On Windows the launcher probes the OpenGL version before starting the UI; when hardware OpenGL 2.1 is missing (typical for RDP sessions on Windows Server), a native dialog offers to download the Mesa3D software renderer (~24 MB, mirrored in this repo's `mesa3d-26.2.0` release, ghproxy fallback). DLLs are extracted next to the exe and the window renders immediately, no restart. Opt-outs: `SINGBOX_LAUNCHER_NO_MESA=1`, force for debugging: `SINGBOX_LAUNCHER_FORCE_MESA=1`. See `docs/RDP_OPENGL.md`.

### Technical / Internal

- Internal `-gl-probe` flag: the launcher re-runs itself to probe WGL in a subprocess, so the parent process never loads the system `opengl32.dll` and can preload Mesa by full path (base-name reuse by the Windows loader). Win7 32-bit build shows a dialog pointing to the manual guide instead (modern Mesa needs Windows 10+).

## RU
### Основное

- **Окно теперь открывается по RDP / на серверах без GPU** (issue #105). На Windows лаунчер проверяет версию OpenGL до старта UI; если аппаратного OpenGL 2.1 нет (типично для RDP-сессий на Windows Server), нативный диалог предлагает скачать программный рендерер Mesa3D (~24 МБ, зеркало в релизе `mesa3d-26.2.0` этого репозитория, фолбэк через ghproxy). DLL распаковываются рядом с exe, окно отрисовывается сразу, без перезапуска. Отключение: `SINGBOX_LAUNCHER_NO_MESA=1`, принудительно для отладки: `SINGBOX_LAUNCHER_FORCE_MESA=1`. См. `docs/RDP_OPENGL.ru.md`.

### Техническое / Внутреннее

- Служебный флаг `-gl-probe`: лаунчер перезапускает сам себя для WGL-пробы в подпроцессе, чтобы родитель не грузил системный `opengl32.dll` и мог подгрузить Mesa по полному пути (переиспользование модуля по базовому имени загрузчиком Windows). Win7 32-bit сборка вместо автозагрузки показывает диалог со ссылкой на ручную инструкцию (современной Mesa нужна Windows 10+).
