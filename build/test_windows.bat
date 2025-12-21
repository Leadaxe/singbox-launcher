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
echo CGO_ENABLED=%CGO_ENABLED%
echo GOROOT=%GOROOT%
echo Test package: %TEST_PACKAGE%
echo Test flags: %TEST_FLAGS%
echo.

echo This may take a while...
go test %TEST_FLAGS% %TEST_PACKAGE%

set TEST_EXIT_CODE=%ERRORLEVEL%

echo.
echo ========================================
if %TEST_EXIT_CODE% EQU 0 (
    echo   All tests passed!
) else (
    echo   Some tests failed (exit code: %TEST_EXIT_CODE%)
)
echo ========================================

if %NO_PAUSE%==0 pause
exit /b %TEST_EXIT_CODE%

