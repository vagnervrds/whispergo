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
	"time"

	"github.com/atotto/clipboard"
	"github.com/jchv/go-webview2"
)

//go:embed assets/*
var assetsFS embed.FS

var (
	BuildNumber = "1"
	activeRecorder *AudioRecorder
	recorderMutex  sync.Mutex
	transcribedTexts []string
	textsMutex     sync.Mutex
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

func main() {
	// Servidor local de assets estáticos
	subFS, err := fs.Sub(assetsFS, "assets")
	if err != nil {
		panic(err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
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

	// Criação da Janela Nativa WebView2
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     false,
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title:  "WhisperGo",
			Width:  460,
			Height: 640,
			Center: true,
		},
	})
	if w == nil {
		fmt.Println("Erro ao inicializar WebView2. Certifique-se de ter o WebView2 Runtime instalado.")
		return
	}
	defer w.Destroy()

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
			return "[]"
		}
		data, _ := json.Marshal(mics)
		return string(data)
	})

	// Binding: fetchModels
	w.Bind("fetchModels", func() string {
		c := LoadConfig()
		models, _ := FetchAvailableModels(c)
		data, _ := json.Marshal(models)
		return string(data)
	})

	// Binding: copyToClipboard
	w.Bind("copyToClipboard", func(text string) bool {
		err := clipboard.WriteAll(text)
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
		rec := NewAudioRecorder(currentCfg)

		textsMutex.Lock()
		transcribedTexts = nil
		textsMutex.Unlock()

		onVol := func(vol float64, sec float64, isPauseWait bool) {
			w.Dispatch(func() {
				w.Eval(fmt.Sprintf("window.onVolumeUpdate(%f, %f, %t);", vol, sec, isPauseWait))
			})
		}

		if err := rec.Start(onVol); err != nil {
			return map[string]string{"error": err.Error()}
		}

		activeRecorder = rec

		// Goroutine assíncrona consumindo e transcrevendo blocos em tempo real
		go func(r *AudioRecorder, c Config) {
			tempDir := os.TempDir()

			for chunk := range r.chunkChan {
				rawWav := filepath.Join(tempDir, fmt.Sprintf("whisper_raw_%d_%d.wav", time.Now().UnixNano(), chunk.Index))
				cleanWav := filepath.Join(tempDir, fmt.Sprintf("whisper_clean_%d_%d.wav", time.Now().UnixNano(), chunk.Index))

				if err := WriteWavFile(rawWav, chunk.Samples, c.SampleRate); err != nil {
					continue
				}

				wavToSend := rawWav
				if CleanAudioFFmpeg(rawWav, cleanWav) {
					wavToSend = cleanWav
				}

				w.Dispatch(func() {
					w.Eval(fmt.Sprintf("window.onStatusChange('Transcrevendo Bloco %d...', 'processing');", chunk.Index))
				})

				text, _ := TranscribeAudio(wavToSend, c.LastAudioModel, c)

				// Remove arquivos temporários
				_ = os.Remove(rawWav)
				_ = os.Remove(cleanWav)

				if strings.TrimSpace(text) != "" {
					textsMutex.Lock()
					transcribedTexts = append(transcribedTexts, text)
					textsMutex.Unlock()

					escapedText, _ := json.Marshal(text)
					w.Dispatch(func() {
						w.Eval(fmt.Sprintf("window.onChunkTranscribed(%d, %s);", chunk.Index, string(escapedText)))
					})
				}
			}
		}(rec, currentCfg)

		return map[string]string{"status": "ok"}
	})

	// Binding: stopRecording
	w.Bind("stopRecording", func() bool {
		recorderMutex.Lock()
		rec := activeRecorder
		activeRecorder = nil
		recorderMutex.Unlock()

		if rec == nil {
			return false
		}

		currentCfg := LoadConfig()

		go func() {
			w.Dispatch(func() {
				w.Eval("window.onStatusChange('Finalizando blocos...', 'processing');")
			})

			// Aguarda todos os blocos serem processados
			chunkChan := rec.Stop()
			for range chunkChan {
				// Drena qualquer resíduo se houver
			}

			// Pequena pausa para garantir que o worker de transcrição termine o último bloco
			time.Sleep(800 * time.Millisecond)

			textsMutex.Lock()
			fullRaw := strings.Join(transcribedTexts, " ")
			textsMutex.Unlock()

			if strings.TrimSpace(fullRaw) == "" {
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
				finalText = fullRaw
			}

			// Copia para a área de transferência
			_ = clipboard.WriteAll(finalText)

			// Salva em transcricao_final.txt
			_ = os.WriteFile("transcricao_final.txt", []byte(finalText+"\n"), 0644)

			escapedRaw, _ := json.Marshal(fullRaw)
			escapedFinal, _ := json.Marshal(finalText)

			w.Dispatch(func() {
				w.Eval(fmt.Sprintf("window.onFinalTextReady(%s, %s);", string(escapedRaw), string(escapedFinal)))
			})
		}()

		return true
	})

	w.Navigate(fmt.Sprintf("http://127.0.0.1:%d/index.html", port))
	w.Run()
}

