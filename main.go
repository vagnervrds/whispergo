package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/atotto/clipboard"
	"github.com/jchv/go-webview2"
)

//go:embed assets/*
var assetsFS embed.FS

var (
	BuildNumber      = "1"
	activeRecorder   *AudioRecorder
	recorderMutex    sync.Mutex
)

func getEffectiveBuildNumber() string {
	data, err := os.ReadFile("build_info.json")
	if err == nil {
		var bi struct {
			Build int `json:"build"`
		}
		if json.Unmarshal(data, &bi) == nil && bi.Build > 0 {
			return fmt.Sprintf("%d", bi.Build)
		}
	}
	return BuildNumber
}

func applyDarkTitleBar(hwnd uintptr) {
	dwm := syscall.NewLazyDLL("dwmapi.dll")
	setAttr := dwm.NewProc("DwmSetWindowAttribute")

	darkMode := int32(1)
	// DWMWA_USE_IMMERSIVE_DARK_MODE (20 no Win 11/10 20H1+, 19 nas builds anteriores)
	_, _, _ = setAttr.Call(hwnd, 20, uintptr(unsafe.Pointer(&darkMode)), 4)
	_, _, _ = setAttr.Call(hwnd, 19, uintptr(unsafe.Pointer(&darkMode)), 4)

	// DWMWA_CAPTION_COLOR = 35 (Windows 11) -> 0x00190F0B (#0b0f19)
	captionColor := uint32(0x00190F0B)
	_, _, _ = setAttr.Call(hwnd, 35, uintptr(unsafe.Pointer(&captionColor)), 4)

	// DWMWA_TEXT_COLOR = 36 (Windows 11) -> 0x00F9FAF1 (#f1f5f9)
	textColor := uint32(0x00F9FAF1)
	_, _, _ = setAttr.Call(hwnd, 36, uintptr(unsafe.Pointer(&textColor)), 4)
}

func setWindowIcon(hwnd uintptr) {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	user32 := syscall.NewLazyDLL("user32.dll")
	getModuleHandle := kernel32.NewProc("GetModuleHandleW")
	loadIcon := user32.NewProc("LoadIconW")
	sendMessage := user32.NewProc("SendMessageW")

	hInst, _, _ := getModuleHandle.Call(0)
	hIcon, _, _ := loadIcon.Call(hInst, uintptr(1))
	if hIcon != 0 {
		const (
			WM_SETICON = 0x0080
			ICON_SMALL = 0
			ICON_BIG   = 1
		)
		_, _, _ = sendMessage.Call(hwnd, WM_SETICON, ICON_BIG, hIcon)
		_, _, _ = sendMessage.Call(hwnd, WM_SETICON, ICON_SMALL, hIcon)
	}
}

func main() {
	KillPreviousInstances()
	LogInfo("Iniciando WhisperGo (Build: #%s)", getEffectiveBuildNumber())

	subFS, err := fs.Sub(assetsFS, "assets")
	if err != nil {
		LogError(err, "Falha ao carregar assets embutidos")
		panic(err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		LogError(err, "Falha ao iniciar listener loopback")
		panic(err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	server := &http.Server{
		Handler: http.FileServer(http.FS(subFS)),
	}
	go func() {
		_ = server.Serve(listener)
	}()

	LogInfo("Servidor interno de assets rodando na porta %d", port)

	// Janela compacta e moderna
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     false,
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title:  "WhisperGo",
			Width:  360,
			Height: 250,
			Center: true,
		},
	})
	if w == nil {
		errWv := fmt.Errorf("erro ao inicializar WebView2. Verifique se o WebView2 Runtime está instalado")
		LogError(errWv, "Falha na inicialização do WebView2")
		return
	}
	defer w.Destroy()

	// Garante tamanho inicial padronizado da área útil (modo compacto)
	w.SetSize(360, 208, webview2.HintNone)

	// Binding: setWindowSize para expandir e contrair a janela nativa
	w.Bind("setWindowSize", func(width, height int) {
		w.Dispatch(func() {
			w.SetSize(width, height, webview2.HintNone)
		})
	})

	hwnd := uintptr(w.Window())

	// Ajusta a barra de título do Windows para o tema escuro da aplicação (#0b0f19)
	applyDarkTitleBar(hwnd)
	setWindowIcon(hwnd)

	initialCfg := LoadConfig()

	// Inicia o gerenciador de atalho de teclado global
	StartGlobalHotkeyManager(hwnd, initialCfg.GlobalHotkey, func() {
		w.Dispatch(func() {
			w.Eval("btnRecord.click();")
		})
	})

	// Binding: getConfig
	w.Bind("getConfig", func() string {
		c := LoadConfig()
		data, _ := json.Marshal(c)
		return string(data)
	})

	// Binding: saveConfig
	w.Bind("saveConfig", func(jsonStr string) bool {
		var newCfg Config
		if err := json.Unmarshal([]byte(jsonStr), &newCfg); err != nil {
			LogError(err, "Falha ao desserializar configurações recebidas da UI")
			return false
		}
		if err := SaveConfig(newCfg); err != nil {
			return false
		}
		if globalHotkeyMgr != nil {
			globalHotkeyMgr.UpdateHotkey(newCfg.GlobalHotkey)
		}
		return true
	})

	// Binding: listMicrophones
	w.Bind("listMicrophones", func() string {
		mics, err := ListMicrophones()
		if err != nil {
			LogError(err, "Erro ao listar microfones")
			return "[]"
		}
		data, _ := json.Marshal(mics)
		return string(data)
	})

	// Binding: fetchModels
	w.Bind("fetchModels", func() string {
		c := LoadConfig()
		models, err := FetchAvailableModels(c)
		if err != nil {
			LogWarn("Aviso ao buscar modelos: %v", err)
		}
		data, _ := json.Marshal(models)
		return string(data)
	})

	// Binding: copyToClipboard
	w.Bind("copyToClipboard", func(text string) bool {
		err := clipboard.WriteAll(text)
		if err != nil {
			LogError(err, "Falha ao copiar para clipboard")
		}
		return err == nil
	})

	// Binding: playNotificationSound
	w.Bind("playNotificationSound", func() {
		PlayNotificationSound()
	})

	// Binding: saveSingleRecording
	w.Bind("saveSingleRecording", func(text string, timeStr string) map[string]any {
		_ = os.MkdirAll("transcricoes", 0755)
		now := time.Now()
		millis := now.Nanosecond() / 1e6
		filename := fmt.Sprintf("gravacao_%04d-%02d-%02d_%02d-%02d-%02d-%03d.txt",
			now.Year(), now.Month(), now.Day(),
			now.Hour(), now.Minute(), now.Second(), millis)
		filePath := filepath.Join("transcricoes", filename)

		header := fmt.Sprintf("=== WhisperGo - Gravação [%s] ===\n\n", timeStr)
		err := os.WriteFile(filePath, []byte(header+text+"\n"), 0644)
		if err != nil {
			LogError(err, "Falha ao salvar gravação individual em %s", filePath)
			return map[string]any{"success": false, "error": err.Error()}
		}
		LogInfo("Gravação individual salva em: %s", filePath)
		return map[string]any{"success": true, "filename": filename, "path": filePath}
	})

	// Binding: saveSessionAll
	w.Bind("saveSessionAll", func(content string) map[string]any {
		_ = os.MkdirAll("transcricoes", 0755)
		now := time.Now()
		millis := now.Nanosecond() / 1e6
		filename := fmt.Sprintf("sessao_%04d-%02d-%02d_%02d-%02d-%02d-%03d.txt",
			now.Year(), now.Month(), now.Day(),
			now.Hour(), now.Minute(), now.Second(), millis)
		filePath := filepath.Join("transcricoes", filename)

		header := fmt.Sprintf("=====================================================\n"+
			"         WhisperGo - Sessão Completa de Gravações    \n"+
			"         Data: %s                                     \n"+
			"=====================================================\n\n",
			now.Format("02/01/2006 15:04:05"))

		err := os.WriteFile(filePath, []byte(header+content+"\n"), 0644)
		if err != nil {
			LogError(err, "Falha ao salvar sessão completa em %s", filePath)
			return map[string]any{"success": false, "error": err.Error()}
		}
		LogInfo("Sessão completa salva em: %s", filePath)
		return map[string]any{"success": true, "filename": filename, "path": filePath}
	})

	// Binding: getAppInfo
	w.Bind("getAppInfo", func() string {
		info := map[string]string{
			"build": getEffectiveBuildNumber(),
		}
		data, _ := json.Marshal(info)
		return string(data)
	})

	// Binding: startRecording
	w.Bind("startRecording", func() map[string]string {
		recorderMutex.Lock()
		defer recorderMutex.Unlock()

		if activeRecorder != nil {
			return map[string]string{"error": "ALREADY_RECORDING", "message": "Já existe uma gravação ativa"}
		}

		currentCfg := LoadConfig()

		// VERIFICAÇÃO CRÍTICA: API Key obrigatória antes de gravar!
		if strings.TrimSpace(currentCfg.APIKey) == "" {
			LogWarn("Tentativa de gravação bloqueada: API Key não configurada")
			return map[string]string{
				"error":   "API_KEY_REQUIRED",
				"message": "Nenhuma API Key configurada. Por favor, insira sua chave nas configurações antes de iniciar a gravação.",
			}
		}

		rec := NewAudioRecorder(currentCfg)

		onVol := func(vol float64, totalSec float64, isPauseWait bool) {
			w.Dispatch(func() {
				w.Eval(fmt.Sprintf("window.onVolumeUpdate(%f, %f, %t);", vol, totalSec, isPauseWait))
			})
		}

		if err := rec.Start(onVol); err != nil {
			LogError(err, "Falha ao iniciar AudioRecorder")
			return map[string]string{"error": err.Error()}
		}

		activeRecorder = rec
		LogInfo("Gravação iniciada com sucesso")

		// Coloca a janela em primeiro plano (Always-on-Top) enquanto grava
		RestoreAndFocusWindow(hwnd)
		SetWindowTopmost(hwnd, true)

		return map[string]string{"status": "ok"}
	})

	// Binding: stopRecording
	// Libera a gravação imediatamente para permitir que o usuário inicie outra gravação em seguida sem travar!
	w.Bind("stopRecording", func() bool {
		recorderMutex.Lock()
		rec := activeRecorder
		activeRecorder = nil
		recorderMutex.Unlock()

		// Remove a janela do modo Always-on-Top ao encerrar a gravação
		SetWindowTopmost(hwnd, false)

		if rec == nil {
			return false
		}

		currentCfg := LoadConfig()
		sessionStartTime := time.Now().Format("15:04:05")

		// Despacha o processamento em background independente para esta sessão!
		go func(r *AudioRecorder, cfg Config, sessTime string) {
			LogInfo("Iniciando fila de processamento em background para gravação de [%s]", sessTime)

			// Para a gravação física (libera o microfone para a próxima gravação!)
			r.Stop()

			var sessionTexts []string
			tempDir := os.TempDir()

			for chunk := range r.chunkChan {
				LogInfo("Fila: processando Bloco %d (%d amostras, Final: %t, Motivo: %s)",
					chunk.Index, len(chunk.Samples), chunk.IsFinal, chunk.Reason)

				rawWav := filepath.Join(tempDir, fmt.Sprintf("whisper_raw_%d_%d.wav", time.Now().UnixNano(), chunk.Index))
				cleanWav := filepath.Join(tempDir, fmt.Sprintf("whisper_clean_%d_%d.wav", time.Now().UnixNano(), chunk.Index))

				if err := WriteWavFile(rawWav, chunk.Samples, cfg.SampleRate); err != nil {
					LogError(err, "Falha ao gravar arquivo WAV temporário %s", rawWav)
					continue
				}

				wavToSend := rawWav
				if CleanAudioFFmpeg(rawWav, cleanWav) {
					wavToSend = cleanWav
					LogInfo("FFmpeg silenceremove aplicado com sucesso no Bloco %d", chunk.Index)
				} else {
					LogWarn("FFmpeg não reduziu o áudio do Bloco %d. Usando WAV original.", chunk.Index)
				}

				text, err := TranscribeAudio(wavToSend, cfg.LastAudioModel, cfg)
				if err != nil {
					LogError(err, "Erro ao transcrever Bloco %d", chunk.Index)
				}

				_ = os.Remove(rawWav)
				_ = os.Remove(cleanWav)

				if strings.TrimSpace(text) != "" {
					LogInfo("Bloco %d transcrito com sucesso: \"%s\"", chunk.Index, text)
					sessionTexts = append(sessionTexts, text)
				} else {
					LogInfo("Bloco %d finalizado sem falas identificadas", chunk.Index)
				}
			}

			fullRaw := strings.Join(sessionTexts, " ")
			LogInfo("Todos os blocos da gravação [%s] foram transcritos. Texto bruto (%d caracteres): \"%s\"",
				sessTime, len(fullRaw), fullRaw)

			if strings.TrimSpace(fullRaw) == "" {
				LogWarn("Nenhuma fala identificada na gravação de [%s]", sessTime)
				w.Dispatch(func() {
					w.Eval("window.onStatusChange('Nenhuma fala identificada.', '');")
				})
				return
			}

			w.Dispatch(func() {
				w.Eval("window.onStatusChange('Polindo texto...', 'processing');")
			})

			finalText, err := RewriteText(fullRaw, cfg.LastRewriteModel, cfg)
			if err != nil || strings.TrimSpace(finalText) == "" {
				LogError(err, "Falha ao reescrever texto. Utilizando texto bruto.")
				finalText = fullRaw
			} else {
				LogInfo("Texto polido e reescrito com sucesso para [%s] (%d caracteres)", sessTime, len(finalText))
			}

			// Copia para a área de transferência
			_ = clipboard.WriteAll(finalText)
			LogInfo("Texto final copiado para o Clipboard")

			// Salva em transcricao_final.txt
			_ = os.WriteFile("transcricao_final.txt", []byte(finalText+"\n"), 0644)

			// Sinal sonoro agradável após conclusão da otimização do texto
			if cfg.SoundNotification {
				PlayNotificationSound()
			}

			escapedFinal, _ := json.Marshal(finalText)
			escapedTime, _ := json.Marshal(sessTime)

			w.Dispatch(func() {
				w.Eval(fmt.Sprintf("window.onSessionFinished(%s, %s);", string(escapedFinal), string(escapedTime)))
			})
		}(rec, currentCfg, sessionStartTime)

		return true
	})

	targetURL := fmt.Sprintf("http://127.0.0.1:%d/index.html", port)
	LogInfo("Carregando WebView2 em: %s", targetURL)
	w.Navigate(targetURL)
	w.Run()

	if globalHotkeyMgr != nil {
		globalHotkeyMgr.Stop()
	}
	LogInfo("WhisperGo encerrado com sucesso")
	os.Exit(0)
}
