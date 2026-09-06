package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type ChatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Code    any    `json:"code"`
	} `json:"error,omitempty"`
}

type ModelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

var httpClient = &http.Client{
	Timeout: 45 * time.Second,
}

func buildHeaders(cfg Config) http.Header {
	h := make(http.Header)
	h.Set("Content-Type", "application/json")
	h.Set("User-Agent", "WhisperGo/1.0")
	h.Set("HTTP-Referer", "https://github.com/whispergo")
	h.Set("X-Title", "WhisperGo")
	if cfg.APIKey != "" {
		h.Set("Authorization", "Bearer "+cfg.APIKey)
	}
	return h
}

// TranscribeAudio envia o áudio WAV em base64 para a API do modelo transcrever
func TranscribeAudio(wavPath string, modelName string, cfg Config) (string, error) {
	audioBytes, err := os.ReadFile(wavPath)
	if err != nil {
		return "", fmt.Errorf("falha ao ler wav: %w", err)
	}
	if len(audioBytes) < 1000 {
		return "", nil
	}

	b64Audio := base64.StdEncoding.EncodeToString(audioBytes)
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	apiURL := baseURL + "/chat/completions"

	// Formato 1: input_audio (OpenAI / OpenRouter padrão)
	payload1 := map[string]any{
		"model": modelName,
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{
						"type": "input_audio",
						"input_audio": map[string]string{
							"data":   b64Audio,
							"format": "wav",
						},
					},
					map[string]any{
						"type": "text",
						"text": "Transcreva este áudio em português com máxima fidelidade. Retorne APENAS o texto falado, sem nenhuma palavra ou explicação adicional.",
					},
				},
			},
		},
	}

	text, err := sendChatCompletion(apiURL, payload1, cfg)
	if err == nil && text != "" {
		return text, nil
	}

	// Formato 2: data URL (fallback para modelos que aceitam data URL como imagem/multimodal)
	payload2 := map[string]any{
		"model": modelName,
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{
						"type": "image_url",
						"image_url": map[string]string{
							"url": "data:audio/wav;base64," + b64Audio,
						},
					},
					map[string]any{
						"type": "text",
						"text": "Transcreva este áudio em português com fidelidade. Retorne APENAS o texto falado.",
					},
				},
			},
		},
	}

	textFallback, errFallback := sendChatCompletion(apiURL, payload2, cfg)
	if errFallback == nil && textFallback != "" {
		return textFallback, nil
	}

	if err != nil {
		return "", err
	}
	return textFallback, errFallback
}

// RewriteText envia o texto transcrito completo para pós-processamento e reescrita
func RewriteText(rawText string, modelName string, cfg Config) (string, error) {
	if strings.TrimSpace(rawText) == "" {
		return "", nil
	}

	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	apiURL := baseURL + "/chat/completions"

	promptSistema := "Você é um assistente de pós-processamento de fala e transcrição. " +
		"Receberá o texto bruto falado que foi transcrito de um microfone em blocos. " +
		"Sua tarefa é reescrever o texto completo corrigindo pontuação, concordância, ortografia, " +
		"remoção de repetições desnecessárias, gaguejos e vícios de fala, deixando-o coeso, limpo e natural. " +
		"NÃO altere o significado nem acrescente ideias inexistentes. " +
		"Retorne APENAS o texto final corrigido e polido."

	payload := map[string]any{
		"model": modelName,
		"messages": []any{
			map[string]string{
				"role":    "system",
				"content": promptSistema,
			},
			map[string]string{
				"role":    "user",
				"content": fmt.Sprintf("Texto transcrito bruto:\n\n%s", rawText),
			},
		},
	}

	return sendChatCompletion(apiURL, payload, cfg)
}

func sendChatCompletion(apiURL string, payload map[string]any, cfg Config) (string, error) {
	bodyJSON, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(bodyJSON))
	if err != nil {
		return "", err
	}
	req.Header = buildHeaders(cfg)

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var chatResp ChatCompletionResponse
	if err := json.Unmarshal(respBytes, &chatResp); err != nil {
		return "", fmt.Errorf("resposta inválida da API: %s", string(respBytes))
	}

	if resp.StatusCode != http.StatusOK {
		errMsg := fmt.Sprintf("Status HTTP %d", resp.StatusCode)
		if chatResp.Error != nil && chatResp.Error.Message != "" {
			errMsg += ": " + chatResp.Error.Message
		} else {
			errMsg += ": " + string(respBytes)
		}
		return "", errors.New(errMsg)
	}

	if len(chatResp.Choices) > 0 {
		return strings.TrimSpace(chatResp.Choices[0].Message.Content), nil
	}

	return "", nil
}

// FetchAvailableModels consulta o endpoint /models
func FetchAvailableModels(cfg Config) ([]string, error) {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	apiURL := baseURL + "/models"

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header = buildHeaders(cfg)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return getFallbackModels(cfg.Provider), err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var modResp ModelsResponse
		if err := json.NewDecoder(resp.Body).Decode(&modResp); err == nil && len(modResp.Data) > 0 {
			var models []string
			for _, m := range modResp.Data {
				if m.ID != "" {
					models = append(models, m.ID)
				}
			}
			return models, nil
		}
	}

	return getFallbackModels(cfg.Provider), nil
}

func getFallbackModels(provider string) []string {
	if strings.Contains(strings.ToLower(provider), "openrouter") {
		return []string{
			"google/gemini-2.5-flash",
			"google/gemini-2.5-pro",
			"google/gemini-2.0-flash-001",
			"openai/gpt-4o-audio-preview",
			"openai/gpt-4o-mini",
			"anthropic/claude-3.5-sonnet",
			"meta-llama/llama-3.3-70b-instruct",
		}
	}
	return []string{
		"gemini-3.7-flash-high",
		"gemini-3.8-flash-high",
		"gemini-3.1-pro-low",
		"gemini-3.1-flash-lite",
		"claude-sonnet-4-6",
		"claude-opus-4-6-thinking",
		"gpt-oss-120b-medium",
		"grok-4.6",
	}
}
