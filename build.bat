@echo off
setlocal enabledelayedexpansion
title Compilador WhisperGo

echo ======================================================
echo             WhisperGo - Script de Compilacao
echo ======================================================

set JSON_FILE=build_info.json

REM Verifica se o arquivo build_info.json existe, senao cria
if not exist "%JSON_FILE%" (
    echo {"build": 0, "last_build_time": ""} > "%JSON_FILE%"
)

REM Executa script powershell para incrementar o numero da build
for /f %%i in ('powershell -NoProfile -Command ^
    "$json = Get-Content '%JSON_FILE%' -Raw | ConvertFrom-Json;" ^
    "$json.build = [int]$json.build + 1;" ^
    "$json.last_build_time = (Get-Date).ToString('o');" ^
    "$json | ConvertTo-Json | Set-Content '%JSON_FILE%' -Encoding UTF8;" ^
    "Write-Output $json.build"') do (
    set BUILD_NUM=%%i
)

echo [INFO] Incrementando Build para: #%BUILD_NUM%
echo [INFO] Compilando WhisperGo.exe...

REM Flags de compilacao
REM Por padrao compila com -H windowsgui para interface limpa sem terminal preto
REM Para compilar com console de debug, execute: build.bat --debug
set LDFLAGS=-X main.BuildNumber=%BUILD_NUM%
if not "%1"=="--debug" (
    set LDFLAGS=%LDFLAGS% -H windowsgui
)

go build -ldflags "%LDFLAGS%" -o WhisperGo.exe .

if %ERRORLEVEL% equ 0 (
    echo.
    echo ======================================================
    echo  [SUCESSO] Build #%BUILD_NUM% gerada com sucesso!
    echo ======================================================
    echo  Executavel gerado: WhisperGo.exe
    echo  Data: %DATE% %TIME%
    echo ======================================================
) else (
    echo.
    echo ======================================================
    echo  [ERRO] Falha na compilacao! Codigo: %ERRORLEVEL%
    echo ======================================================
)

echo.
pause
