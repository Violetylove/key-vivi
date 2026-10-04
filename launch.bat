@echo off
rem 本代理环境中，工作区内 exe 为 Low，工作区外副本为 Medium；启动方式不改变此限制。
rem Low 进程无法可靠采集更高完整性应用的输入，因此先复制到工作区外再启动。
rem 每次构建会改变未签名程序的哈希；解除网络来源标记不等于代码签名或建立信誉。
rem 副本位于用户本地目录，卸载时退出程序并删除该副本。
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
