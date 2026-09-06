# WhisperGo 🎙️✨

Aplicativo desktop ultraleve e minimalista desenvolvido em **Golang** para transcrição contínua de áudio via microfone e reescrita inteligente com modelos de Inteligência Artificial na nuvem (suporte a servidores compatíveis com OpenAI e Anthropic).

---

## 🌟 Funcionalidades

- **Interface Gráfica Minimalista Dark:** Janela compacta (390x460 px), tema escuro moderno, barra do Windows estilizada com a mesma cor da aplicação via DWM Dark Mode.
- **Atalho Rápido (ENTER):** Pressione a tecla **ENTER** para iniciar ou parar a gravação de forma ágil e intuitiva.
- **Fila Assíncrona Não-Bloqueante:** Se você parar uma gravação e quiser iniciar outra imediatamente, pode fazê-lo sem esperar! O processamento anterior continua em segundo plano enquanto você já grava a próxima fala.
- **Timer Limpo e Contínuo:** Contador monotônico que exibe a duração real da sua fala (`00:00`, `00:15`, `01:20`...) sem reinícios estranhos a cada bloco.
- **Detecção Inteligente de Pausas (VAD):** Gravação ininterrupta com corte dinâmico na primeira pausa entre palavras após 20 segundos (máx 35s), evitando cortar palavras no meio.
- **Pré-processamento de Áudio:** Remoção de ruídos e silêncios utilizando o filtro `silenceremove` do FFmpeg antes do envio para a nuvem.
- **Reescrita e Polimento Automático:** Ao finalizar a fala, todo o texto é reunido e polido por um modelo de IA para corrigir pontuação, ortografia, hesitações e gaguejos, deixando-o coeso e natural.
- **Área de Transferência Automática:** O texto final é automaticamente copiado para o Clipboard do sistema operacional e gravado no arquivo `transcricao_final.txt`.
- **Exportação de Transcrições (.txt):**
  - Botão individual **💾 Salvar** no Histórico para exportar qualquer gravação como arquivo de texto com data e hora.
  - Botão **💾 Salvar Tudo** para exportar a sessão completa em um único arquivo organizado.
  - Os arquivos são salvos na pasta `transcricoes/`.
- **Provedores Suportados:**
  - **OpenAI compatível** (Ex: OpenRouter, OpenAI, vLLM, LocalAI, etc.)
  - **Anthropic** (Claude 3.5, Claude 3.7, etc.)
  - URL definida livremente pelo usuário, com o OpenRouter servindo como referência de exemplo.
- **Validação Prévia de API Key:** Bloqueio inteligente que avisa e abre as configurações caso nenhuma chave esteja configurada, evitando falar sem poder transcrever.
- **Sistema Robusto de Logs com Rotação:**
  - Arquivo ativo em `log/log.log` com rastreamento detalhado de chamadas e stack traces completos em caso de erro.
  - Rotação automática ao atingir **1MB** (1.048.576 bytes) com padrão `log_YYYY-MM-DD_HH-mm-ss-SSS.log`.
  - Mantém apenas os **últimos 2 arquivos** rotacionados, excluindo automaticamente os mais antigos.
- **Segurança e Privacidade:** Credenciais salvas em `whisper_config.json`, logs em `log/` e arquivos de áudio/texto ignorados pelo Git para segurança total ao publicar online.

---

## 🚀 Como Compilar e Executar

### Pré-requisitos
1. **Go** (1.20 ou superior).
2. **FFmpeg** instalado e adicionado ao `PATH` do sistema.
3. **WebView2 Runtime** (já incluído nativamente no Windows 10 e 11).

### Compilação Automatizada com Versionamento (`build.bat`)
O script `build.bat` encerra automaticamente instâncias anteriores, controla e incrementa o número da build no arquivo `build_info.json`:

```cmd
build.bat
```

Para compilar em modo debug (com console aberto para visualização de logs):
```cmd
build.bat --debug
```

O executável `WhisperGo.exe` será gerado na pasta raiz.
