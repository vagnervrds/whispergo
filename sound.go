package main

import (
	_ "embed"
	"syscall"
	"unsafe"
)

//go:embed assets/notify.wav
var notifySoundBytes []byte

var (
	modWinmmSound       = syscall.NewLazyDLL("winmm.dll")
	procPlaySoundWInner = modWinmmSound.NewProc("PlaySoundW")
)

const (
	SND_ASYNC     = 0x0001
	SND_NODEFAULT = 0x0002
	SND_MEMORY    = 0x0004
)

// PlayNotificationSound reproduz de forma assíncrona o sinal sonoro em segundo plano
func PlayNotificationSound() {
	if len(notifySoundBytes) == 0 {
		return
	}
	go func() {
		_, _, _ = procPlaySoundWInner.Call(
			uintptr(unsafe.Pointer(&notifySoundBytes[0])),
			0,
			uintptr(SND_MEMORY|SND_ASYNC|SND_NODEFAULT),
		)
	}()
}
