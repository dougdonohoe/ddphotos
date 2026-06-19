@echo off
setlocal EnableExtensions
rem SPDX-License-Identifier: AGPL-3.0-only
rem https://github.com/dougdonohoe/ddphotos
rem
rem ddphotos.cmd - Windows launcher for the 'ddphotos' bash wrapper.
rem
rem Finds a Git for Windows bash.exe and uses it to run the 'ddphotos' bash
rem script next to this file. (ddphotos repairs its own PATH for the Unix tools.)
rem
rem Set DDPHOTOS_BASH to a bash.exe full path to skip the search.

set "BASH="

rem 1. Caller-supplied bash.exe (e.g. a frontend already confirmed the path).
if defined DDPHOTOS_BASH if exist "%DDPHOTOS_BASH%" set "BASH=%DDPHOTOS_BASH%"

rem 2. Standard Git for Windows locations. Literal C:\ paths cover callers with a
rem    reduced environment (e.g. a JVM) that lacks %ProgramFiles% / %LOCALAPPDATA%.
if not defined BASH (
    for %%G in (
        "%ProgramFiles%\Git"
        "%ProgramFiles(x86)%\Git"
        "%LOCALAPPDATA%\Programs\Git"
        "C:\Program Files\Git"
        "C:\Program Files (x86)\Git"
    ) do (
        if not defined BASH if exist "%%~G\bin\bash.exe" set "BASH=%%~G\bin\bash.exe"
    )
)

rem 3. Fall back to bash.exe on PATH.
if not defined BASH for %%B in (bash.exe) do if not defined BASH if exist "%%~$PATH:B" set "BASH=%%~$PATH:B"

if not defined BASH (
    echo Error: Git Bash not found. Install Git for Windows: https://git-scm.com/download/win 1>&2
    echo        Or set DDPHOTOS_BASH to the full path of bash.exe. 1>&2
    exit /b 1
)

"%BASH%" "%~dp0ddphotos" %*
exit /b %ERRORLEVEL%
