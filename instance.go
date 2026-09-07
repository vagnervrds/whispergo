package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// KillPreviousInstances encerra forçadamente quaisquer outras instâncias do WhisperGo
// que já estejam abertas ou congeladas em segundo plano, liberando arquivos do WebView2,
// portas de rede e atalhos globais para que a nova instância inicialize normalmente.
func KillPreviousInstances() {
	myPid := uint32(os.Getpid())

	exePath, err := os.Executable()
	var currentExe string
	if err == nil {
		currentExe = strings.ToLower(filepath.Base(exePath))
	}

	snapshot, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return
	}
	defer syscall.CloseHandle(snapshot)

	var entry syscall.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))

	var pidsToKill []uint32

	if err := syscall.Process32First(snapshot, &entry); err == nil {
		for {
			if entry.ProcessID != myPid && entry.ProcessID != 0 {
				procName := strings.ToLower(syscall.UTF16ToString(entry.ExeFile[:]))
				if (currentExe != "" && procName == currentExe) ||
					strings.HasPrefix(procName, "whispergo") ||
					strings.HasPrefix(procName, "whispergoals") {
					pidsToKill = append(pidsToKill, entry.ProcessID)
				}
			}
			if err := syscall.Process32Next(snapshot, &entry); err != nil {
				break
			}
		}
	}

	if len(pidsToKill) == 0 {
		return
	}

	for _, pid := range pidsToKill {
		// 1. Usa taskkill /F /T para encerrar toda a árvore de processos (incluindo msedgewebview2.exe filhos)
		// Sem janela de terminal piscando (CreationFlags: 0x08000000 = CREATE_NO_WINDOW)
		cmd := exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(int(pid)))
		cmd.SysProcAttr = &syscall.SysProcAttr{
			HideWindow:    true,
			CreationFlags: 0x08000000,
		}
		_ = cmd.Run()

		// 2. Redundância nativa com TerminateProcess
		hProc, err := syscall.OpenProcess(syscall.PROCESS_TERMINATE, false, pid)
		if err == nil {
			_ = syscall.TerminateProcess(hProc, 1)
			_ = syscall.CloseHandle(hProc)
		}
		LogInfo("Instância anterior identificada e finalizada (PID: %d)", pid)
	}

	// Breve pausa para o sistema operacional desalocar handles, soquetes e diretórios do WebView2
	time.Sleep(300 * time.Millisecond)
}
