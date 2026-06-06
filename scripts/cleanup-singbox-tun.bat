@echo off
REM ============================================================
REM  Singbox Launcher - cleanup ghost TUN adapters (Windows 7+)
REM ============================================================
REM
REM  WHAT THIS DOES:
REM    Opens Device Manager with hidden devices visible so you
REM    can manually remove the leftover "singbox-tunN" adapters
REM    that accumulate after each launcher start on Windows 7.
REM
REM  HOW TO USE:
REM    1. Double-click this file (no admin needed to LIST devices,
REM       but you WILL need admin to UNINSTALL them - Device
REM       Manager will ask).
REM    2. In Device Manager: View -> Show hidden devices.
REM    3. Expand "Network adapters".
REM    4. Ghost adapters appear faded/grey and named "singbox-tun0",
REM       "singbox-tun1", etc.
REM    5. Right-click each grey one -> Uninstall device -> OK.
REM    6. Leave the ACTIVE (non-grey) singbox-tunN alone if VPN
REM       is currently running, or stop the launcher first.
REM
REM  WHY THIS HAPPENS:
REM    On Windows 7 the WinTun driver's adapter cleanup is
REM    unreliable - Plug-and-Play Manager keeps dead device
REM    instances around. Each launcher start creates a new
REM    incrementally-named adapter (tun0, tun1, tun2...) to
REM    avoid conflict with the ghost.
REM
REM  ALTERNATIVE (one-command cleanup via devcon.exe):
REM    See cleanup-singbox-tun-devcon.bat in the same folder.
REM ============================================================

echo.
echo ======================================================
echo  Singbox Launcher - ghost TUN adapter cleanup
echo ======================================================
echo.
echo Opening Device Manager with hidden devices visible...
echo.
echo Steps:
echo   1. View -^> Show hidden devices
echo   2. Expand "Network adapters"
echo   3. Right-click each grey "singbox-tun*" -^> Uninstall
echo   4. Close Device Manager when done
echo.

set DEVMGR_SHOW_NONPRESENT_DEVICES=1
start devmgmt.msc

echo Device Manager launched. Press any key to close this window.
pause >nul
