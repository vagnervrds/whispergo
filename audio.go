package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gen2brain/malgo"
)

type AudioDevice struct {
	Index     int    `json:"index"`
	Name      string `json:"name"`
	IsDefault bool   `json:"is_default"`
}

type AudioChunk struct {
	Index   int
	Samples []float32
	Reason  string
	IsFinal bool
}

type AudioRecorder struct {
	ctx         *malgo.AllocatedContext
	device      *malgo.Device
	cfg         Config
	isRecording atomic.Bool
	stopChan    chan struct{}
	chunkChan   chan AudioChunk
	mu          sync.Mutex
	currentVol  atomic.Value // float64
	currentSec  atomic.Value // float64
	targetDevID *malgo.DeviceID
}

func NewAudioRecorder(cfg Config) *AudioRecorder {
	r := &AudioRecorder{
		cfg:       cfg,
		chunkChan: make(chan AudioChunk, 100),
		stopChan:  make(chan struct{}),
	}
	r.currentVol.Store(float64(0))
	r.currentSec.Store(float64(0))
	return r
}

// ListMicrophones lista todos os microfones válidos disponíveis
func ListMicrophones() ([]AudioDevice, error) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		LogError(err, "Falha ao inicializar malgo para listar microfones")
		return nil, fmt.Errorf("falha ao inicializar malgo: %w", err)
	}
	defer ctx.Uninit()

	devices, err := ctx.Devices(malgo.Capture)
	if err != nil {
		LogError(err, "Falha ao consultar dispositivos de captura")
		return nil, fmt.Errorf("falha ao listar dispositivos de captura: %w", err)
	}

	var list []AudioDevice
	seen := make(map[string]bool)

	for i, d := range devices {
		name := d.Name()
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		list = append(list, AudioDevice{
			Index:     i,
			Name:      name,
			IsDefault: i == 0,
		})
	}
	LogInfo("Detectados %d microfone(s) no sistema", len(list))
	return list, nil
}

// Start inicia a captura de áudio com chunking inteligente contínuo
func (r *AudioRecorder) Start(onVolumeUpdate func(vol float64, sec float64, isPauseWait bool)) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.isRecording.Load() {
		return fmt.Errorf("gravação já em andamento")
	}

	LogInfo("Iniciando gravação de áudio...")
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		LogError(err, "Falha ao inicializar contexto de áudio miniaudio")
		return fmt.Errorf("falha ao inicializar contexto de áudio: %w", err)
	}
	r.ctx = ctx

	sampleRate := uint32(r.cfg.SampleRate)
	if sampleRate == 0 {
		sampleRate = 16000
	}

	deviceConfig := malgo.DefaultDeviceConfig(malgo.Capture)
	deviceConfig.Capture.Format = malgo.FormatF32
	deviceConfig.Capture.Channels = 1
	deviceConfig.SampleRate = sampleRate
	deviceConfig.Alsa.NoMMap = 1

	// Seleção de dispositivo configurado
	micName := "Padrão do Sistema"
	if r.cfg.SelectedMicrophone != "" && r.cfg.SelectedMicrophone != "default" {
		devs, _ := ctx.Devices(malgo.Capture)
		for _, d := range devs {
			if d.Name() == r.cfg.SelectedMicrophone {
				devIDCopy := d.ID
				r.targetDevID = &devIDCopy
				deviceConfig.Capture.DeviceID = r.targetDevID.Pointer()
				micName = d.Name()
				break
			}
		}
	}

	LogInfo("Configurando dispositivo de captura: '%s' (Taxa: %dHz, 1 canal, Float32)", micName, sampleRate)

	rawSamplesChan := make(chan []float32, 500)
	r.stopChan = make(chan struct{})
	r.isRecording.Store(true)

	var totalSamplesReceived int64

	onRecvFrames := func(pOutput, pInput []byte, frameCount uint32) {
		if !r.isRecording.Load() || frameCount == 0 {
			return
		}

		n := int(frameCount)
		if len(pInput) < n*4 {
			n = len(pInput) / 4
		}
		if n <= 0 {
			return
		}

		samples := make([]float32, n)
		var sumSq float64
		for i := 0; i < n; i++ {
			u := binary.LittleEndian.Uint32(pInput[i*4 : (i+1)*4])
			val := math.Float32frombits(u)
			samples[i] = val
			sumSq += float64(val * val)
		}

		rms := math.Sqrt(sumSq / float64(n))
		normVol := math.Min(1.0, rms*18.0)
		r.currentVol.Store(normVol)
		atomic.AddInt64(&totalSamplesReceived, int64(n))

		select {
		case rawSamplesChan <- samples:
		default:
			LogWarn("Fila de áudio cheia, frame descartado")
		}
	}

	captureCallbacks := malgo.DeviceCallbacks{
		Data: onRecvFrames,
	}

	device, err := malgo.InitDevice(ctx.Context, deviceConfig, captureCallbacks)
	if err != nil {
		LogError(err, "Falha ao inicializar dispositivo de áudio miniaudio")
		ctx.Uninit()
		return fmt.Errorf("falha ao inicializar dispositivo de áudio: %w", err)
	}
	r.device = device

	if err := device.Start(); err != nil {
		LogError(err, "Falha ao iniciar stream de áudio")
		device.Uninit()
		ctx.Uninit()
		return fmt.Errorf("falha ao iniciar stream de áudio: %w", err)
	}

	LogInfo("Stream de microfone iniciado com sucesso no hardware")

	// Goroutine de chunking e detecção de pausas
	go r.chunkingWorker(rawSamplesChan, sampleRate, onVolumeUpdate)

	return nil
}

func (r *AudioRecorder) chunkingWorker(rawChan <-chan []float32, sampleRate uint32, onVolumeUpdate func(vol float64, sec float64, isPauseWait bool)) {
	var buffer []float32
	chunkIndex := 1
	var accumulatedSilence float64
	minChunkSec := float64(r.cfg.MinChunkSeconds)
	if minChunkSec <= 0 {
		minChunkSec = 20.0
	}
	maxChunkSec := float64(r.cfg.MaxChunkSeconds)
	if maxChunkSec <= 0 {
		maxChunkSec = 35.0
	}
	silencePauseSec := r.cfg.SilencePauseSeconds
	if silencePauseSec <= 0 {
		silencePauseSec = 0.45
	}
	silenceThreshold := r.cfg.SilenceThreshold
	if silenceThreshold <= 0 {
		silenceThreshold = 0.008
	}

	ticker := time.NewTicker(80 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-r.stopChan:
			LogInfo("Finalizando captura de áudio. Drenando buffers pendentes...")
			// Esvazia as amostras pendentes
			for {
				select {
				case chunk := <-rawChan:
					buffer = append(buffer, chunk...)
				default:
					goto drained
				}
			}
		drained:
			if len(buffer) > 0 {
				dur := float64(len(buffer)) / float64(sampleRate)
				LogInfo("Bloco final com duração: %.2fs (%d amostras)", dur, len(buffer))
				if dur >= 0.3 {
					r.chunkChan <- AudioChunk{
						Index:   chunkIndex,
						Samples: buffer,
						Reason:  fmt.Sprintf("Bloco Final ao encerrar (%.1fs)", dur),
						IsFinal: true,
					}
				} else {
					LogInfo("Bloco final muito curto (<0.3s). Descartado.")
				}
			}
			close(r.chunkChan)
			LogInfo("Canal de chunks fechado com sucesso")
			return

		case chunk, ok := <-rawChan:
			if !ok {
				return
			}
			buffer = append(buffer, chunk...)
			chunkDur := float64(len(chunk)) / float64(sampleRate)

			// RMS deste frame individual
			var sumSq float64
			for _, s := range chunk {
				sumSq += float64(s * s)
			}
			chunkRMS := math.Sqrt(sumSq / float64(len(chunk)))

			if chunkRMS < silenceThreshold {
				accumulatedSilence += chunkDur
			} else {
				accumulatedSilence = 0
			}

			elapsedSec := float64(len(buffer)) / float64(sampleRate)
			r.currentSec.Store(elapsedSec)

			pauseDetected := elapsedSec >= minChunkSec && accumulatedSilence >= silencePauseSec
			limitReached := elapsedSec >= maxChunkSec

			if pauseDetected || limitReached {
				reason := fmt.Sprintf("Pausa detectada (%.1fs, silêncio acumulado: %.2fs)", elapsedSec, accumulatedSilence)
				if limitReached {
					reason = fmt.Sprintf("Limite máximo de %.0fs atingido", maxChunkSec)
				}

				LogInfo("Corte de Bloco %d: %s", chunkIndex, reason)

				chunkToSend := make([]float32, len(buffer))
				copy(chunkToSend, buffer)
				buffer = nil
				accumulatedSilence = 0
				r.currentSec.Store(float64(0))

				r.chunkChan <- AudioChunk{
					Index:   chunkIndex,
					Samples: chunkToSend,
					Reason:  reason,
					IsFinal: false,
				}
				chunkIndex++
			}

		case <-ticker.C:
			if onVolumeUpdate != nil && r.isRecording.Load() {
				vol, _ := r.currentVol.Load().(float64)
				sec, _ := r.currentSec.Load().(float64)
				isPauseWait := sec >= minChunkSec
				onVolumeUpdate(vol, sec, isPauseWait)
			}
		}
	}
}

// Stop finaliza a gravação
func (r *AudioRecorder) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.isRecording.Load() {
		return
	}

	LogInfo("Comando Stop recebido. Parando dispositivo de gravação...")
	r.isRecording.Store(false)
	close(r.stopChan)

	if r.device != nil {
		r.device.Stop()
		r.device.Uninit()
	}
	if r.ctx != nil {
		r.ctx.Uninit()
	}
}
