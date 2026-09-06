# WhisperGo 🎙️✨

Aplicativo desktop ultraleve e minimalista desenvolvido em **Golang** para transcrição contínua de áudio via microfone e reescrita inteligente com modelos de Inteligência Artificial na nuvem (OpenRouter, ProxyAI ou Provedores Customizados compatíveis com a API OpenAI).

---

## 🌟 Funcionalidades

- **Interface Gráfica Minimalista Dark:** Janela compacta, tema escuro moderno, feedback em tempo real e visualizador de ondas sonoras reativo ao volume da voz.
- **Detecção Inteligente de Pausas (VAD):** Gravação ininterrupta com corte dinâmico na primeira pausa entre palavras após 20 segundos (máx 35s), evitando cortar palavras no meio.
- **Pré-processamento de Áudio:** Remoção de ruídos e silêncios utilizando o filtro `silenceremove` do FFmpeg antes do envio para a nuvem.
- **Execução Assíncrona:** Cada bloco de áudio cortado é processado e transcrito em segundo plano enquanto você continua falando o próximo bloco.
- **Reescrita e Polimento Automático:** Ao finalizar a fala, todo o texto é reunido e polido por um modelo de IA para corrigir pontuação, ortografia, hesitações e gaguejos, deixando-o coeso e natural.
- **Área de Transferência Automática:** O texto final é automaticamente copiado para o Clipboard do sistema operacional e gravado no arquivo `transcricao_final.txt`.
- **Configurações Flexíveis:**
  - Provedor padrão: **OpenRouter** (`https://openrouter.ai/api/v1`).
  - Suporte a ProxyAI e qualquer servidor compatível com OpenAI.
  - Seleção de modelos de áudio e texto.
  - Detecção e seleção de microfones reais do sistema.
  - Persistência das credenciais e configurações em `whisper_config.json` (ignorado pelo Git para total segurança).

---

## 🚀 Como Compilar e Executar

### Pré-requisitos
1. **Go** (1.20 ou superior).
2. **FFmpeg** instalado e adicionado ao `PATH` do sistema.
3. **WebView2 Runtime** (já incluído nativamente no Windows 10 e 11).

### Compilação Automatizada com Versionamento (`build.bat`)
O projeto conta com o script `build.bat` que controla e incrementa o número da build automaticamente no arquivo `build_info.json`:

```cmd
build.bat
```

Para compilar em modo debug (com console aberto para visualização de logs):
```cmd
build.bat --debug
```

O executável `WhisperGo.exe` será gerado na pasta raiz.

---

## ⚙️ Configuração (`whisper_config.json`)

Na primeira execução, o arquivo `whisper_config.json` é criado automaticamente. Exemplo de estrutura:

```json
{
  "provider": "openrouter",
  "base_url": "https://openrouter.ai/api/v1",
  "api_key": "sua-chave-aqui",
  "last_audio_model": "google/gemini-2.5-flash",
  "last_rewrite_model": "google/gemini-2.5-flash",
  "selected_microphone": "default",
  "chunk_seconds": 30,
  "min_chunk_seconds": 20,
  "max_chunk_seconds": 35,
  "silence_pause_seconds": 0.45,
  "silence_threshold": 0.012,
  "sample_rate": 16000
}
```

> **Nota de Segurança:** Os arquivos `whisper_config.json`, `config.json` e `transcricao_final.txt` estão incluídos no `.gitignore` para garantir que suas chaves de API e transcrições pessoais nunca sejam enviadas para repositórios públicos como o GitHub.

---

## 📁 Estrutura do Código

```
wisper_go/
├── main.go               # Ponto de entrada, webview, loopback e bindings RPC
├── config.go             # Leitura, validação e persistência do whisper_config.json
├── audio.go              # Captura de microfone (malgo/WASAPI) e VAD em tempo real
├── ffmpeg.go             # Filtro de áudio com silenceremove via FFmpeg
├── wav.go                # Codificador nativo WAV PCM 16-bit
├── api.go                # Requisições assíncronas /chat/completions e /models
├── assets/               # Frontend embutido na aplicação
│   ├── index.html        # Estrutura da interface
│   ├── app.css           # Tema escuro elegante
│   └── app.js            # Lógica reativa, animação canvas e eventos
├── build_info.json       # Contador incremental de builds
├── build.bat             # Script de compilação automática
├── whisper_config.example.json # Modelo de configuração sem credenciais
└── .gitignore            # Proteção contra vazamento de credenciais
```
