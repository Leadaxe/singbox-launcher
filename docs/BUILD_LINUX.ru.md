# Сборка на Linux

**🌐 Язык**: [English](BUILD_LINUX.md) | Русский

## Требования

1. **Go 1.25** (или версия из `go.mod`)
   - Установка: [https://go.dev/dl/](https://go.dev/dl/) или пакет дистрибутива
   - Проверка: `go version`

2. **Системные пакеты для CGO и Fyne (OpenGL + X11/GLFW)**

   Без них сборка падает с ошибками вроде `Package gl was not found` или `X11/Xcursor/Xcursor.h: No such file or directory`.

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

   **RHEL 9 и совместимые дистрибутивы:**

   Пакеты `libxkbcommon-x11-devel` и `libXxf86vm-devel` находятся в репозитории CodeReady Builder (CRB). Сначала включите его, затем установите приведённый выше набор пакетов Fedora.

   В RHEL с активной подпиской:
   ```bash
   sudo subscription-manager repos \
     --enable="codeready-builder-for-rhel-9-$(arch)-rpms"
   ```

   В Rocky Linux 9 или AlmaLinux 9:
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

3. **CGO** — должен быть включён (по умолчанию `CGO_ENABLED=1`).

## Сборка

### Вариант 1: Скрипт (рекомендуется)

Скрипт проверяет наличие зависимостей и выводит команды установки при их отсутствии. При наличии соответствующих файлов разработки он включает X11 и нативный Wayland; иначе собирает X11-бэкенд, который также работает в Wayland-сессиях через XWayland.

```bash
cd /path/to/singbox-launcher
./build/build_linux.sh
```

Результат: бинарник `singbox-launcher` (или `singbox-launcher-1`, …) в корне репозитория.

### Вариант 2: Сборка в Docker

Если не хотите ставить системные пакеты, можно собрать в контейнере. Запуск **из корня репозитория**:

```bash
docker build -f build/Dockerfile.linux --target export -o type=local,dest=. .
chmod +x singbox-launcher
```

Бинарник появится в текущей папке.

### Вариант 3: Ручная сборка

После установки зависимостей:

```bash
export CGO_ENABLED=1
export CGO_CFLAGS="$(pkg-config --cflags-only-I wayland-client wayland-cursor wayland-egl xkbcommon)"
GOOS=linux GOARCH=amd64 go build -buildvcs=false -ldflags="-s -w" -o singbox-launcher
```

## Решение проблем

### Package gl was not found / pkg-config

- Установите `pkg-config` и пакеты OpenGL: на Debian/Ubuntu — `libgl1-mesa-dev`, на openSUSE — `Mesa-libGL-devel`. См. блок «Системные пакеты» выше.

### X11/Xcursor/Xcursor.h: No such file or directory

- Не хватает заголовков X11. На Debian/Ubuntu — `libxcursor-dev`; на openSUSE — `libXcursor-devel` и остальные пакеты из списка выше (`libXrandr-devel`, `libXi-devel` и т.д.).

### wayland-client-core.h: No such file or directory

- GLFW включает Wayland-бэкенд на Linux. В openSUSE заголовки Wayland находятся в `/usr/include/wayland`; `build_linux.sh` автоматически добавляет этот путь через `pkg-config`. При ручной сборке используйте команду `CGO_CFLAGS`, приведённую выше.
- Проверить настройку можно командой `pkg-config --cflags wayland-client`: в openSUSE вывод должен содержать `-I/usr/include/wayland`.

### EGL/egl.h: No such file or directory

- Заголовки EGL нужны для нативного Wayland-бэкенда GLFW. Они устанавливаются как транзитивная зависимость наборов пакетов, проверенных в Debian 12, Ubuntu 24.04, Fedora 44 и Rocky Linux 9. Если в другом выпуске заголовков нет, установите `libegl1-mesa-dev` в Debian/Ubuntu, `mesa-libEGL-devel` в Fedora/RHEL или `Mesa-libEGL-devel` в openSUSE. При их отсутствии `build_linux.sh` автоматически использует X11-бэкенд.

### Сборка в Docker: COPY failed / no such file

- Запускайте `docker build` **из корня репозитория** (где лежат `go.mod` и `go.sum`), с контекстом `.` и `-f build/Dockerfile.linux`.

### Пароль спрашивают трижды при старте VPN (и ещё раз при остановке)

- Это не проблема сборки — см. [TROUBLESHOOTING.ru.md](TROUBLESHOOTING.ru.md#linux).

## Запуск

```bash
./singbox-launcher
```

Если `sing-box` установлен из пакета дистрибутива и доступен в `PATH`, лаунчер использует его; иначе положите бинарник в `bin/sing-box` рядом с лаунчером или скачайте через вкладку **Локально**.

При необходимости настройки TUN см. основной README (раздел про Linux capabilities и `setcap`).
