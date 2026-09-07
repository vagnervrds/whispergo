package main

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

var (
	modUser32               = syscall.NewLazyDLL("user32.dll")
	modKernel32             = syscall.NewLazyDLL("kernel32.dll")
	procRegisterHotKey      = modUser32.NewProc("RegisterHotKey")
	procUnregisterHotKey    = modUser32.NewProc("UnregisterHotKey")
	procGetMessage          = modUser32.NewProc("GetMessageW")
	procPostThreadMessage   = modUser32.NewProc("PostThreadMessageW")
	procShowWindow          = modUser32.NewProc("ShowWindow")
	procSetForegroundWindow = modUser32.NewProc("SetForegroundWindow")
	procBringWindowToTop    = modUser32.NewProc("BringWindowToTop")
	procSetWindowPos        = modUser32.NewProc("SetWindowPos")
	procGetCurrentThreadId  = modKernel32.NewProc("GetCurrentThreadId")
)

const (
	MOD_ALT      = 0x0001
	MOD_CONTROL  = 0x0002
	MOD_SHIFT    = 0x0004
	MOD_WIN      = 0x0008
	MOD_NOREPEAT = 0x4000

	WM_HOTKEY = 0x0312
	WM_QUIT   = 0x0012
	WM_APP    = 0x8000
	WM_RELOAD = WM_APP + 1

	SW_RESTORE     = 9
	SWP_NOMOVE     = 0x0002
	SWP_NOSIZE     = 0x0001
	SWP_SHOWWINDOW = 0x0040

	HWND_TOPMOST   = ^uintptr(0) // -1
	HWND_NOTOPMOST = ^uintptr(1) // -2
)

type POINT struct {
	X, Y int32
}

type MSG struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      POINT
}

// SetWindowTopmost coloca ou remove a janela em modo flutuante fixo no primeiro plano
func SetWindowTopmost(hwnd uintptr, topmost bool) {
	if hwnd == 0 {
		return
	}
	insertAfter := HWND_NOTOPMOST
	if topmost {
		insertAfter = HWND_TOPMOST
	}
	_, _, _ = procSetWindowPos.Call(
		hwnd,
		insertAfter,
		0, 0, 0, 0,
		SWP_NOMOVE|SWP_NOSIZE|SWP_SHOWWINDOW,
	)
}

// RestoreAndFocusWindow restaura da barra de tarefas (caso minimizada) e traz ao foco
func RestoreAndFocusWindow(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	_, _, _ = procShowWindow.Call(hwnd, SW_RESTORE)
	_, _, _ = procBringWindowToTop.Call(hwnd)
	_, _, _ = procSetForegroundWindow.Call(hwnd)
}

// parseHotkeyString decompõe strings como "Ctrl + Alt + Win + R" em modificadores e Virtual Key code
func parseHotkeyString(hotkeyStr string) (uintptr, uintptr, error) {
	parts := strings.Split(hotkeyStr, "+")
	var mod uintptr
	var vk uintptr
	var keyPart string

	for _, p := range parts {
		p = strings.ToUpper(strings.TrimSpace(p))
		switch p {
		case "CTRL", "CONTROL":
			mod |= MOD_CONTROL
		case "ALT":
			mod |= MOD_ALT
		case "SHIFT":
			mod |= MOD_SHIFT
		case "WIN", "WINDOWS", "META", "SUPER":
			mod |= MOD_WIN
		default:
			if p != "" {
				keyPart = p
			}
		}
	}

	if keyPart == "" {
		return 0, 0, fmt.Errorf("nenhuma tecla principal especificada no atalho: %s", hotkeyStr)
	}

	// Teclas de função F1-F12
	if strings.HasPrefix(keyPart, "F") && len(keyPart) <= 3 {
		var fNum int
		if _, err := fmt.Sscanf(keyPart, "F%d", &fNum); err == nil && fNum >= 1 && fNum <= 12 {
			vk = uintptr(0x70 + (fNum - 1))
		}
	}

	if vk == 0 {
		switch keyPart {
		case "SPACE", "ESPACO", "ESPAÇO":
			vk = 0x20
		case "ENTER", "RETURN":
			vk = 0x0D
		case "TAB":
			vk = 0x09
		case "ESCAPE", "ESC":
			vk = 0x1B
		default:
			if len(keyPart) == 1 {
				ch := keyPart[0]
				// 'A'-'Z' ou '0'-'9'
				if (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') {
					vk = uintptr(ch)
				}
			}
		}
	}

	if vk == 0 {
		return 0, 0, fmt.Errorf("tecla '%s' não suportada como atalho global", keyPart)
	}

	// Sempre aplica MOD_NOREPEAT para evitar triggers repetidos seguidos ao manter a tecla pressionada
	mod |= MOD_NOREPEAT

	return mod, vk, nil
}

type HotkeyManager struct {
	hwnd          uintptr
	threadID      uintptr
	currentHotkey string
	onTrigger     func()
	mu            sync.Mutex
	running       bool
}

var globalHotkeyMgr *HotkeyManager

// StartGlobalHotkeyManager inicializa o gerenciador de atalho global em background
func StartGlobalHotkeyManager(hwnd uintptr, initialHotkey string, onTrigger func()) *HotkeyManager {
	mgr := &HotkeyManager{
		hwnd:          hwnd,
		currentHotkey: initialHotkey,
		onTrigger:     onTrigger,
		running:       true,
	}
	globalHotkeyMgr = mgr

	readyChan := make(chan uintptr)

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		tId, _, _ := procGetCurrentThreadId.Call()
		readyChan <- tId

		const hotkeyID = 1001
		var isRegistered bool

		register := func(str string) {
			if isRegistered {
				_, _, _ = procUnregisterHotKey.Call(0, hotkeyID)
				isRegistered = false
			}

			mod, vk, err := parseHotkeyString(str)
			if err != nil {
				LogWarn("Atalho global inválido '%s': %v", str, err)
				return
			}

			ret, _, err := procRegisterHotKey.Call(0, hotkeyID, mod, vk)
			if ret != 0 {
				isRegistered = true
				LogInfo("Atalho de teclado global ativado: [%s]", str)
			} else {
				LogWarn("Falha ao registrar atalho global '%s' no Windows (pode estar em uso por outro programa): %v", str, err)
			}
		}

		// Registra o atalho inicial
		register(initialHotkey)
		defer func() {
			if isRegistered {
				_, _, _ = procUnregisterHotKey.Call(0, hotkeyID)
			}
		}()

		var msg MSG
		for mgr.running {
			res, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
			if int32(res) <= 0 {
				break
			}

			if msg.Message == WM_HOTKEY && msg.WParam == hotkeyID {
				LogInfo("Atalho global disparado pelo usuário")
				// Restaura a janela se estiver minimizada e traz para o primeiro plano
				RestoreAndFocusWindow(mgr.hwnd)

				if mgr.onTrigger != nil {
					mgr.onTrigger()
				}
			} else if msg.Message == WM_RELOAD {
				mgr.mu.Lock()
				hk := mgr.currentHotkey
				mgr.mu.Unlock()
				LogInfo("Atualizando atalho global para: [%s]", hk)
				register(hk)
			}
		}
	}()

	mgr.threadID = <-readyChan
	return mgr
}

// UpdateHotkey atualiza dinamicamente a combinação de atalho
func (m *HotkeyManager) UpdateHotkey(newHotkey string) {
	if m == nil || m.threadID == 0 {
		return
	}
	m.mu.Lock()
	m.currentHotkey = newHotkey
	m.mu.Unlock()

	_, _, _ = procPostThreadMessage.Call(m.threadID, WM_RELOAD, 0, 0)
}

// Stop encerra o loop de escuta de atalhos e libera os atalhos registrados
func (m *HotkeyManager) Stop() {
	if m == nil {
		return
	}
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return
	}
	m.running = false
	threadID := m.threadID
	m.mu.Unlock()

	if threadID != 0 {
		_, _, _ = procPostThreadMessage.Call(threadID, WM_QUIT, 0, 0)
	}
}

