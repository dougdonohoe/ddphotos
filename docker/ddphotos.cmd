@echo off
setlocal EnableExtensions
rem SPDX-License-Identifier: AGPL-3.0-only
rem https://github.com/dougdonohoe/ddphotos
rem
rem ddphotos.cmd - Windows launcher for the 'ddphotos' bash wrapper.
rem
rem Git\bin\bash.exe, when run from cmd/Explorer, inherits the Windows PATH,
rem which does not include Git's Unix tools (dirname, awk, sed, grep, find,
rem sort, curl, ...). This launcher locates Git for Windows, prepends its
rem usr\bin and bin to PATH, then runs the 'ddphotos' bash script that lives
rem next to this file.
rem
rem To skip the search, set DDPHOTOS_BASH to the full path of bash.exe (e.g.
rem the path a frontend already confirmed with the user). If it points to a
rem real file it is used directly; otherwise the search below runs as a fallback.

set "BASH="

rem 1. Caller-supplied bash.exe via DDPHOTOS_BASH (skips the search).
if defined DDPHOTOS_BASH (
    if exist "%DDPHOTOS_BASH%" (
        set "BASH=%DDPHOTOS_BASH%"
    ) else (
        echo Warning: DDPHOTOS_BASH "%DDPHOTOS_BASH%" not found; searching for Git Bash. 1>&2
    )
)

rem 2. Search standard Git for Windows install locations.
if not defined BASH (
    for %%G in ("%ProgramFiles%\Git" "%ProgramFiles(x86)%\Git" "%LOCALAPPDATA%\Programs\Git") do (
        if not defined BASH if exist "%%~G\bin\bash.exe" set "BASH=%%~G\bin\bash.exe"
    )
)

rem 3. Fall back to bash.exe on PATH.
if not defined BASH (
    for %%B in (bash.exe) do set "BASH=%%~$PATH:B"
)

if not defined BASH (
    echo Error: Git Bash not found. Install Git for Windows: https://git-scm.com/download/win 1>&2
    echo        Or set DDPHOTOS_BASH to the full path of bash.exe. 1>&2
    exit /b 1
)

rem Prepend the Git Unix tools (usr\bin and bin, relative to bash.exe) to PATH
rem so dirname/awk/sed/etc. resolve.
for %%B in ("%BASH%") do set "BINDIR=%%~dpB"
set "PATH=%BINDIR%..\usr\bin;%BINDIR%;%PATH%"

"%BASH%" "%~dp0ddphotos" %*
exit /b %ERRORLEVEL%
