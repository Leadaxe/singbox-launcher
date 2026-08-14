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

   **Fedora / RHEL:**
   ```bash
   sudo dnf install -y \
     mesa-libGL-devel libXcursor-devel libXrandr-devel libXi-devel \
     libXinerama-devel libXft-devel libxkbcommon-x11-devel \
     libXxf86vm-devel libwayland-devel
   ```

3. **CGO** — должен быть включён (по умолчанию `CGO_ENABLED=1`).

## Сборка

### Вариант 1: Скрипт (рекомендуется)

Скрипт проверяет наличие зависимостей и выводит команды установки при их отсутствии.

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
GOOS=linux GOARCH=amd64 go build -buildvcs=false -ldflags="-s -w" -o singbox-launcher
```

## Решение проблем

### Package gl was not found / pkg-config

- Установите `pkg-config` и пакеты OpenGL: на Debian/Ubuntu — `libgl1-mesa-dev`, см. блок «Системные пакеты» выше.

### X11/Xcursor/Xcursor.h: No such file or directory

- Не хватает заголовков X11. На Debian/Ubuntu: `libxcursor-dev` и остальные пакеты из списка выше (libxrandr-dev, libxi-dev и т.д.).

### Сборка в Docker: COPY failed / no such file

- Запускайте `docker build` **из корня репозитория** (где лежат `go.mod` и `go.sum`), с контекстом `.` и `-f build/Dockerfile.linux`.

## Запуск

```bash
./singbox-launcher
```

Если `sing-box` установлен из пакета дистрибутива и доступен в `PATH`, лаунчер использует его; иначе положите бинарник в `bin/sing-box` рядом с лаунчером или скачайте через вкладку **Локально**.

При необходимости настройки TUN см. основной README (раздел про Linux capabilities и `setcap`).
