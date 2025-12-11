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
echo   Building Sing-Box Launcher (Windows)
echo ========================================
echo.

:: Устанавливаем окружение для сборки ПЕРЕД использованием go
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

echo === Tidying Go modules ===
go mod tidy
if %ERRORLEVEL% NEQ 0 (
    echo !!! Failed to tidy modules !!!
    if %NO_PAUSE%==0 pause
    exit /b %ERRORLEVEL%
)

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
        ) else (
            echo !!! GCC not found. CGO build may fail !!!
        )
    ) else (
        echo GCC found:
        gcc --version | findstr /C:"gcc"
    )
    echo.
)

:: === Embed icon into the executable ===
echo.
echo === Embedding icon from assets/app.ico ===
:: Проверяем наличие rsrc в PATH или в стандартной папке Go
where rsrc >nul 2>&1
if %ERRORLEVEL% EQU 0 (
    rsrc -ico assets/app.ico -manifest app.manifest -o rsrc.syso
    if %ERRORLEVEL% NEQ 0 (
        echo !!! Failed to embed icon. Skipping... !!!
    ) else (
        echo Icon embedded successfully.
    )
) else if exist "%USERPROFILE%\go\bin\rsrc.exe" (
    "%USERPROFILE%\go\bin\rsrc.exe" -ico assets/app.ico -manifest app.manifest -o rsrc.syso
    if %ERRORLEVEL% NEQ 0 (
        echo !!! Failed to embed icon. Skipping... !!!
    ) else (
        echo Icon embedded successfully.
    )
) else (
    echo rsrc.exe not found. Icon embedding skipped.
    echo To embed icons, install rsrc: go install github.com/akavel/rsrc@latest
    echo Then add %USERPROFILE%\go\bin to your PATH or restart command prompt.
)

:: Определяем имя выходного файла
echo.
echo === Determining output filename ===
set "BASE_NAME=singbox-launcher"
set "EXTENSION=.exe"
set "OUTPUT_FILENAME=%BASE_NAME%%EXTENSION%"
set "COUNTER=0"

:FIND_UNIQUE_FILENAME
if exist "%OUTPUT_FILENAME%" (
    set /a COUNTER+=1
    set "OUTPUT_FILENAME=%BASE_NAME%-!COUNTER!%EXTENSION%"
    goto :FIND_UNIQUE_FILENAME
)

echo Using output file: "%OUTPUT_FILENAME%"

:: Собираем проект
echo.
echo === Starting Build ===
echo Building with CGO_ENABLED=%CGO_ENABLED%
echo GOROOT=%GOROOT%
echo.
echo This may take a while on first build...
go build -v -buildvcs=false -ldflags="-H windowsgui -s -w" -o "%OUTPUT_FILENAME%"

if %ERRORLEVEL% NEQ 0 (
    echo.
    echo !!! Build failed !!!
    if %NO_PAUSE%==0 pause
    exit /b %ERRORLEVEL%
)

echo.
echo ========================================
echo   Build completed successfully!
echo   Output: %OUTPUT_FILENAME%
echo ========================================
if %NO_PAUSE%==0 pause

