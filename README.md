# WhisperGo 🎙️✨

Aplicativo desktop ultraleve e minimalista desenvolvido em **Golang** para transcrição contínua de áudio via microfone e reescrita inteligente com modelos de Inteligência Artificial na nuvem (suporte a servidores compatíveis com OpenAI e Anthropic).

---

## 🌟 Funcionalidades

- **Interface Gráfica Minimalista Dark:** Janela compacta, tema escuro moderno, feedback em tempo real e visualizador de ondas sonoras reativo ao volume da voz.
- **Atalho Rápido (ENTER):** Pressione a tecla **ENTER** a qualquer momento para iniciar ou parar a gravação de forma ágil e intuitiva.
- **Detecção Inteligente de Pausas (VAD):** Gravação ininterrupta com corte dinâmico na primeira pausa entre palavras após 20 segundos (máx 35s), evitando cortar palavras no meio.
- **Pré-processamento de Áudio:** Remoção de ruídos e silêncios utilizando o filtro `silenceremove` do FFmpeg antes do envio para a nuvem.
- **Execução Assíncrona:** Cada bloco de áudio cortado é processado e transcrito em segundo plano enquanto você continua falando o próximo bloco.
- **Reescrita e Polimento Automático:** Ao finalizar a fala, todo o texto é reunido e polido por um modelo de IA para corrigir pontuação, ortografia, hesitações e gaguejos, deixando-o coeso e natural.
- **Área de Transferência Automática:** O texto final é automaticamente copiado para o Clipboard do sistema operacional e gravado no arquivo `transcricao_final.txt`.
- **Provedores Suportados:**
  - **OpenAI compatível** (Ex: OpenRouter, OpenAI, vLLM, LocalAI, etc.)
  - **Anthropic** (Claude 3.5, Claude 3.7, etc.)
  - URL definida livremente pelo usuário, com o OpenRouter servindo como referência de exemplo.
- **Sistema Robusto de Logs com Rotação:**
  - Arquivo ativo em `log/log.log` com rastreamento detalhado de chamadas e stack traces completos em caso de erro.
  - Rotação automática ao atingir **1MB** (1.048.576 bytes) com padrão `log_YYYY-MM-DD_HH-mm-ss-SSS.log`.
  - Mantém apenas os **últimos 2 arquivos** rotacionados, excluindo automaticamente os mais antigos.
- **Segurança e Privacidade:** Credenciais salvas em `whisper_config.json` e logs em `log/`, ambos ignorados pelo Git para segurança total ao publicar online.

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
  "provider": "openai",
  "base_url": "https://openrouter.ai/api/v1",
  "api_key": "sua-chave-aqui",
  "last_audio_model": "google/gemini-2.5-flash",
  "last_rewrite_model": "google/gemini-2.5-flash",
  "selected_microphone": "default",
  "chunk_seconds": 30,
  "min_chunk_seconds": 20,
  "max_chunk_seconds": 35,
  "silence_pause_seconds": 0.45,
  "silence_threshold": 0.008,
  "sample_rate": 16000
}
```

---

## 📁 Estrutura do Código

```
wisper_go/
├── main.go               # Ponto de entrada, webview, loopback e bindings RPC
├── config.go             # Leitura, validação e persistência do whisper_config.json
├── audio.go              # Captura de microfone (malgo/WASAPI) e VAD em tempo real
├── ffmpeg.go             # Filtro de áudio com silenceremove via FFmpeg
├── wav.go                # Codificador nativo WAV PCM 16-bit
├── api.go                # Requisições assíncronas (OpenAI e Anthropic)
├── logger.go             # Logger com rotação de 1MB e retenção dos 2 últimos
├── assets/               # Frontend embutido na aplicação
│   ├── index.html        # Estrutura da interface
│   ├── app.css           # Tema escuro elegante
│   └── app.js            # Lógica reativa, atalho ENTER e animações
├── build_info.json       # Contador incremental de builds
├── build.bat             # Script de compilação automática
├── whisper_config.example.json # Modelo de configuração sem credenciais
└── .gitignore            # Proteção contra vazamento de credenciais e logs
```
