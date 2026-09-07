@echo off
setlocal enabledelayedexpansion
title Compilador WhisperGo

echo ======================================================
echo             WhisperGo - Script de Compilacao
echo ======================================================

REM Fecha qualquer instancia do aplicativo em execucao para liberar o arquivo
echo [INFO] Encerrando instancias ativas do WhisperGo/WhisperGoals...
taskkill /F /IM WhisperGo.exe /T >nul 2>&1
taskkill /F /IM WhisperGoals.exe /T >nul 2>&1

REM Pequena pausa para garantir a liberacao do processo pelo sistema operacional
ping 127.0.0.1 -n 2 >nul

REM Se o executavel antigo ainda existir e estiver bloqueado, tenta renomear
if exist "WhisperGo.exe" (
    del /f /q "WhisperGo.exe" >nul 2>&1
    if exist "WhisperGo.exe" (
        echo [AVISO] WhisperGo.exe ainda em uso. Renomeando para substituir...
        ren "WhisperGo.exe" "WhisperGo_old_%RANDOM%.bak" >nul 2>&1
    )
)

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

REM Garante a geracao/atualizacao dos recursos de icone com go-winres
if exist "icon.ico" (
    where go-winres >nul 2>&1
    if !errorlevel! equ 0 (
        echo [INFO] Atualizando recursos de icone do executavel...
        go-winres simply --icon icon.ico --manifest gui --arch amd64 >nul 2>&1
    )
)

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
    REM Limpa possiveis arquivos renomeados residuais
    del /f /q WhisperGo_old_*.bak >nul 2>&1
    set "BUILD_SUCCESS=1"
) else (
    echo.
    echo ======================================================
    echo  [ERRO] Falha na compilacao! Codigo: %ERRORLEVEL%
    echo ======================================================
    set "BUILD_SUCCESS=0"
)

if "!BUILD_SUCCESS!"=="1" (
    echo.
    set "RUN_RELEASE=N"
    set "DO_RELEASE=0"
    set /p "RUN_RELEASE=Deseja executar o release (generate_release_notes.py)? (s/N) [Padrao: N]: "
    if /i "!RUN_RELEASE!"=="S" set "DO_RELEASE=1"
    if /i "!RUN_RELEASE!"=="SIM" set "DO_RELEASE=1"
    if /i "!RUN_RELEASE!"=="Y" set "DO_RELEASE=1"
    if /i "!RUN_RELEASE!"=="YES" set "DO_RELEASE=1"

    if "!DO_RELEASE!"=="1" (
        echo.
        echo ======================================================
        echo  [INFO] Executando generate_release_notes.py...
        echo ======================================================
        where python >nul 2>&1
        if !errorlevel! equ 0 (
            python generate_release_notes.py
        ) else (
            py generate_release_notes.py
        )
    ) else (
        echo [INFO] Release ignorado.
    )
)

echo.
pause


