@echo off
setlocal
rem ---------------------------------------------------------------------------
rem  singbox-launcher - remove the Task Scheduler autostart entry created by
rem  autostart_add.bat.
rem
rem  Run this file as administrator.
rem ---------------------------------------------------------------------------

set "TASKNAME=singbox-launcher"

net session >nul 2>&1
if errorlevel 1 (
    echo [ERROR] Administrator rights are required.
    echo         Right-click this file and choose "Run as administrator".
    goto :end
)

schtasks /Query /TN "%TASKNAME%" >nul 2>&1
if errorlevel 1 (
    echo [INFO] Task "%TASKNAME%" does not exist. Nothing to remove.
    goto :end
)

echo Deleting scheduled task "%TASKNAME%"...
schtasks /Delete /TN "%TASKNAME%" /F
if errorlevel 1 (
    echo.
    echo [ERROR] Failed to delete the task. See the message above.
    goto :end
)

echo.
echo [OK] Task "%TASKNAME%" removed. The launcher will no longer start at logon.

:end
echo.
pause
endlocal
