# RDP / сервер без GPU — окно не появляется (OpenGL)

**🌐 Язык**: [English](RDP_OPENGL.md) | Русский

## Симптом

На Windows Server (или ВМ/сервере без GPU), обычно при работе через RDP:

- процесс запускается, иконка в трее появляется
- главное окно не открывается, «Открыть» из трея не помогает
- воспроизводится и в `mstsc.exe`, и в Windows App (issue [#105](https://github.com/Leadaxe/singbox-launcher/issues/105))

## Причина

UI лаунчера построен на Fyne (GLFW + OpenGL), которому нужен **OpenGL 2.1+**.
В RDP-сессии на машине без GPU (или без драйвера, отдающего GL в сессию)
Windows предоставляет только software-реализацию «GDI Generic» **OpenGL 1.1** —
контекст не создаётся, и окно молча не отрисовывается.

## Решение — автоматическое (с версии 1.4.2)

Лаунчер сам проверяет версию OpenGL до старта UI. Если аппаратного OpenGL 2.1
нет, появляется нативный диалог с предложением скачать **Mesa3D (llvmpipe)** —
software-рендерер OpenGL (~24 МБ). После согласия DLL распаковываются рядом с
`singbox-launcher.exe`, и окно открывается сразу, без перезапуска.

Скачивание идёт с релиза
[`mesa3d-26.2.0`](https://github.com/Leadaxe/singbox-launcher/releases/tag/mesa3d-26.2.0)
этого репозитория (зеркало [mesa-dist-win](https://github.com/pal1000/mesa-dist-win)),
при недоступности GitHub — через ghproxy-зеркало.

## Решение — вручную

Если автоустановка недоступна (нет интернета на сервере) — перенесите файлы сами:

1. Скачайте `mesa3d-26.2.0-win64.zip` из релиза
   [`mesa3d-26.2.0`](https://github.com/Leadaxe/singbox-launcher/releases/tag/mesa3d-26.2.0)
   (либо возьмите `x64/opengl32.dll`, `x64/libgallium_wgl.dll`, `x64/dxil.dll`
   из `mesa3d-<version>-release-msvc.7z` с
   [mesa-dist-win](https://github.com/pal1000/mesa-dist-win/releases)).
2. Распакуйте все DLL в папку рядом с `singbox-launcher.exe`.
3. Запустите лаунчер.

Порядок поиска DLL в Windows подхватит локальный `opengl32.dll` раньше
системного — Fyne увидит OpenGL 4.x (llvmpipe) и окно отрендерится.

## Откат / служебные переменные

- Вернуться на аппаратный OpenGL: удалить `opengl32.dll`, `libgallium_wgl.dll`,
  `dxil.dll` из папки лаунчера.
- `SINGBOX_LAUNCHER_NO_MESA=1` — полностью отключить проверку и предложение Mesa.
- `SINGBOX_LAUNCHER_FORCE_MESA=1` — принудительно предложить установку Mesa,
  даже если аппаратный OpenGL найден (для отладки).

Диагностика: строки с префиксом `gl:` в `logs/main.log` — там версия и
renderer, которые увидел пробник.

## Производительность

llvmpipe рендерит на CPU. Для UI лаунчера этого достаточно (кадр < 10 мс),
но CPU-нагрузка при открытом окне выше, чем с аппаратным GL. В трее
(окно скрыто) разницы нет.

## См. также

- [WIN7_OPENGL.ru.md](WIN7_OPENGL.ru.md) — та же проблема на старом железе
  Win7 (там нужна 32-битная Mesa и старая версия — автоустановки нет).
