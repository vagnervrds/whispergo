package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const ConfigFileName = "whisper_config.json"

type Config struct {
	Provider            string  `json:"provider"`
	BaseURL             string  `json:"base_url"`
	APIKey              string  `json:"api_key"`
	LastAudioModel      string  `json:"last_audio_model"`
	LastRewriteModel    string  `json:"last_rewrite_model"`
	SelectedMicrophone  string  `json:"selected_microphone"`
	ChunkSeconds        int     `json:"chunk_seconds"`
	MinChunkSeconds     int     `json:"min_chunk_seconds"`
	MaxChunkSeconds     int     `json:"max_chunk_seconds"`
	SilencePauseSeconds float64 `json:"silence_pause_seconds"`
	SilenceThreshold    float64 `json:"silence_threshold"`
	SampleRate          int     `json:"sample_rate"`
}

func DefaultConfig() Config {
	return Config{
		Provider:            "openrouter",
		BaseURL:             "https://openrouter.ai/api/v1",
		APIKey:              "",
		LastAudioModel:      "google/gemini-2.5-flash",
		LastRewriteModel:    "google/gemini-2.5-flash",
		SelectedMicrophone:  "default",
		ChunkSeconds:        30,
		MinChunkSeconds:     20,
		MaxChunkSeconds:     35,
		SilencePauseSeconds: 0.45,
		SilenceThreshold:    0.012,
		SampleRate:          16000,
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
		// Tenta verificar se existe config.json alternativo
		if altData, errAlt := os.ReadFile("config.json"); errAlt == nil {
			var alt struct {
				BaseURL       string `json:"base_url"`
				APIKey        string `json:"api_key"`
				SelectedModel string `json:"selected_model"`
			}
			if json.Unmarshal(altData, &alt) == nil {
				if alt.BaseURL != "" {
					cfg.BaseURL = alt.BaseURL
				}
				if alt.APIKey != "" {
					cfg.APIKey = alt.APIKey
				}
				if alt.SelectedModel != "" {
					cfg.LastRewriteModel = alt.SelectedModel
				}
			}
		}
		_ = SaveConfig(cfg)
		return cfg
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg
	}

	// Garante valores mínimos e consistentes
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://openrouter.ai/api/v1"
	}
	if cfg.Provider == "" {
		cfg.Provider = "openrouter"
	}
	if cfg.LastAudioModel == "" {
		cfg.LastAudioModel = "google/gemini-2.5-flash"
	}
	if cfg.LastRewriteModel == "" {
		cfg.LastRewriteModel = "google/gemini-2.5-flash"
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
	if cfg.SilencePauseSeconds <= 0 {
		cfg.SilencePauseSeconds = 0.45
	}
	if cfg.SilenceThreshold <= 0 {
		cfg.SilenceThreshold = 0.012
	}

	return cfg
}

func SaveConfig(cfg Config) error {
	configPath := getConfigPath()
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, data, 0644)
}
