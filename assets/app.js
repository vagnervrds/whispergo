// WhisperGo UI Logic - Compact Modern Recorder & Dynamic Screen Expansion

let isRecording = false;
let currentConfig = null;
let currentVolume = 0;
let animFrameId = null;
let sessionHistory = [];
let activePanel = null; // null = compacto, 'history' = gravações, 'settings' = configurações

// Elementos Principais
const appContainer = document.getElementById("appContainer");
const btnRecord = document.getElementById("btnRecord");
const recordBtnContainer = document.querySelector(".record-button-container");
const statusPill = document.getElementById("statusPill");
const statusText = document.getElementById("statusText");
const timerDisplay = document.getElementById("timerDisplay");
const pauseNotice = document.getElementById("pauseNotice");
const buildBadge = document.getElementById("buildBadge");

// Alternadores de Painel no Cabeçalho
const btnToggleHistory = document.getElementById("btnToggleHistory");
const btnToggleSettings = document.getElementById("btnToggleSettings");
const historyBadge = document.getElementById("historyBadge");

// Painel de Histórico / Gravações
const panelHistory = document.getElementById("panelHistory");
const btnCloseHistory = document.getElementById("btnCloseHistory");
const historyPanelCount = document.getElementById("historyPanelCount");
const historyList = document.getElementById("historyList");
const btnSaveAllHistory = document.getElementById("btnSaveAllHistory");
const btnClearHistory = document.getElementById("btnClearHistory");
const saveToast = document.getElementById("saveToast");

// Painel de Configurações
const panelSettings = document.getElementById("panelSettings");
const btnCloseSettings = document.getElementById("btnCloseSettings");
const btnCancelSettings = document.getElementById("btnCancelSettings");
const btnSaveSettings = document.getElementById("btnSaveSettings");
const apiKeyAlert = document.getElementById("apiKeyAlert");
const cfgProvider = document.getElementById("cfgProvider");
const cfgBaseURL = document.getElementById("cfgBaseURL");
const cfgAPIKey = document.getElementById("cfgAPIKey");
const btnToggleKey = document.getElementById("btnToggleKey");
const cfgMicrophone = document.getElementById("cfgMicrophone");
const btnRefreshMics = document.getElementById("btnRefreshMics");
const cfgMinChunkSeconds = document.getElementById("cfgMinChunkSeconds");
const cfgMaxChunkSeconds = document.getElementById("cfgMaxChunkSeconds");
const cfgAudioModel = document.getElementById("cfgAudioModel");
const cfgRewriteModel = document.getElementById("cfgRewriteModel");
const btnFetchModels = document.getElementById("btnFetchModels");
const btnFetchRewriteModels = document.getElementById("btnFetchRewriteModels");
const audioModelList = document.getElementById("audioModelList");
const rewriteModelList = document.getElementById("rewriteModelList");
const cfgHotkey = document.getElementById("cfgHotkey");
const btnResetHotkey = document.getElementById("btnResetHotkey");
const globalHotkeyHint = document.getElementById("globalHotkeyHint");
const cfgSoundNotification = document.getElementById("cfgSoundNotification");
const btnTestSound = document.getElementById("btnTestSound");
const chunksTracker = document.getElementById("chunksTracker");
const keyHint = document.getElementById("keyHint");

// Canvas Waveform
const canvas = document.getElementById("waveform");
const ctx = canvas.getContext("2d");

// ============================================================================
// Expansão / Retração Dinâmica da Janela
// ============================================================================
function setPanel(panelName) {
  if (activePanel === panelName) {
    // Se clicar no mesmo botão que já está aberto, fecha e volta para o modo compacto
    activePanel = null;
  } else {
    activePanel = panelName;
  }

  // Atualiza visibilidade dos painéis
  if (activePanel === "history") {
    panelHistory.style.display = "flex";
    panelSettings.style.display = "none";
    btnToggleHistory.classList.add("active");
    btnToggleSettings.classList.remove("active");
    resizeNativeWindow(360, 520);
    hideToast();
  } else if (activePanel === "settings") {
    panelHistory.style.display = "none";
    panelSettings.style.display = "flex";
    btnToggleHistory.classList.remove("active");
    btnToggleSettings.classList.add("active");
    resizeNativeWindow(360, 620);
    loadSettingsData();
  } else {
    panelHistory.style.display = "none";
    panelSettings.style.display = "none";
    btnToggleHistory.classList.remove("active");
    btnToggleSettings.classList.remove("active");
    resizeNativeWindow(360, 214);
  }
}

function resizeNativeWindow(w, h) {
  if (window.setWindowSize) {
    window.setWindowSize(w, h);
  }
}

btnToggleHistory.addEventListener("click", () => setPanel("history"));
btnToggleSettings.addEventListener("click", () => setPanel("settings"));
btnCloseHistory.addEventListener("click", () => setPanel(null));
btnCloseSettings.addEventListener("click", () => setPanel(null));
btnCancelSettings.addEventListener("click", () => setPanel(null));

// ============================================================================
// Atalho Tecla ENTER
// ============================================================================
document.addEventListener("keydown", (e) => {
  if (e.key === "Enter") {
    const activeEl = document.activeElement;
    const tag = activeEl ? activeEl.tagName.toLowerCase() : "";
    if (tag === "input" || tag === "select" || tag === "textarea") return;

    e.preventDefault();
    btnRecord.click();
  }
});

// ============================================================================
// Visualizador Waveform Canvas
// ============================================================================
function drawWaveform() {
  const width = canvas.width;
  const height = canvas.height;
  ctx.clearRect(0, 0, width, height);

  const numBars = 26;
  const barWidth = 4;
  const spacing = (width - numBars * barWidth) / (numBars + 1);
  const time = Date.now() * 0.005;

  for (let i = 0; i < numBars; i++) {
    const x = spacing + i * (barWidth + spacing);
    let barHeight = 3;

    if (isRecording) {
      const norm = Math.min(1.0, currentVolume);
      const centerFactor = Math.sin((i / (numBars - 1)) * Math.PI);
      const osc = 0.5 + 0.5 * Math.sin(time * 3.5 + i * 0.45);
      barHeight = 3 + (norm * centerFactor * osc * (height - 6));
      barHeight = Math.max(3, Math.min(height - 2, barHeight));

      const gradient = ctx.createLinearGradient(0, height - barHeight, 0, height);
      gradient.addColorStop(0, "#ef4444");
      gradient.addColorStop(1, "#f97316");
      ctx.fillStyle = gradient;
    } else {
      // Idle sutil
      const pulse = 2 + Math.sin(time * 0.8 + i * 0.2) * 1.5;
      barHeight = Math.max(2, pulse);
      ctx.fillStyle = "#1e293b";
    }

    const y = (height - barHeight) / 2;
    ctx.beginPath();
    ctx.roundRect(x, y, barWidth, barHeight, 2);
    ctx.fill();
  }

  animFrameId = requestAnimationFrame(drawWaveform);
}

// ============================================================================
// Gravação
// ============================================================================
btnRecord.addEventListener("click", async () => {
  if (!isRecording) {
    // Validação de API Key
    if (!currentConfig || !currentConfig.api_key || !currentConfig.api_key.trim()) {
      setPanel("settings");
      showApiKeyAlert();
      return;
    }

    if (window.startRecording) {
      const res = await window.startRecording();
      if (res && res.error) {
        if (res.error === "API_KEY_REQUIRED") {
          setPanel("settings");
          showApiKeyAlert();
        } else {
          alert("Aviso: " + (res.message || res.error));
        }
        return;
      }
    }
    setRecordingState(true);
  } else {
    // Para gravação física e envia para fila de transcrição
    setRecordingState(false);
    updateStatus("Processando áudio...", "processing");
    if (window.stopRecording) {
      window.stopRecording();
    }
  }
});

let chunksTrackerTimer = null;

function clearChunksTracker() {
  if (chunksTrackerTimer) {
    clearTimeout(chunksTrackerTimer);
    chunksTrackerTimer = null;
  }
  if (chunksTracker) {
    Array.from(chunksTracker.children).forEach(chip => {
      if (chip._doneTimer) clearTimeout(chip._doneTimer);
      if (chip._removeTimer) clearTimeout(chip._removeTimer);
    });
    chunksTracker.innerHTML = "";
    chunksTracker.style.display = "none";
  }
  if (keyHint) {
    keyHint.style.display = "block";
  }
}

window.onChunkStatus = function(index, state, label) {
  if (!chunksTracker) return;
  if (chunksTrackerTimer) {
    clearTimeout(chunksTrackerTimer);
    chunksTrackerTimer = null;
  }

  chunksTracker.style.display = "flex";
  if (keyHint) keyHint.style.display = "none";

  let chip = document.getElementById(`chunkChip_${index}`);
  if (!chip) {
    chip = document.createElement("div");
    chip.id = `chunkChip_${index}`;
    chip.className = `chunk-chip ${state}`;
    chip.innerHTML = `<span class="chip-dot"></span><span class="chip-label">Bloco ${index}: ${escapeHtml(label)}</span>`;
    chunksTracker.appendChild(chip);
  } else {
    chip.className = `chunk-chip ${state}`;
    const labelEl = chip.querySelector(".chip-label");
    if (labelEl) labelEl.innerText = `Bloco ${index}: ${label}`;
  }

  // Garante que o bloco atual ou recém-adicionado fique 100% visível na tela
  requestAnimationFrame(() => {
    chunksTracker.scrollTo({
      left: chunksTracker.scrollWidth,
      behavior: "smooth"
    });
    if (chip && typeof chip.scrollIntoView === "function") {
      chip.scrollIntoView({ behavior: "smooth", inline: "end", block: "nearest" });
    }
  });

  // Se o bloco foi concluído, inicia contagem de 3 segundos para remoção suave
  if (chip._doneTimer) {
    clearTimeout(chip._doneTimer);
    chip._doneTimer = null;
  }
  if (chip._removeTimer) {
    clearTimeout(chip._removeTimer);
    chip._removeTimer = null;
  }

  if (state === "done") {
    chip._doneTimer = setTimeout(() => {
      if (chip && chip.parentNode) {
        chip.classList.add("fade-out");
        chip._removeTimer = setTimeout(() => {
          if (chip && chip.parentNode) {
            chip.remove();
            if (chunksTracker && chunksTracker.children.length === 0 && !isRecording) {
              clearChunksTracker();
            }
          }
        }, 300);
      }
    }, 3000);
  }
};

function setRecordingState(recording) {
  isRecording = recording;
  if (recording) {
    clearChunksTracker();
    recordBtnContainer.classList.add("is-recording");
    btnRecord.title = "Finalizar Gravação (ENTER)";
    updateStatus("Gravando...", "recording");
    timerDisplay.innerText = "00:00";
    pauseNotice.style.display = "none";
  } else {
    recordBtnContainer.classList.remove("is-recording");
    btnRecord.title = "Iniciar Gravação (ENTER)";
    timerDisplay.innerText = "00:00";
    pauseNotice.style.display = "none";
  }
}

function updateStatus(text, type) {
  statusText.innerText = text;
  statusPill.className = "status-pill " + (type || "");
}

// Callbacks acionados pelo Go
window.onVolumeUpdate = function(vol, totalSec, isPauseWait) {
  currentVolume = vol;
  if (isRecording) {
    const m = Math.floor(totalSec / 60);
    const s = Math.floor(totalSec % 60);
    timerDisplay.innerText = `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`;
    pauseNotice.style.display = isPauseWait ? "inline" : "none";
  }
};

window.onStatusChange = function(status, type) {
  if (!isRecording) {
    updateStatus(status, type);
  }
};

// Disparado quando uma gravação termina o polimento e é copiada para o clipboard
window.onSessionFinished = function(finalText, timeStr) {
  if (!isRecording) {
    updateStatus("✓ Copiado!", "success");
    setTimeout(() => {
      if (!isRecording) updateStatus("Pronto", "");
    }, 4000);

    // Mantém os chips concluídos visíveis por 6s antes de restaurar a dica do teclado
    chunksTrackerTimer = setTimeout(() => {
      if (!isRecording) clearChunksTracker();
    }, 6000);
  }

  // Adiciona ao histórico da sessão
  sessionHistory.unshift({
    id: Date.now(),
    time: timeStr,
    text: finalText
  });

  // Atualiza a badge com animação pop
  historyBadge.classList.remove("bump");
  void historyBadge.offsetWidth; // Força reflow
  historyBadge.classList.add("bump");

  updateHistoryUI();
};

// ============================================================================
// Histórico / Gravações da Sessão
// ============================================================================
function updateHistoryUI() {
  const count = sessionHistory.length;
  historyBadge.innerText = count;
  historyPanelCount.innerText = `${count} ${count === 1 ? 'gravação' : 'gravações'}`;

  if (count === 0) {
    historyList.innerHTML = `
      <div class="empty-state">
        <div class="empty-icon">🎙️</div>
        <div>Nenhuma gravação realizada nesta sessão ainda.</div>
        <div class="empty-sub">Grave sua fala e o texto transcrito aparecerá aqui.</div>
      </div>
    `;
    return;
  }

  historyList.innerHTML = "";
  sessionHistory.forEach((item, index) => {
    const isLatest = index === 0;
    const card = document.createElement("div");
    card.className = `history-card ${isLatest ? 'latest' : ''}`;
    card.innerHTML = `
      <div class="history-card-header">
        <div class="history-meta">
          <span class="history-timestamp">🕒 ${item.time}</span>
          ${isLatest ? '<span class="latest-tag">Mais Recente</span>' : ''}
        </div>
        <div class="history-actions">
          <button class="sm-action-btn btn-copy-card" title="Copiar texto">📋 Copiar</button>
          <button class="sm-action-btn btn-save-card" title="Salvar em arquivo .txt">💾 Salvar</button>
        </div>
      </div>
      <div class="history-card-body">${escapeHtml(item.text)}</div>
    `;

    // Copiar card individual
    const copyBtn = card.querySelector(".btn-copy-card");
    copyBtn.addEventListener("click", async () => {
      if (window.copyToClipboard) {
        await window.copyToClipboard(item.text);
      } else {
        navigator.clipboard.writeText(item.text);
      }
      copyBtn.innerText = "✓ Copiado!";
      copyBtn.classList.add("success");
      setTimeout(() => {
        copyBtn.innerText = "📋 Copiar";
        copyBtn.classList.remove("success");
      }, 1500);
    });

    // Salvar card individual
    const saveBtn = card.querySelector(".btn-save-card");
    saveBtn.addEventListener("click", async () => {
      if (window.saveSingleRecording) {
        const res = await window.saveSingleRecording(item.text, item.time);
        if (res && res.success) {
          saveBtn.innerText = "✓";
          saveBtn.classList.add("success");
          showToast(`✓ Salvo em transcricoes/${res.filename}`);
          setTimeout(() => {
            saveBtn.innerText = "💾 Salvar";
            saveBtn.classList.remove("success");
          }, 2000);
        } else {
          alert("Erro ao salvar arquivo: " + (res && res.error));
        }
      }
    });

    historyList.appendChild(card);
  });
}

btnClearHistory.addEventListener("click", () => {
  sessionHistory = [];
  updateHistoryUI();
  hideToast();
});

btnSaveAllHistory.addEventListener("click", async () => {
  if (sessionHistory.length === 0) {
    alert("Não há gravações na sessão para salvar.");
    return;
  }
  let combined = "";
  sessionHistory.forEach((item, idx) => {
    combined += `-----------------------------------------------------\n` +
      `Gravação #${sessionHistory.length - idx} [${item.time}]\n` +
      `-----------------------------------------------------\n` +
      `${item.text}\n\n`;
  });

  if (window.saveSessionAll) {
    const res = await window.saveSessionAll(combined);
    if (res && res.success) {
      showToast(`✓ Sessão completa salva em transcricoes/${res.filename}`);
    } else {
      alert("Erro ao salvar sessão: " + (res && res.error));
    }
  }
});

function showToast(msg) {
  saveToast.innerText = msg;
  saveToast.style.display = "block";
  setTimeout(hideToast, 4000);
}

function hideToast() {
  saveToast.style.display = "none";
}

function escapeHtml(str) {
  return str.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}

// ============================================================================
// Configurações
// ============================================================================
async function loadSettingsData() {
  if (window.getConfig) {
    const jsonStr = await window.getConfig();
    currentConfig = JSON.parse(jsonStr);
    populateSettings(currentConfig);
  }
  await refreshMicrophones();
}

function showApiKeyAlert() {
  apiKeyAlert.style.display = "block";
  setTimeout(() => { cfgAPIKey.focus(); }, 150);
}

function populateSettings(cfg) {
  cfgProvider.value = (cfg.provider === "anthropic") ? "anthropic" : "openai";
  cfgBaseURL.value = cfg.base_url || "";
  cfgAPIKey.value = cfg.api_key || "";
  cfgAudioModel.value = cfg.last_audio_model || "google/gemini-2.5-flash";
  cfgRewriteModel.value = cfg.last_rewrite_model || "google/gemini-2.5-flash";
  if (cfgHotkey) {
    cfgHotkey.value = cfg.global_hotkey || "Ctrl + Alt + Win + R";
  }
  if (cfgMinChunkSeconds) {
    cfgMinChunkSeconds.value = cfg.min_chunk_seconds || 20;
  }
  if (cfgMaxChunkSeconds) {
    cfgMaxChunkSeconds.value = cfg.max_chunk_seconds || 35;
  }
  if (cfgSoundNotification) {
    cfgSoundNotification.checked = (cfg.sound_notification !== false);
  }

  updateProviderHints(cfgProvider.value);
}

if (btnTestSound) {
  btnTestSound.addEventListener("click", () => {
    if (window.playNotificationSound) {
      window.playNotificationSound();
    }
  });
}

// Interatividade do Campo de Atalho Global
let savedHotkeyBeforeFocus = "";

if (cfgHotkey) {
  cfgHotkey.addEventListener("focus", () => {
    savedHotkeyBeforeFocus = cfgHotkey.value;
    cfgHotkey.classList.add("recording-key");
    cfgHotkey.value = "Pressione as teclas...";
  });

  cfgHotkey.addEventListener("blur", () => {
    cfgHotkey.classList.remove("recording-key");
    if (!cfgHotkey.value || cfgHotkey.value === "Pressione as teclas..." || cfgHotkey.value.endsWith(" + ...")) {
      cfgHotkey.value = savedHotkeyBeforeFocus || "Ctrl + Alt + Win + R";
    }
  });

  cfgHotkey.addEventListener("keydown", (e) => {
    e.preventDefault();
    e.stopPropagation();

    // Cancelar com ESC
    if (e.key === "Escape") {
      cfgHotkey.value = savedHotkeyBeforeFocus || "Ctrl + Alt + Win + R";
      cfgHotkey.blur();
      return;
    }

    const mods = [];
    if (e.ctrlKey) mods.push("Ctrl");
    if (e.altKey) mods.push("Alt");
    if (e.shiftKey) mods.push("Shift");
    if (e.metaKey) mods.push("Win");

    const isModifierOnly = ["Control", "Alt", "Shift", "Meta"].includes(e.key);

    if (isModifierOnly) {
      if (mods.length > 0) {
        cfgHotkey.value = mods.join(" + ") + " + ...";
      }
      return;
    }

    // Identifica o nome da tecla
    let keyName = "";
    if (e.code && e.code.startsWith("Key")) {
      keyName = e.code.slice(3).toUpperCase();
    } else if (e.code && e.code.startsWith("Digit")) {
      keyName = e.code.slice(5);
    } else if (e.code && /^F\d+$/i.test(e.code)) {
      keyName = e.code.toUpperCase();
    } else if (e.code === "Space") {
      keyName = "Space";
    } else if (e.key && e.key.length === 1) {
      keyName = e.key.toUpperCase();
    } else {
      keyName = e.key;
    }

    // Garante ao menos um modificador (Ctrl por segurança) se nenhum foi pressionado
    if (mods.length === 0) {
      mods.push("Ctrl");
    }

    const finalHotkey = mods.join(" + ") + " + " + keyName;
    cfgHotkey.value = finalHotkey;
    savedHotkeyBeforeFocus = finalHotkey;
    cfgHotkey.blur();
  });
}

if (btnResetHotkey) {
  btnResetHotkey.addEventListener("click", () => {
    if (cfgHotkey) {
      cfgHotkey.value = "Ctrl + Alt + Win + R";
      savedHotkeyBeforeFocus = "Ctrl + Alt + Win + R";
    }
  });
}

cfgProvider.addEventListener("change", () => {
  updateProviderHints(cfgProvider.value);
});

function updateProviderHints(provider) {
  if (provider === "anthropic") {
    cfgBaseURL.placeholder = "Ex: https://api.anthropic.com";
    if (!cfgRewriteModel.value || cfgRewriteModel.value.includes("gemini")) {
      cfgRewriteModel.value = "claude-3-7-sonnet-20250219";
    }
  } else {
    cfgBaseURL.placeholder = "Ex: https://openrouter.ai/api/v1";
    if (!cfgAudioModel.value) {
      cfgAudioModel.value = "google/gemini-2.5-flash";
    }
    if (!cfgRewriteModel.value || cfgRewriteModel.value.includes("claude")) {
      cfgRewriteModel.value = "google/gemini-2.5-flash";
    }
  }
}

btnToggleKey.addEventListener("click", () => {
  cfgAPIKey.type = cfgAPIKey.type === "password" ? "text" : "password";
});

async function refreshMicrophones() {
  if (!window.listMicrophones) return;
  const listJson = await window.listMicrophones();
  const mics = JSON.parse(listJson);
  cfgMicrophone.innerHTML = '<option value="default">Microfone Padrão do Windows</option>';
  mics.forEach(m => {
    const opt = document.createElement("option");
    opt.value = m.name;
    opt.innerText = m.name + (m.is_default ? " (Padrão)" : "");
    if (currentConfig && currentConfig.selected_microphone === m.name) {
      opt.selected = true;
    }
    cfgMicrophone.appendChild(opt);
  });
}
btnRefreshMics.addEventListener("click", refreshMicrophones);

async function fetchModelsHandler(btn) {
  btn.innerText = "...";
  try {
    if (window.fetchModels) {
      const modelsJson = await window.fetchModels();
      const models = JSON.parse(modelsJson);
      audioModelList.innerHTML = "";
      rewriteModelList.innerHTML = "";
      models.forEach(m => {
        const opt1 = document.createElement("option");
        opt1.value = m;
        audioModelList.appendChild(opt1);
        const opt2 = document.createElement("option");
        opt2.value = m;
        rewriteModelList.appendChild(opt2);
      });
      alert(`Encontrados ${models.length} modelos.`);
    }
  } catch(e) {
    alert("Erro ao buscar modelos: " + e);
  } finally {
    btn.innerText = btn === btnFetchModels ? "🔍 Buscar" : "🔍 Modelos";
  }
}

btnFetchModels.addEventListener("click", () => fetchModelsHandler(btnFetchModels));
if (btnFetchRewriteModels) {
  btnFetchRewriteModels.addEventListener("click", () => fetchModelsHandler(btnFetchRewriteModels));
}

btnSaveSettings.addEventListener("click", async () => {
  const minChunk = parseInt(cfgMinChunkSeconds ? cfgMinChunkSeconds.value : "20", 10) || 20;
  let maxChunk = parseInt(cfgMaxChunkSeconds ? cfgMaxChunkSeconds.value : "35", 10) || 35;
  if (maxChunk <= minChunk) {
    maxChunk = minChunk + 5;
    if (cfgMaxChunkSeconds) cfgMaxChunkSeconds.value = maxChunk;
  }

  const newCfg = {
    provider: cfgProvider.value,
    base_url: cfgBaseURL.value.trim(),
    api_key: cfgAPIKey.value.trim(),
    selected_microphone: cfgMicrophone.value,
    last_audio_model: cfgAudioModel.value.trim(),
    last_rewrite_model: cfgRewriteModel.value.trim(),
    min_chunk_seconds: minChunk,
    max_chunk_seconds: maxChunk,
    silence_pause_seconds: (currentConfig && currentConfig.silence_pause_seconds) || 0.45,
    silence_threshold: (currentConfig && currentConfig.silence_threshold) || 0.008,
    sample_rate: (currentConfig && currentConfig.sample_rate) || 16000,
    global_hotkey: (cfgHotkey && cfgHotkey.value.trim()) || "Ctrl + Alt + Win + R",
    sound_notification: cfgSoundNotification ? cfgSoundNotification.checked : true
  };

  if (window.saveConfig) {
    await window.saveConfig(JSON.stringify(newCfg));
  }
  currentConfig = newCfg;
  if (globalHotkeyHint) {
    globalHotkeyHint.innerText = newCfg.global_hotkey;
  }
  apiKeyAlert.style.display = "none";
  setPanel(null); // Volta ao modo compacto com sucesso!
});

// ============================================================================
// Inicialização
// ============================================================================
window.addEventListener("DOMContentLoaded", async () => {
  drawWaveform();
  if (window.getConfig) {
    const jsonStr = await window.getConfig();
    currentConfig = JSON.parse(jsonStr);
    if (currentConfig && currentConfig.global_hotkey && globalHotkeyHint) {
      globalHotkeyHint.innerText = currentConfig.global_hotkey;
    }
  }
  if (window.getAppInfo) {
    const info = JSON.parse(await window.getAppInfo());
    if (info.build && buildBadge) {
      buildBadge.innerText = `b${info.build}`;
    }
  }
});

