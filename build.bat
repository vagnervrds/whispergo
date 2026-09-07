@echo off
setlocal enabledelayedexpansion
title WhisperGo - Build ^& Release

REM Suporte a execucao direta por argumento (ex: build.bat 1, build.bat 2, build.bat 3, build.bat 0)
if "%1"=="1" goto do_build_only
if "%1"=="2" goto do_build_and_release
if "%1"=="3" goto do_release_only
if "%1"=="0" goto do_exit

:menu
cls
echo ======================================================
echo            WhisperGo - Painel de Controle
echo ======================================================
echo.
echo  [1] Apenas fazer o Build (compilar WhisperGo.exe)
echo  [2] Fazer o Build e Enviar Release para o GitHub
echo  [3] Apenas Enviar Release para o GitHub (sem compilar)
echo  [0] Sair
echo.
echo ======================================================
set "OPCAO=1"
set /p "OPCAO=Escolha uma opcao [Padrao: 1]: "

if "%OPCAO%"=="1" goto do_build_only
if "%OPCAO%"=="2" goto do_build_and_release
if "%OPCAO%"=="3" goto do_release_only
if "%OPCAO%"=="0" goto do_exit

echo [AVISO] Opcao invalida. Tente novamente.
ping 127.0.0.1 -n 3 >nul
goto menu

:do_build_only
set "TRIGGER_RELEASE=0"
goto start_build

:do_build_and_release
set "TRIGGER_RELEASE=1"
goto start_build

:do_release_only
echo.
echo ======================================================
echo  [INFO] Executando generate_release_notes.py...
echo ======================================================
where python >nul 2>&1
if !errorlevel! equ 0 (
    python generate_release_notes.py
) else (
    where py >nul 2>&1
    if !errorlevel! equ 0 (
        py generate_release_notes.py
    ) else (
        echo [ERRO] Python nao foi encontrado no sistema.
    )
)
goto finish

:start_build
echo.
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
    "$json.last_build_time = (Get-Date).ToString('dd/MM/yyyy HH:mm:ss');" ^
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
    if not "%2"=="--debug" (
        set LDFLAGS=%LDFLAGS% -H windowsgui
    )
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

    if "!TRIGGER_RELEASE!"=="1" (
        echo.
        echo ======================================================
        echo  [INFO] Executando generate_release_notes.py...
        echo ======================================================
        where python >nul 2>&1
        if !errorlevel! equ 0 (
            python generate_release_notes.py
        ) else (
            where py >nul 2>&1
            if !errorlevel! equ 0 (
                py generate_release_notes.py
            ) else (
                echo [ERRO] Python nao foi encontrado no sistema.
            )
        )
    )
) else (
    echo.
    echo ======================================================
    echo  [ERRO] Falha na compilacao! Codigo: %ERRORLEVEL%
    echo ======================================================
)

goto finish

:do_exit
echo.
echo [INFO] Operacao cancelada pelo usuario.
goto finish

:finish
echo.
pause
