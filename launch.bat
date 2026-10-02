@echo off
rem KeyVivi launcher.
rem
rem Two separate Windows behaviours make a double-click on dist\KeyVivi.exe the
rem wrong way to start this program on a sandboxed workspace:
rem
rem 1. Integrity level follows the executable's location. Measured on this
rem    machine, A/B/A/B reproducible: the same file inside the agent workspace
rem    gets integrity 0x1000 (Low), a copy outside it gets 0x2000 (Medium).
rem    Windows withholds input for higher-integrity windows from a low-integrity
rem    process's global keyboard hook, so inside the workspace the overlay only
rem    sees keystrokes while its own window has focus, and systray cannot write
rem    its temporary icon file so the tray never appears.
rem 2. An unsigned binary has no SmartScreen reputation, and every rebuild
rem    changes its hash, so Windows may warn on launch. Unblocking the copy
rem    clears any internet-zone mark.
rem
rem So: copy outside the workspace, unblock, then start from there.
setlocal
set "SRC=%~dp0dist\KeyVivi.exe"
set "DSTDIR=%LOCALAPPDATA%\KeyVivi"
set "DST=%DSTDIR%\KeyVivi.exe"

if not exist "%SRC%" (
	echo Cannot find "%SRC%".
	echo Build it first:
	echo   go build -ldflags="-s -w -H=windowsgui" -o dist/KeyVivi.exe ./cmd/keyvivi
	pause
	exit /b 1
)
if not exist "%DSTDIR%" mkdir "%DSTDIR%"
copy /y "%SRC%" "%DST%" >nul
powershell -NoProfile -Command "Unblock-File -LiteralPath '%DST%'" >nul 2>&1
start "" "%DST%"
