# Инструкция по сборке на Windows

**🌐 Язык**: [English](BUILD_WINDOWS.md) | Русский

## 📋 Требования

1. **Go 1.25 или новее**
   - Скачайте с [https://go.dev/dl/](https://go.dev/dl/)
   - Установите в стандартную папку `C:\Program Files\Go`
   - Проверьте установку: `go version`

2. **Компилятор C (GCC) - ОБЯЗАТЕЛЬНО**
   - Fyne требует CGO, а CGO требует GCC
   - **Установите один из вариантов:**
     - [TDM-GCC](https://jmeubank.github.io/tdm-gcc/) - самый простой вариант
     - [MinGW-w64](https://www.mingw-w64.org/) - через MSYS2 или WinLibs
   - После установки добавьте `bin` папку GCC в PATH
   - **Важно:** Перезапустите командную строку после установки!
   - Проверьте: `gcc --version`

3. **CGO** (включен по умолчанию)
   - Fyne требует CGO для работы
   - Убедитесь, что CGO_ENABLED=1

4. **Опционально: rsrc** (для встраивания иконки)
   ```bash
   go install github.com/akavel/rsrc@latest
   ```
   После установки `rsrc.exe` будет в `%USERPROFILE%\go\bin\`

## 🔨 Сборка

### Вариант 1: Использование скрипта (рекомендуется)

1. Откройте командную строку (CMD) или PowerShell в папке проекта
2. Запустите скрипт сборки:

```batch
build\build_windows.bat
```

Или из корня проекта:

```batch
.\build\build_windows.bat
```

Скрипт автоматически:
- Обновит зависимости (`go mod tidy`)
- Встроит иконку (если установлен rsrc)
- Соберет проект
- Создаст `singbox-launcher.exe` в корне проекта

### Вариант 2: Ручная сборка

1. Откройте командную строку в папке проекта

2. Обновите зависимости:
```batch
go mod tidy
```

3. (Опционально) Встройте иконку:
```batch
rsrc -ico assets/app.ico -manifest app.manifest -o rsrc.syso
```

4. Соберите проект:
```batch
go build -ldflags="-H windowsgui -s -w" -o singbox-launcher.exe
```

Флаги сборки:
- `-H windowsgui` - скрывает консольное окно (GUI приложение)
- `-s` - убирает таблицу символов
- `-w` - убирает отладочную информацию

## ⚠️ Решение проблем

### Ошибка: "go: command not found"
- Убедитесь, что Go установлен
- Проверьте PATH: `echo %PATH%` должен содержать `C:\Program Files\Go\bin`
- Перезапустите командную строку после установки Go

### Ошибка: "build constraints exclude all Go files"
- Это нормально для некоторых зависимостей Fyne на Windows
- Убедитесь, что CGO_ENABLED=1
- Попробуйте: `set CGO_ENABLED=1` перед сборкой

### Ошибка: "rsrc: command not found"
- Это не критично, иконка просто не будет встроена
- Установите: `go install github.com/akavel/rsrc@latest`
- Убедитесь, что `%USERPROFILE%\go\bin` в PATH

### Ошибка: "gcc: executable file not found in %PATH%"

**Проблема:** CGO требует компилятор C (GCC), который не входит в стандартную установку Go на Windows.

**Решение - установите один из вариантов:**

#### Вариант 1: TDM-GCC (рекомендуется для начинающих)

1. Скачайте установщик с [https://jmeubank.github.io/tdm-gcc/](https://jmeubank.github.io/tdm-gcc/)
2. Запустите установщик и выберите:
   - **Architecture**: x86_64 (64-bit)
   - **Installation directory**: `C:\TDM-GCC-64` (по умолчанию)
   - **Add to PATH**: ✅ Отметьте галочку
3. Перезапустите командную строку
4. Проверьте установку:
   ```batch
   gcc --version
   ```

#### Вариант 2: MinGW-w64 (через MSYS2)

1. Скачайте MSYS2 с [https://www.msys2.org/](https://www.msys2.org/)
2. Установите MSYS2
3. Откройте **MSYS2 MSYS** (не MinGW64!)
4. Обновите пакеты:
   ```bash
   pacman -Syu
   ```
5. Установите MinGW-w64:
   ```bash
   pacman -S mingw-w64-x86_64-gcc
   ```
6. Добавьте в PATH: `C:\msys64\mingw64\bin`
7. Перезапустите командную строку

#### Вариант 3: MinGW-w64 (прямая установка)

1. Скачайте установщик с [https://www.mingw-w64.org/downloads/](https://www.mingw-w64.org/downloads/)
2. Или используйте [WinLibs](https://winlibs.com/) - готовые сборки
3. Распакуйте в `C:\mingw64`
4. Добавьте `C:\mingw64\bin` в PATH
5. Перезапустите командную строку

**После установки:**
- Перезапустите командную строку (важно!)
- Проверьте: `gcc --version`
- Попробуйте сборку снова: `build\build_windows.bat`

## 📦 Результат

После успешной сборки в корне проекта появится:
- `singbox-launcher.exe` - исполняемый файл приложения

Размер файла обычно составляет 15-25 МБ (зависит от версии Go и флагов сборки).

## 🚀 Запуск

Просто запустите `singbox-launcher.exe` двойным кликом или из командной строки:

```batch
.\singbox-launcher.exe
```

## 📝 Примечания

- Первая сборка может занять несколько минут (скачивание зависимостей)
- Последующие сборки будут быстрее
- Для отладки можно убрать флаг `-H windowsgui` чтобы видеть консольные логи

