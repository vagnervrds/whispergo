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
	ctx          *malgo.AllocatedContext
	device       *malgo.Device
	cfg          Config
	isRecording  atomic.Bool
	stopChan     chan struct{}
	chunkChan    chan AudioChunk
	mu           sync.Mutex
	currentVol   atomic.Value // float64
	currentSec   atomic.Value // float64
	isWaitingSec atomic.Bool
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
		return nil, fmt.Errorf("falha ao inicializar malgo: %w", err)
	}
	defer ctx.Uninit()

	devices, err := ctx.Devices(malgo.Capture)
	if err != nil {
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
	return list, nil
}

// Start inicia a captura de áudio com chunking inteligente contínuo
func (r *AudioRecorder) Start(onVolumeUpdate func(vol float64, sec float64, isPauseWait bool)) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.isRecording.Load() {
		return fmt.Errorf("gravação já em andamento")
	}

	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
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
	if r.cfg.SelectedMicrophone != "" && r.cfg.SelectedMicrophone != "default" {
		devs, _ := ctx.Devices(malgo.Capture)
		for _, d := range devs {
			if d.Name() == r.cfg.SelectedMicrophone {
				deviceConfig.Capture.DeviceID = d.ID.Pointer()
				break
			}
		}
	}

	rawSamplesChan := make(chan []float32, 500)
	r.stopChan = make(chan struct{})
	r.isRecording.Store(true)

	onRecvFrames := func(pOutput, pInput []byte, frameCount uint32) {
		if !r.isRecording.Load() || frameCount == 0 {
			return
		}
		n := int(frameCount)
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

		select {
		case rawSamplesChan <- samples:
		default:
		}
	}

	captureCallbacks := malgo.DeviceCallbacks{
		Data: onRecvFrames,
	}

	device, err := malgo.InitDevice(ctx.Context, deviceConfig, captureCallbacks)
	if err != nil {
		ctx.Uninit()
		return fmt.Errorf("falha ao inicializar dispositivo de áudio: %w", err)
	}
	r.device = device

	if err := device.Start(); err != nil {
		device.Uninit()
		ctx.Uninit()
		return fmt.Errorf("falha ao iniciar stream de áudio: %w", err)
	}

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
		silenceThreshold = 0.012
	}

	ticker := time.NewTicker(80 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-r.stopChan:
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
				if dur >= 0.3 {
					r.chunkChan <- AudioChunk{
						Index:   chunkIndex,
						Samples: buffer,
						Reason:  fmt.Sprintf("Bloco Final ao encerrar (%.1fs)", dur),
						IsFinal: true,
					}
				}
			}
			close(r.chunkChan)
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
				reason := fmt.Sprintf("Pausa detectada (%.1fs)", elapsedSec)
				if limitReached {
					reason = fmt.Sprintf("Limite máximo de %.0fs atingido", maxChunkSec)
				}

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

// Stop finaliza a gravação e retorna o canal de chunks
func (r *AudioRecorder) Stop() <-chan AudioChunk {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.isRecording.Load() {
		return r.chunkChan
	}

	r.isRecording.Store(false)
	close(r.stopChan)

	if r.device != nil {
		r.device.Stop()
		r.device.Uninit()
	}
	if r.ctx != nil {
		r.ctx.Uninit()
	}

	return r.chunkChan
}
