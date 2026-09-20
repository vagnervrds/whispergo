package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const ConfigFileName = "whisper_config.json"

type Config struct {
	Provider            string  `json:"provider"`              // "openai" ou "anthropic"
	BaseURL             string  `json:"base_url"`              // URL definida pelo usuário
	APIKey              string  `json:"api_key"`               // Chave da API
	LastAudioModel      string  `json:"last_audio_model"`      // Modelo de transcrição
	LastRewriteModel    string  `json:"last_rewrite_model"`    // Modelo de reescrita
	SelectedMicrophone  string  `json:"selected_microphone"`   // Microfone em uso
	ChunkSeconds        int     `json:"chunk_seconds"`
	MinChunkSeconds     int     `json:"min_chunk_seconds"`
	MaxChunkSeconds     int     `json:"max_chunk_seconds"`
	SilencePauseSeconds float64 `json:"silence_pause_seconds"`
	SilenceThreshold    float64 `json:"silence_threshold"`
	SampleRate          int     `json:"sample_rate"`
	GlobalHotkey        string  `json:"global_hotkey"`
	SoundNotification   bool    `json:"sound_notification"`
}

func DefaultConfig() Config {
	return Config{
		Provider:            "openai",
		BaseURL:             "",
		APIKey:              "",
		LastAudioModel:      "google/gemini-2.5-flash",
		LastRewriteModel:    "google/gemini-2.5-flash",
		SelectedMicrophone:  "default",
		ChunkSeconds:        30,
		MinChunkSeconds:     20,
		MaxChunkSeconds:     35,
		SilencePauseSeconds: 0.45,
		SilenceThreshold:    0.008, // Sensibilidade aprimorada para captar fala mais sutil
		SampleRate:          16000,
		GlobalHotkey:        "Ctrl + Alt + Win + R",
		SoundNotification:   true,
	}
}

func getConfigPath() string {
	exePath, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(exePath)
		target := filepath.Join(dir, ConfigFileName)
		if _, err := os.Stat(target); err == nil {
			return target
		}
	}
	return ConfigFileName
}

func LoadConfig() Config {
	cfg := DefaultConfig()
	configPath := getConfigPath()

	data, err := os.ReadFile(configPath)
	if err != nil {
		_ = SaveConfig(cfg)
		return cfg
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		LogWarn("Falha ao desserializar %s: %v. Usando padrão.", configPath, err)
		return cfg
	}

	// Normaliza provedores para apenas "openai" ou "anthropic"
	if cfg.Provider != "openai" && cfg.Provider != "anthropic" {
		cfg.Provider = "openai"
	}

	if cfg.SampleRate <= 0 {
		cfg.SampleRate = 16000
	}
	if cfg.MinChunkSeconds <= 0 {
		cfg.MinChunkSeconds = 20
	}
	if cfg.MaxChunkSeconds <= 0 {
		cfg.MaxChunkSeconds = 35
	}
	if cfg.MaxChunkSeconds <= cfg.MinChunkSeconds {
		cfg.MaxChunkSeconds = cfg.MinChunkSeconds + 5
	}
	if cfg.SilencePauseSeconds <= 0 {
		cfg.SilencePauseSeconds = 0.45
	}
	if cfg.SilenceThreshold <= 0 {
		cfg.SilenceThreshold = 0.008
	}
	if cfg.LastAudioModel == "" {
		cfg.LastAudioModel = "google/gemini-2.5-flash"
	}
	if cfg.LastRewriteModel == "" {
		cfg.LastRewriteModel = "google/gemini-2.5-flash"
	}
	if cfg.GlobalHotkey == "" {
		cfg.GlobalHotkey = "Ctrl + Alt + Win + R"
	}

	return cfg
}

func SaveConfig(cfg Config) error {
	configPath := getConfigPath()
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		LogError(err, "Falha ao codificar configurações para JSON")
		return err
	}
	err = os.WriteFile(configPath, data, 0644)
	if err != nil {
		LogError(err, "Falha ao gravar arquivo de configuração %s", configPath)
	} else {
		LogInfo("Configurações salvas com sucesso em %s (Provedor: %s, Microfone: %s)", configPath, cfg.Provider, cfg.SelectedMicrophone)
	}
	return err
}
