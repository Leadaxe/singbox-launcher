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

:: Устанавливаем окружение для сборки ПЕРВЫМ делом (локально), но НЕ в CI
echo === Setting PATH and environment ===
if defined GITHUB_ACTIONS (
    echo === CI detected ^(GITHUB_ACTIONS=true^). Skipping GOROOT/PATH override ===
) else (
    rem --- Pick GOROOT (local only) ---
    if exist "C:\Program Files\Go" (
        set "GOROOT=C:\Program Files\Go"
    ) else if exist "C:\Go" (
        set "GOROOT=C:\Go"
    )

    rem --- Prepend toolchains to PATH safely inside parentheses ---
    rem Use CALL SET with %%PATH%% to avoid parsing issues with parentheses in PATH
    call set "PATH=C:\Program Files\Go\bin;%%PATH%%"
    call set "PATH=C:\msys64\mingw64\bin;%%PATH%%"

    if exist "%LOCALAPPDATA%\Programs\Git\cmd" (
        call set "PATH=%LOCALAPPDATA%\Programs\Git\cmd;%%PATH%%"
    ) else if exist "%LOCALAPPDATA%\Programs\Git\bin" (
        call set "PATH=%LOCALAPPDATA%\Programs\Git\bin;%%PATH%%"
    ) else if exist "C:\Program Files\Git\cmd" (
        call set "PATH=C:\Program Files\Git\cmd;%%PATH%%"
    ) else if exist "C:\Program Files\Git\bin" (
        call set "PATH=C:\Program Files\Git\bin;%%PATH%%"
    ) else if exist "C:\Program Files (x86)\Git\cmd" (
        call set "PATH=C:\Program Files (x86)\Git\cmd;%%PATH%%"
    ) else if exist "C:\Program Files (x86)\Git\bin" (
        call set "PATH=C:\Program Files (x86)\Git\bin;%%PATH%%"
    )

    call set "PATH=%USERPROFILE%\go\bin;%%PATH%%"

    echo Updated PATH and GOROOT for local build
)

if defined GOROOT (
    echo GOROOT=%GOROOT%
) else (
    echo GOROOT is not set ^(OK in CI with actions/setup-go^)
)
echo.

:: Проверяем, что Go доступен (до любых команд go)
where go >nul 2>&1
if %ERRORLEVEL% NEQ 0 (
    echo !!! Go not found in PATH !!!
    if %NO_PAUSE%==0 pause
    exit /b 1
) else (
    echo Go found
    set "GO_VER="
    for /f "delims=" %%v in ('go version 2^>nul') do set "GO_VER=%%v"
    if defined GO_VER (
        echo !GO_VER!
    ) else (
        echo Warning: failed to run "go version"
    )
)
echo.

:: === Go modules tidy (skip in CI) ===
if defined GITHUB_ACTIONS (
    echo === Skipping "go mod tidy" in CI ^(GITHUB_ACTIONS=true^) ===
) else (
    echo === Tidying Go modules ===
    echo Note: go mod tidy may modify go.mod/go.sum files.
    go mod tidy
    if %ERRORLEVEL% NEQ 0 (
        echo !!! Failed to tidy modules !!!
        if %NO_PAUSE%==0 pause
        exit /b %ERRORLEVEL%
    )
)
echo.

set CGO_ENABLED=1
set GOOS=windows
set GOARCH=amd64

:: Проверяем наличие gcc для CGO
if %CGO_ENABLED%==1 (
    where gcc >nul 2>&1
    if !ERRORLEVEL! NEQ 0 (
        echo !!! WARNING: GCC not found in PATH !!!
        echo CGO requires GCC compiler. Checking common locations...
        if exist "C:\msys64\mingw64\bin\gcc.exe" (
            echo Found GCC at C:\msys64\mingw64\bin\gcc.exe
        ) else (
            echo !!! GCC not found. CGO build may fail !!!
        )
    ) else (
        echo GCC found:
        gcc --version 2>nul | findstr /I /C:"gcc"
    )
    echo.
)

:: === Getting version ===
echo.
if defined APP_VERSION (
    set "VERSION=%APP_VERSION%"
    echo Version ^(from APP_VERSION^): !VERSION!
) else (
    for /f "delims=" %%v in ('git describe --tags --always --dirty 2^>nul') do set "VERSION=%%v"
    echo Version ^(from git describe^): !VERSION!
)
if not defined VERSION (
    set "VERSION=unnamed-dev"
    echo Version default: !VERSION!
)

:: Determine GOPATH (for tools like rsrc)
for /f "delims=" %%g in ('go env GOPATH 2^>nul') do set "GOPATH=%%g"

:: Clean old resource object to avoid stale embedding
if exist rsrc.syso del /q rsrc.syso

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
) else if defined GOPATH if exist "%GOPATH%\bin\rsrc.exe" (
    "%GOPATH%\bin\rsrc.exe" -ico assets/app.ico -manifest app.manifest -o rsrc.syso
    if %ERRORLEVEL% NEQ 0 (
        echo !!! Failed to embed icon. Skipping... !!!
    ) else (
        echo Icon embedded successfully.
    )
) else (
    echo rsrc.exe not found. Icon embedding skipped.
    echo To embed icons, install rsrc: go install github.com/akavel/rsrc@latest
    if not defined GITHUB_ACTIONS (
        if defined GOPATH (
            echo Then add !GOPATH!\bin to your PATH or restart command prompt.
        ) else (
            echo Then add ^<GOPATH^>\bin to your PATH or restart command prompt.
            echo You can check GOPATH with: go env GOPATH
        )
    )
)

:: Post-check: did we actually produce rsrc.syso?
if exist rsrc.syso (
    echo === Resource file rsrc.syso is present ^(resources will be embedded^) ===
) else (
    echo === Resource file rsrc.syso is NOT present ^(building without embedded icon/manifest^) ===
)

:: Определяем имя выходного файла
echo.
echo === Determining output filename ===
set "OUTPUT_FILENAME=singbox-launcher.exe"

:: Удаляем старый файл, если существует
if exist "%OUTPUT_FILENAME%" (
    echo Removing old file: %OUTPUT_FILENAME%
    del /q "%OUTPUT_FILENAME%"
)

echo Using output file: "%OUTPUT_FILENAME%"

:: Формируем ldflags с версией
set "LDFLAGS=-H windowsgui -s -w -X singbox-launcher/internal/constants.AppVersion=!VERSION!"

:: Уровень шумности
set "BUILD_VERBOSE=-v"
if defined GITHUB_ACTIONS set "BUILD_VERBOSE="

:: Собираем проект
echo.
echo === Starting Build ===
echo Building with CGO_ENABLED=%CGO_ENABLED%
echo.
echo This may take a while on first build...
go build %BUILD_VERBOSE% -buildvcs=false -ldflags="%LDFLAGS%" -o "%OUTPUT_FILENAME%"

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

