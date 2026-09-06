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

type AnthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type ModelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

var httpClient = &http.Client{
	Timeout: 50 * time.Second,
}

func maskKey(k string) string {
	if len(k) <= 8 {
		return "***"
	}
	return k[:4] + "..." + k[len(k)-4:]
}

func getBaseURL(cfg Config) string {
	u := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if u == "" {
		return "https://openrouter.ai/api/v1"
	}
	return u
}

// TranscribeAudio envia o áudio WAV para a API do modelo transcrever
func TranscribeAudio(wavPath string, modelName string, cfg Config) (string, error) {
	audioBytes, err := os.ReadFile(wavPath)
	if err != nil {
		LogError(err, "Falha ao ler arquivo WAV: %s", wavPath)
		return "", fmt.Errorf("falha ao ler wav: %w", err)
	}
	if len(audioBytes) < 1000 {
		LogWarn("Arquivo WAV muito pequeno (%d bytes). Ignorando.", len(audioBytes))
		return "", nil
	}

	b64Audio := base64.StdEncoding.EncodeToString(audioBytes)
	baseURL := getBaseURL(cfg)
	LogInfo("Iniciando transcrição de áudio (%d bytes base64) para o modelo '%s' via '%s'", len(b64Audio), modelName, baseURL)

	// Se o provedor for Anthropic direto, a API nativa não possui suporte direto a WAV puro;
	// No entanto, se o usuário estiver usando um proxy/gateway Anthropic ou formato compatível:
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

	text, err := sendOpenAIRequest(apiURL, payload1, cfg)
	if err == nil && strings.TrimSpace(text) != "" {
		LogInfo("Transcrição com input_audio concluída com sucesso (%d caracteres)", len(text))
		return text, nil
	}

	LogWarn("Tentativa primária de transcrição falhou (%v). Tentando formato alternativo (data URL)...", err)

	// Formato 2: data URL (fallback para modelos multimodais que aceitam áudio como data URI)
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

	textFallback, errFallback := sendOpenAIRequest(apiURL, payload2, cfg)
	if errFallback == nil && strings.TrimSpace(textFallback) != "" {
		LogInfo("Transcrição fallback (data URL) concluída com sucesso (%d caracteres)", len(textFallback))
		return textFallback, nil
	}

	if errFallback != nil {
		LogError(errFallback, "Falha na transcrição do áudio com ambos os formatos")
		return "", errFallback
	}
	return text, err
}

// RewriteText envia o texto transcrito completo para pós-processamento e reescrita
func RewriteText(rawText string, modelName string, cfg Config) (string, error) {
	if strings.TrimSpace(rawText) == "" {
		return "", nil
	}

	baseURL := getBaseURL(cfg)
	LogInfo("Iniciando reescrita de texto (%d caracteres) com o modelo '%s' (Provedor: %s)", len(rawText), modelName, cfg.Provider)

	promptSistema := "Você é um assistente de pós-processamento de fala e transcrição. " +
		"Receberá o texto bruto falado que foi transcrito de um microfone em blocos. " +
		"Sua tarefa é reescrever o texto completo corrigindo pontuação, concordância, ortografia, " +
		"remoção de repetições desnecessárias, gaguejos e vícios de fala, deixando-o coeso, limpo e natural. " +
		"NÃO altere o significado nem acrescente ideias inexistentes. " +
		"Retorne APENAS o texto final corrigido e polido."

	if cfg.Provider == "anthropic" {
		return sendAnthropicRequest(baseURL, modelName, promptSistema, rawText, cfg)
	}

	// Provedor padrão: OpenAI compatível
	apiURL := baseURL + "/chat/completions"
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

	return sendOpenAIRequest(apiURL, payload, cfg)
}

func sendOpenAIRequest(apiURL string, payload map[string]any, cfg Config) (string, error) {
	bodyJSON, err := json.Marshal(payload)
	if err != nil {
		LogError(err, "Falha ao serializar payload OpenAI")
		return "", err
	}

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(bodyJSON))
	if err != nil {
		LogError(err, "Falha ao criar requisição HTTP para %s", apiURL)
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "WhisperGo/1.0")
	req.Header.Set("HTTP-Referer", "https://github.com/whispergo")
	req.Header.Set("X-Title", "WhisperGo")
	if cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}

	LogDebug("POST %s (Chave: %s)", apiURL, maskKey(cfg.APIKey))

	resp, err := httpClient.Do(req)
	if err != nil {
		LogError(err, "Erro na requisição HTTP para %s", apiURL)
		return "", err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		LogError(err, "Erro ao ler resposta HTTP de %s", apiURL)
		return "", err
	}

	var chatResp ChatCompletionResponse
	if err := json.Unmarshal(respBytes, &chatResp); err != nil {
		errParse := fmt.Errorf("resposta inválida da API (Status %d): %s", resp.StatusCode, string(respBytes))
		LogError(errParse, "Falha no parse JSON de %s", apiURL)
		return "", errParse
	}

	if resp.StatusCode != http.StatusOK {
		errMsg := fmt.Sprintf("Status HTTP %d", resp.StatusCode)
		if chatResp.Error != nil && chatResp.Error.Message != "" {
			errMsg += ": " + chatResp.Error.Message
		} else {
			errMsg += ": " + string(respBytes)
		}
		errApi := errors.New(errMsg)
		LogError(errApi, "API retornou erro HTTP %d", resp.StatusCode)
		return "", errApi
	}

	if len(chatResp.Choices) > 0 {
		return strings.TrimSpace(chatResp.Choices[0].Message.Content), nil
	}

	LogWarn("Resposta da API sem choices. Conteúdo: %s", string(respBytes))
	return "", nil
}

func sendAnthropicRequest(baseURL string, model string, systemPrompt string, rawText string, cfg Config) (string, error) {
	apiURL := baseURL
	if !strings.HasSuffix(apiURL, "/messages") {
		if strings.HasSuffix(apiURL, "/v1") {
			apiURL = apiURL + "/messages"
		} else {
			apiURL = apiURL + "/v1/messages"
		}
	}

	payload := map[string]any{
		"model":      model,
		"max_tokens": 4096,
		"system":     systemPrompt,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": fmt.Sprintf("Texto transcrito bruto:\n\n%s", rawText),
			},
		},
	}

	bodyJSON, err := json.Marshal(payload)
	if err != nil {
		LogError(err, "Falha ao serializar payload Anthropic")
		return "", err
	}

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(bodyJSON))
	if err != nil {
		LogError(err, "Falha ao criar requisição Anthropic para %s", apiURL)
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", cfg.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("User-Agent", "WhisperGo/1.0")

	LogDebug("POST %s (Anthropic, Chave: %s)", apiURL, maskKey(cfg.APIKey))

	resp, err := httpClient.Do(req)
	if err != nil {
		LogError(err, "Erro na requisição Anthropic para %s", apiURL)
		return "", err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		LogError(err, "Erro ao ler resposta Anthropic de %s", apiURL)
		return "", err
	}

	var anthResp AnthropicResponse
	if err := json.Unmarshal(respBytes, &anthResp); err != nil {
		errParse := fmt.Errorf("resposta inválida da Anthropic (Status %d): %s", resp.StatusCode, string(respBytes))
		LogError(errParse, "Falha no parse JSON de Anthropic")
		return "", errParse
	}

	if resp.StatusCode != http.StatusOK {
		errMsg := fmt.Sprintf("Status HTTP %d", resp.StatusCode)
		if anthResp.Error != nil && anthResp.Error.Message != "" {
			errMsg += ": " + anthResp.Error.Message
		} else {
			errMsg += ": " + string(respBytes)
		}
		errApi := errors.New(errMsg)
		LogError(errApi, "API Anthropic retornou erro")
		return "", errApi
	}

	if len(anthResp.Content) > 0 {
		for _, c := range anthResp.Content {
			if c.Type == "text" && c.Text != "" {
				return strings.TrimSpace(c.Text), nil
			}
		}
	}

	return "", nil
}

// FetchAvailableModels consulta o endpoint de modelos
func FetchAvailableModels(cfg Config) ([]string, error) {
	baseURL := getBaseURL(cfg)
	apiURL := baseURL + "/models"

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		LogError(err, "Falha ao criar requisição /models")
		return getFallbackModels(cfg.Provider), err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "WhisperGo/1.0")
	if cfg.Provider == "anthropic" {
		req.Header.Set("x-api-key", cfg.APIKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	} else if cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		LogWarn("Não foi possível buscar modelos em %s: %v. Usando modelos padrão.", apiURL, err)
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
			LogInfo("Obtidos %d modelos com sucesso de %s", len(models), apiURL)
			return models, nil
		}
	}

	LogWarn("Endpoint %s retornou status %d. Usando fallback.", apiURL, resp.StatusCode)
	return getFallbackModels(cfg.Provider), nil
}

func getFallbackModels(provider string) []string {
	if provider == "anthropic" {
		return []string{
			"claude-3-7-sonnet-20250219",
			"claude-3-5-sonnet-20241022",
			"claude-3-5-haiku-20241022",
			"claude-3-opus-20240229",
		}
	}
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
