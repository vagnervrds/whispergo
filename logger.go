package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	LogDir      = "log"
	ActiveLog   = "log/log.log"
	MaxLogBytes = 1048576 // 1MB
	MaxBackups  = 2
)

type Logger struct {
	mu sync.Mutex
}

var GlobalLogger = &Logger{}

func init() {
	_ = os.MkdirAll(LogDir, 0755)
}

func (l *Logger) checkAndRotate(bytesToAdd int) error {
	info, err := os.Stat(ActiveLog)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if info.Size()+int64(bytesToAdd) >= MaxLogBytes {
		now := time.Now()
		// Padrão: log_YYYY-MM-DD_HH-mm-ss-SSS.log
		millis := now.Nanosecond() / 1e6
		rotatedName := fmt.Sprintf("log_%04d-%02d-%02d_%02d-%02d-%02d-%03d.log",
			now.Year(), now.Month(), now.Day(),
			now.Hour(), now.Minute(), now.Second(), millis,
		)
		rotatedPath := filepath.Join(LogDir, rotatedName)

		if err := os.Rename(ActiveLog, rotatedPath); err != nil {
			return err
		}

		// Limpa arquivos antigos para manter apenas os 2 mais recentes
		l.cleanOldLogs()
	}
	return nil
}

func (l *Logger) cleanOldLogs() {
	files, err := os.ReadDir(LogDir)
	if err != nil {
		return
	}

	var rotatedFiles []os.FileInfo
	for _, f := range files {
		name := f.Name()
		if strings.HasPrefix(name, "log_") && strings.HasSuffix(name, ".log") {
			if info, err := f.Info(); err == nil {
				rotatedFiles = append(rotatedFiles, info)
			}
		}
	}

	// Ordena por data de modificação decrescente (mais recentes primeiro)
	sort.Slice(rotatedFiles, func(i, j int) bool {
		return rotatedFiles[i].ModTime().After(rotatedFiles[j].ModTime())
	})

	// Se tiver mais que MaxBackups (2), deleta os mais antigos
	if len(rotatedFiles) > MaxBackups {
		for _, f := range rotatedFiles[MaxBackups:] {
			_ = os.Remove(filepath.Join(LogDir, f.Name()))
		}
	}
}

func (l *Logger) log(level string, msg string, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	_ = os.MkdirAll(LogDir, 0755)

	now := time.Now()
	millis := now.Nanosecond() / 1e6
	timestamp := fmt.Sprintf("%04d-%02d-%02d %02d:%02d:%02d.%03d",
		now.Year(), now.Month(), now.Day(),
		now.Hour(), now.Minute(), now.Second(), millis,
	)

	// Localização do chamador
	caller := "unknown"
	if _, file, line, ok := runtime.Caller(2); ok {
		caller = fmt.Sprintf("%s:%d", filepath.Base(file), line)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[%s] [%s] [%s] %s\n", timestamp, level, caller, msg))

	if err != nil {
		sb.WriteString(fmt.Sprintf("    ERRO: %v\n", err))
		sb.WriteString("    STACK TRACE:\n")
		stack := debug.Stack()
		lines := strings.Split(string(stack), "\n")
		for _, sl := range lines {
			if strings.TrimSpace(sl) != "" {
				sb.WriteString(fmt.Sprintf("        %s\n", sl))
			}
		}
	}

	entry := sb.String()
	_ = l.checkAndRotate(len(entry))

	f, openErr := os.OpenFile(ActiveLog, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if openErr == nil {
		defer f.Close()
		_, _ = f.WriteString(entry)
	}
}

func LogInfo(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	GlobalLogger.log("INFO", msg, nil)
}

func LogWarn(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	GlobalLogger.log("WARN", msg, nil)
}

func LogError(err error, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	GlobalLogger.log("ERROR", msg, err)
}

func LogDebug(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	GlobalLogger.log("DEBUG", msg, nil)
}
