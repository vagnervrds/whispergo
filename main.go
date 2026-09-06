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
	workerDoneChan   chan struct{}
	transcribedTexts []string
	textsMutex       sync.Mutex
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

func main() {
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
			Width:  390,
			Height: 460,
			Center: true,
		},
	})
	if w == nil {
		errWv := fmt.Errorf("erro ao inicializar WebView2. Verifique se o WebView2 Runtime está instalado")
		LogError(errWv, "Falha na inicialização do WebView2")
		return
	}
	defer w.Destroy()

	// Ajusta a barra de título do Windows para o tema escuro da aplicação (#0b0f19)
	applyDarkTitleBar(uintptr(w.Window()))

	_ = LoadConfig()

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

		textsMutex.Lock()
		transcribedTexts = nil
		textsMutex.Unlock()

		done := make(chan struct{})
		workerDoneChan = done

		onVol := func(vol float64, sec float64, isPauseWait bool) {
			w.Dispatch(func() {
				w.Eval(fmt.Sprintf("window.onVolumeUpdate(%f, %f, %t);", vol, sec, isPauseWait))
			})
		}

		if err := rec.Start(onVol); err != nil {
			LogError(err, "Falha ao iniciar AudioRecorder")
			return map[string]string{"error": err.Error()}
		}

		activeRecorder = rec

		// Goroutine assíncrona consumindo e transcrevendo blocos
		go func(r *AudioRecorder, c Config, d chan struct{}) {
			defer close(d)
			tempDir := os.TempDir()

			for chunk := range r.chunkChan {
				LogInfo("Iniciando transcrição do Bloco %d (%d amostras, Final: %t, Motivo: %s)",
					chunk.Index, len(chunk.Samples), chunk.IsFinal, chunk.Reason)

				rawWav := filepath.Join(tempDir, fmt.Sprintf("whisper_raw_%d_%d.wav", time.Now().UnixNano(), chunk.Index))
				cleanWav := filepath.Join(tempDir, fmt.Sprintf("whisper_clean_%d_%d.wav", time.Now().UnixNano(), chunk.Index))

				if err := WriteWavFile(rawWav, chunk.Samples, c.SampleRate); err != nil {
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

				w.Dispatch(func() {
					w.Eval(fmt.Sprintf("window.onStatusChange('Transcrevendo Bloco %d...', 'processing');", chunk.Index))
				})

				text, err := TranscribeAudio(wavToSend, c.LastAudioModel, c)
				if err != nil {
					LogError(err, "Erro ao transcrever Bloco %d", chunk.Index)
				}

				// Remove arquivos temporários
				_ = os.Remove(rawWav)
				_ = os.Remove(cleanWav)

				if strings.TrimSpace(text) != "" {
					LogInfo("Bloco %d transcrito com sucesso: \"%s\"", chunk.Index, text)
					textsMutex.Lock()
					transcribedTexts = append(transcribedTexts, text)
					textsMutex.Unlock()

					escapedText, _ := json.Marshal(text)
					w.Dispatch(func() {
						w.Eval(fmt.Sprintf("window.onChunkTranscribed(%d, %s);", chunk.Index, string(escapedText)))
					})
				} else {
					LogInfo("Bloco %d finalizado sem falas identificadas", chunk.Index)
				}
			}
			LogInfo("Worker de transcrição processou todos os blocos")
		}(rec, currentCfg, done)

		return map[string]string{"status": "ok"}
	})

	// Binding: stopRecording
	w.Bind("stopRecording", func() bool {
		recorderMutex.Lock()
		rec := activeRecorder
		activeRecorder = nil
		doneChan := workerDoneChan
		recorderMutex.Unlock()

		if rec == nil {
			return false
		}

		currentCfg := LoadConfig()

		go func() {
			w.Dispatch(func() {
				w.Eval("window.onStatusChange('Finalizando gravação e processando blocos...', 'processing');")
			})

			// Para a gravação física (esvazia buffer e envia o bloco final)
			rec.Stop()

			// Aguarda o worker terminar de transcrever todos os blocos pendentes
			if doneChan != nil {
				<-doneChan
			}

			textsMutex.Lock()
			fullRaw := strings.Join(transcribedTexts, " ")
			textsMutex.Unlock()

			LogInfo("Gravação encerrada. Texto bruto acumulado (%d caracteres): \"%s\"", len(fullRaw), fullRaw)

			if strings.TrimSpace(fullRaw) == "" {
				LogWarn("Nenhuma fala identificada ao encerrar gravação")
				w.Dispatch(func() {
					w.Eval("window.onStatusChange('Nenhuma fala identificada.', '');")
				})
				return
			}

			w.Dispatch(func() {
				w.Eval("window.onStatusChange('Reescrevendo e polindo texto...', 'processing');")
			})

			finalText, err := RewriteText(fullRaw, currentCfg.LastRewriteModel, currentCfg)
			if err != nil || strings.TrimSpace(finalText) == "" {
				LogError(err, "Falha ao reescrever texto. Utilizando texto bruto.")
				finalText = fullRaw
			} else {
				LogInfo("Texto polido e reescrito com sucesso (%d caracteres)", len(finalText))
			}

			// Copia para a área de transferência
			_ = clipboard.WriteAll(finalText)
			LogInfo("Texto final copiado para o Clipboard")

			// Salva em transcricao_final.txt
			_ = os.WriteFile("transcricao_final.txt", []byte(finalText+"\n"), 0644)
			LogInfo("Texto gravado em transcricao_final.txt")

			escapedRaw, _ := json.Marshal(fullRaw)
			escapedFinal, _ := json.Marshal(finalText)

			w.Dispatch(func() {
				w.Eval(fmt.Sprintf("window.onFinalTextReady(%s, %s);", string(escapedRaw), string(escapedFinal)))
			})
		}()

		return true
	})

	targetURL := fmt.Sprintf("http://127.0.0.1:%d/index.html", port)
	LogInfo("Carregando WebView2 em: %s", targetURL)
	w.Navigate(targetURL)
	w.Run()
}
