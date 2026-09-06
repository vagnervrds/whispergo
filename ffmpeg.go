package main

import (
	"os"
	"os/exec"
	"syscall"
)

// CleanAudioFFmpeg remove silêncios do áudio usando o FFmpeg com os mesmos parâmetros do Python
func CleanAudioFFmpeg(inputWav, outputWav string) bool {
	cmd := exec.Command(
		"ffmpeg", "-y", "-i", inputWav,
		"-af", "silenceremove=start_periods=1:start_duration=0.1:start_threshold=-40dB:stop_periods=-1:stop_duration=0.3:stop_threshold=-40dB",
		outputWav,
	)
	// Oculta janela de console no Windows
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	err := cmd.Run()
	if err != nil {
		return false
	}

	info, err := os.Stat(outputWav)
	if err != nil || info.Size() < 1000 {
		return false
	}
	return true
}
