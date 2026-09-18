@echo off
setlocal
rem ---------------------------------------------------------------------------
rem  singbox-launcher - create a Task Scheduler entry that starts the launcher
rem  at logon with the highest privileges.
rem
rem  The launcher's manifest requires administrator rights, so a shortcut in the
rem  Startup folder triggers a UAC prompt on every logon. A scheduled task with
rem  /RL HIGHEST starts it silently, and before the regular startup apps.
rem
rem  Place this file NEXT TO singbox-launcher.exe and run it as administrator.
rem ---------------------------------------------------------------------------

set "TASKNAME=singbox-launcher"
set "EXEPATH=%~dp0singbox-launcher.exe"
if not exist "%EXEPATH%" set "EXEPATH=%~dp0singbox-launcher-win7-32.exe"

if not exist "%EXEPATH%" (
    echo [ERROR] singbox-launcher.exe not found next to this script.
    echo         Expected: %~dp0singbox-launcher.exe
    echo         Move this .bat into the launcher folder and run it again.
    goto :end
)

net session >nul 2>&1
if errorlevel 1 (
    echo [ERROR] Administrator rights are required.
    echo         Right-click this file and choose "Run as administrator".
    goto :end
)

echo Creating scheduled task "%TASKNAME%"...
schtasks /Create /TN "%TASKNAME%" /SC ONLOGON /RL HIGHEST /TR "\"%EXEPATH%\"" /F
if errorlevel 1 (
    echo.
    echo [ERROR] Failed to create the task. See the message above.
    goto :end
)

echo.
echo [OK] Task "%TASKNAME%" created. The launcher will start at the next logon.
echo.
echo Notes:
echo   - To start minimized to tray with the VPN up, edit the task action and
echo     append the flags:  -start -tray
echo   - If the launcher hangs at logon, open Task Scheduler and add a delay
echo     of 30 seconds to 1 minute to the "At log on" trigger.
echo   - To remove the task, run autostart_remove.bat as administrator.

:end
echo.
pause
endlocal
