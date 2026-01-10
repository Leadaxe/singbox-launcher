@echo off
setlocal enabledelayedexpansion

:: Проверяем параметр для пропуска ожидания
set "NO_PAUSE=0"
if "%1"=="nopause" set "NO_PAUSE=1"
if "%1"=="silent" set "NO_PAUSE=1"
if "%1"=="/nopause" set "NO_PAUSE=1"
if "%1"=="-nopause" set "NO_PAUSE=1"

cd /d "%~dp0\.."

echo.
echo ========================================
echo   Running Tests for Sing-Box Launcher
echo ========================================
echo.

:: ========================================
:: Очистка временных директорий В НАЧАЛЕ
:: ========================================
echo === Cleaning temporary directories ===

:: Очищаем директорию проекта temp\windows
set "TEST_OUTPUT_DIR=temp\windows"
if exist "%TEST_OUTPUT_DIR%" (
    echo Removing project temp directory: %TEST_OUTPUT_DIR%...
    rmdir /s /q "%TEST_OUTPUT_DIR%" 2>nul
)

:: Создаем чистую директорию для тестов
mkdir "%TEST_OUTPUT_DIR%" 2>nul
mkdir "%TEST_OUTPUT_DIR%\tmp" 2>nul
mkdir "%TEST_OUTPUT_DIR%\cache" 2>nul
echo Test output directory: %CD%\%TEST_OUTPUT_DIR%
echo Cleanup completed. You can inspect %TEST_OUTPUT_DIR% after tests run.
echo.

:: Устанавливаем окружение для тестов ПЕРЕД использованием go
echo === Setting PATH and environment ===
:: Устанавливаем GOROOT явно на правильную установку Go
if exist "C:\Program Files\Go" (
    set "GOROOT=C:\Program Files\Go"
) else if exist "C:\Go" (
    set "GOROOT=C:\Go"
)
:: Добавляем пути к Go, MinGW и Git в начало PATH (Go должен быть ПЕРВЫМ!)
set "PATH=C:\Program Files\Go\bin;%PATH%"
set "PATH=C:\msys64\mingw64\bin;%PATH%"
if exist "%LOCALAPPDATA%\Programs\Git\bin" (
    set "PATH=%LOCALAPPDATA%\Programs\Git\bin;%PATH%"
) else if exist "C:\Program Files\Git\bin" (
    set "PATH=C:\Program Files\Git\bin;%PATH%"
) else if exist "C:\Program Files (x86)\Git\bin" (
    set "PATH=C:\Program Files (x86)\Git\bin;%PATH%"
)
set "PATH=%USERPROFILE%\go\bin;%PATH%"

:: Проверяем, что Go доступен
where go >nul 2>&1
if %ERRORLEVEL% NEQ 0 (
    echo !!! Go not found in PATH !!!
    if %NO_PAUSE%==0 pause
    exit /b 1
)

echo GOROOT=%GOROOT%
echo.

:: Устанавливаем CGO_ENABLED=1 для тестов (требуется для Fyne)
set CGO_ENABLED=1
set GOOS=windows
set GOARCH=amd64

:: Устанавливаем временную директорию для Go в папку проекта
set "GOTMPDIR=%CD%\%TEST_OUTPUT_DIR%\tmp"
set "GOCACHE=%CD%\%TEST_OUTPUT_DIR%\cache"

:: Проверяем наличие gcc для CGO
if %CGO_ENABLED%==1 (
    where gcc >nul 2>&1
    if %ERRORLEVEL% NEQ 0 (
        echo !!! WARNING: GCC not found in PATH !!!
        echo CGO requires GCC compiler. Checking common locations...
        if exist "C:\msys64\mingw64\bin\gcc.exe" (
            echo Found GCC at C:\msys64\mingw64\bin\gcc.exe
            set "PATH=C:\msys64\mingw64\bin;%PATH%"
        ) else (
            echo !!! GCC not found. CGO tests may fail !!!
            echo Please install MinGW-w64 or TDM-GCC for CGO support.
        )
    ) else (
        echo GCC found:
        gcc --version | findstr /C:"gcc"
    )
    echo.
)

:: Проверяем параметры запуска
set "TEST_PACKAGE=./..."
set "TEST_FLAGS=-v"
set "TEST_RUN="

if not "%2"=="" (
    set "TEST_PACKAGE=%2"
)

if "%1"=="short" (
    set "TEST_FLAGS=-v -short"
    shift
    if not "%2"=="" (
        set "TEST_PACKAGE=%2"
    )
)

if "%1"=="run" (
    if "%2"=="" (
        echo !!! Error: 'run' requires test name pattern !!!
        echo Usage: test_windows.bat run TestName
        if %NO_PAUSE%==0 pause
        exit /b 1
    )
    set "TEST_FLAGS=-v -run %2"
    if not "%3"=="" (
        set "TEST_PACKAGE=%3"
    ) else (
        set "TEST_PACKAGE=./..."
    )
)

:: Запускаем тесты
echo.
echo ========================================
echo   Running Tests
echo ========================================
echo.

:: Показываем список пакетов, которые будут тестироваться
echo === Packages to test ===
for /f "delims=" %%p in ('go list %TEST_PACKAGE% 2^>nul') do (
    echo   - %%p
)
echo.

echo CGO_ENABLED=%CGO_ENABLED%
echo GOROOT=%GOROOT%
echo GOTMPDIR=%GOTMPDIR%
echo GOCACHE=%GOCACHE%
echo Test package: %TEST_PACKAGE%
echo Test flags: %TEST_FLAGS%
echo Test binaries will be saved to: %CD%\%TEST_OUTPUT_DIR%
echo.

:: Показываем время начала и создаем лог-файл
set "TEST_LOG=%CD%\%TEST_OUTPUT_DIR%\test_output.log"
echo Test started at: %DATE% %TIME%
echo Test log will be saved to: %TEST_LOG%
echo.
echo Starting tests...
echo.

:: Запускаем тесты с выводом в файл и на экран одновременно
:: Compute package list excluding UI packages and fyne imports
set "PKGS="
for /f "delims=" %%p in ('go list %TEST_PACKAGE% 2^>nul ^| findstr /V /C:"/ui/" /C:"fyne.io"') do (
    if defined PKGS (
        set "PKGS=!PKGS! %%p"
    ) else (
        set "PKGS=%%p"
    )
)

if not defined PKGS (
    echo No packages to test after filtering. Exiting.
    exit /b 0
)

echo Packages to be tested:
echo %PKGS%

:: Run tests with output to file and on screen simultaneously
go test %TEST_FLAGS% -count=1 %PKGS% > "%TEST_LOG%" 2>&1
set TEST_EXIT_CODE=%ERRORLEVEL%
set TEST_EXIT_CODE=%ERRORLEVEL%

:: Показываем содержимое лога на экране
type "%TEST_LOG%"

:: Показываем время окончания
echo.
echo ========================================
echo Test finished at: %DATE% %TIME%
echo Full test log: %TEST_LOG%
echo ========================================

:: После тестов компилируем бинарники для сохранения и проверки
echo.
echo === Compiling test binaries for inspection ===
for /f "delims=" %%p in ('go list %TEST_PACKAGE% 2^>nul') do (
    set "PKG=%%p"
    :: Преобразуем путь пакета в имя файла
    set "PKG_NAME=%%p"
    set "PKG_NAME=!PKG_NAME:singbox-launcher/=!"
    set "PKG_NAME=!PKG_NAME:\=_!"
    set "PKG_NAME=!PKG_NAME:/=_!"
    set "PKG_NAME=!PKG_NAME: =_!"
    if "!PKG_NAME!"=="" set "PKG_NAME=main"
    set "OUTPUT_FILE=%TEST_OUTPUT_DIR%\!PKG_NAME!.test.exe"
    echo Compiling %%p...
    go test -c -o "!OUTPUT_FILE!" %%p >nul 2>&1
    if !ERRORLEVEL! EQU 0 (
        echo   Saved: !OUTPUT_FILE!
    ) else (
        echo   Failed to compile: %%p
    )
)
echo.

echo.
echo ========================================
if %TEST_EXIT_CODE% EQU 0 (
    echo   All tests passed!
) else (
    echo   Some tests failed (exit code: %TEST_EXIT_CODE%)
)
echo ========================================
echo.
echo Test binaries saved to: %TEST_OUTPUT_DIR%
echo You can inspect them manually before next run.
echo.

if %NO_PAUSE%==0 pause
exit /b %TEST_EXIT_CODE%

