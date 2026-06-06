@echo off
REM ============================================================
REM  Singbox Launcher - one-command ghost TUN cleanup (devcon)
REM ============================================================
REM
REM  REQUIREMENTS:
REM    1. devcon.exe in same folder as this .bat OR in PATH.
REM       Download: https://github.com/Drawbackz/DevCon-Installer/releases
REM    2. Run as Administrator (right-click -> Run as administrator).
REM
REM  WHAT THIS DOES:
REM    Lists all WinTun-based network adapters (which is what
REM    singbox-tun is) and offers to remove them all in one shot.
REM    The active adapter is removed too - launcher will
REM    re-create a clean one on next start.
REM
REM  SAFE FALLBACK:
REM    If you don't have devcon.exe or admin rights, use
REM    cleanup-singbox-tun.bat instead (GUI via Device Manager).
REM ============================================================

setlocal

echo.
echo ======================================================
echo  Singbox Launcher - ghost TUN cleanup (devcon)
echo ======================================================
echo.

REM Check devcon.exe availability
where devcon.exe >nul 2>&1
if errorlevel 1 (
    echo [ERROR] devcon.exe not found in PATH or current folder.
    echo.
    echo Download from:
    echo   https://github.com/Drawbackz/DevCon-Installer/releases
    echo.
    echo Place devcon.exe next to this .bat file, or in C:\Windows\System32
    echo and re-run this script.
    echo.
    pause
    exit /b 1
)

REM Check admin rights
net session >nul 2>&1
if errorlevel 1 (
    echo [ERROR] This script must be run as Administrator.
    echo.
    echo Right-click cleanup-singbox-tun-devcon.bat -^> Run as administrator
    echo.
    pause
    exit /b 1
)

echo Searching for singbox-tun adapters (including hidden ghosts)...
echo.
echo --- Currently registered ---
devcon.exe findall =net | findstr /i "singbox-tun"
if errorlevel 1 (
    echo [INFO] No singbox-tun adapters found. Nothing to clean up.
    echo.
    pause
    exit /b 0
)
echo.

REM Confirm before destructive action
set /p CONFIRM="Remove ALL WinTun adapters listed above? (y/N): "
if /i not "%CONFIRM%"=="y" (
    echo Cancelled by user.
    pause
    exit /b 0
)

echo.
echo Stop Singbox Launcher BEFORE continuing if it is still running.
set /p READY="Launcher is stopped, continue? (y/N): "
if /i not "%READY%"=="y" (
    echo Cancelled by user.
    pause
    exit /b 0
)

echo.
echo Removing all WinTun adapters...
devcon.exe remove =net *Wintun*
echo.

echo Done. Start Singbox Launcher again - it will create a clean
echo singbox-tun0 adapter automatically.
echo.
pause

endlocal
