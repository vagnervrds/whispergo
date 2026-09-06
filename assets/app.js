// WhisperGo UI Logic - Ultra Compact & Polished

let isRecording = false;
let currentConfig = null;
let currentVolume = 0;
let animFrameId = null;
let sessionHistory = [];

// Elementos DOM
const btnRecord = document.getElementById("btnRecord");
const statusPill = document.getElementById("statusPill");
const statusText = document.getElementById("statusText");
const timerDisplay = document.getElementById("timerDisplay");
const chunkThresholdDisplay = document.getElementById("chunkThresholdDisplay");
const pauseNotice = document.getElementById("pauseNotice");
const buildBadge = document.getElementById("buildBadge");
const resultBox = document.getElementById("resultBox");
const finalTextPreview = document.getElementById("finalTextPreview");
const btnCopy = document.getElementById("btnCopy");

// Histórico
const btnHistory = document.getElementById("btnHistory");
const historyBadge = document.getElementById("historyBadge");
const historyModal = document.getElementById("historyModal");
const btnCloseHistory = document.getElementById("btnCloseHistory");
const btnCloseHistoryBtn = document.getElementById("btnCloseHistoryBtn");
const btnClearHistory = document.getElementById("btnClearHistory");
const historyList = document.getElementById("historyList");

// Configurações Modal
const btnSettings = document.getElementById("btnSettings");
const settingsModal = document.getElementById("settingsModal");
const btnCloseModal = document.getElementById("btnCloseModal");
const btnCancelSettings = document.getElementById("btnCancelSettings");
const btnSaveSettings = document.getElementById("btnSaveSettings");
const apiKeyAlert = document.getElementById("apiKeyAlert");
const cfgProvider = document.getElementById("cfgProvider");
const cfgBaseURL = document.getElementById("cfgBaseURL");
const cfgAPIKey = document.getElementById("cfgAPIKey");
const btnToggleKey = document.getElementById("btnToggleKey");
const cfgMicrophone = document.getElementById("cfgMicrophone");
const btnRefreshMics = document.getElementById("btnRefreshMics");
const cfgAudioModel = document.getElementById("cfgAudioModel");
const cfgRewriteModel = document.getElementById("cfgRewriteModel");
const btnFetchModels = document.getElementById("btnFetchModels");
const audioModelList = document.getElementById("audioModelList");
const rewriteModelList = document.getElementById("rewriteModelList");

// Canvas
const canvas = document.getElementById("waveform");
const ctx = canvas.getContext("2d");

// Atalho da Tecla ENTER para Iniciar / Parar Gravação
document.addEventListener("keydown", (e) => {
  if (e.key === "Enter") {
    const isSettingsOpen = settingsModal.classList.contains("active");
    const isHistoryOpen = historyModal.classList.contains("active");
    if (isSettingsOpen || isHistoryOpen) return;

    const activeEl = document.activeElement;
    const tag = activeEl ? activeEl.tagName.toLowerCase() : "";
    if (tag === "input" || tag === "textarea") return;

    e.preventDefault();
    btnRecord.click();
  }
});

// Desenho da Forma de Onda Fluida
function drawWaveform() {
  const width = canvas.width;
  const height = canvas.height;
  ctx.clearRect(0, 0, width, height);

  const numBars = 24;
  const barWidth = 6;
  const spacing = (width - numBars * barWidth) / (numBars + 1);
  const time = Date.now() * 0.005;

  for (let i = 0; i < numBars; i++) {
    const x = spacing + i * (barWidth + spacing);
    let barHeight = 4;

    if (isRecording) {
      const norm = Math.min(1.0, currentVolume);
      const centerFactor = Math.sin((i / (numBars - 1)) * Math.PI);
      const osc = 0.5 + 0.5 * Math.sin(time * 3.5 + i * 0.5);
      barHeight = 4 + (norm * centerFactor * osc * (height - 8));
      barHeight = Math.max(4, Math.min(height - 4, barHeight));

      const gradient = ctx.createLinearGradient(0, height - barHeight, 0, height);
      gradient.addColorStop(0, "#ef4444");
      gradient.addColorStop(1, "#f97316");
      ctx.fillStyle = gradient;
    } else {
      // Idle pulse sutil e elegante
      const pulse = 2 + Math.sin(time + i * 0.25) * 1.5;
      barHeight = Math.max(3, pulse);
      ctx.fillStyle = "#25334d";
    }

    const y = (height - barHeight) / 2;
    ctx.beginPath();
    ctx.roundRect(x, y, barWidth, barHeight, 3);
    ctx.fill();
  }

  animFrameId = requestAnimationFrame(drawWaveform);
}

// Botão Iniciar / Parar
btnRecord.addEventListener("click", async () => {
  if (!isRecording) {
    // Verificação de API Key antes de começar
    if (!currentConfig || !currentConfig.api_key || !currentConfig.api_key.trim()) {
      openSettingsModal(true);
      return;
    }

    if (window.startRecording) {
      const res = await window.startRecording();
      if (res && res.error) {
        if (res.error === "API_KEY_REQUIRED") {
          openSettingsModal(true);
        } else {
          alert("Erro: " + (res.message || res.error));
        }
        return;
      }
    }
    setRecordingState(true);
  } else {
    setRecordingState(false);
    updateStatus("Polindo texto...", "processing");
    if (window.stopRecording) {
      await window.stopRecording();
    }
  }
});

function setRecordingState(recording) {
  isRecording = recording;
  if (recording) {
    btnRecord.className = "btn-primary btn-stop";
    btnRecord.innerHTML = "<span>⏹</span> Finalizar e Polir";
    updateStatus("Gravando...", "recording");
    timerDisplay.innerText = "00:00";
  } else {
    btnRecord.className = "btn-primary btn-record";
    btnRecord.innerHTML = "<span>▶</span> Iniciar Gravação";
  }
}

function updateStatus(text, type) {
  statusText.innerText = text;
  statusPill.className = "status-pill " + (type || "");
}

// Callbacks do Backend Go
window.onVolumeUpdate = function(vol, sec, isPauseWait) {
  currentVolume = vol;
  const m = Math.floor(sec / 60);
  const s = Math.floor(sec % 60);
  timerDisplay.innerText = `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`;
  pauseNotice.style.display = isPauseWait ? "inline" : "none";
};

window.onChunkTranscribed = function(chunkIndex, text) {
  if (text && text.trim()) {
    updateStatus(`Bloco ${chunkIndex} transcrito`, "processing");
  }
};

window.onStatusChange = function(status, type) {
  updateStatus(status, type);
};

window.onFinalTextReady = function(rawText, polishedText) {
  finalTextPreview.innerText = polishedText;
  resultBox.style.display = "flex";
  updateStatus("Copiado!", "success");
  setRecordingState(false);

  // Adiciona ao Histórico da Sessão
  sessionHistory.unshift({
    id: Date.now(),
    time: new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' }),
    text: polishedText
  });
  updateHistoryUI();
};

// Copiar texto mais recente
btnCopy.addEventListener("click", async () => {
  const text = finalTextPreview.innerText;
  if (!text) return;
  if (window.copyToClipboard) {
    await window.copyToClipboard(text);
  } else {
    navigator.clipboard.writeText(text);
  }
  btnCopy.innerText = "✓ Copiado!";
  btnCopy.classList.add("success");
  setTimeout(() => {
    btnCopy.innerText = "📋 Copiar";
    btnCopy.classList.remove("success");
  }, 1600);
});

// Modal de Histórico da Sessão
btnHistory.addEventListener("click", () => {
  historyModal.classList.add("active");
});
btnCloseHistory.addEventListener("click", () => {
  historyModal.classList.remove("active");
});
btnCloseHistoryBtn.addEventListener("click", () => {
  historyModal.classList.remove("active");
});
btnClearHistory.addEventListener("click", () => {
  sessionHistory = [];
  updateHistoryUI();
});

function updateHistoryUI() {
  historyBadge.innerText = sessionHistory.length;
  if (sessionHistory.length === 0) {
    historyList.innerHTML = '<div class="empty-history">Nenhuma gravação realizada nesta sessão ainda.</div>';
    return;
  }

  historyList.innerHTML = "";
  sessionHistory.forEach(item => {
    const el = document.createElement("div");
    el.className = "history-item";
    el.innerHTML = `
      <div class="history-item-top">
        <span class="history-time">🕒 ${item.time}</span>
        <button class="sm-btn btn-copy-hist" data-id="${item.id}">📋 Copiar</button>
      </div>
      <div class="history-text">${escapeHtml(item.text)}</div>
    `;

    const copyBtn = el.querySelector(".btn-copy-hist");
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
      }, 1600);
    });

    historyList.appendChild(el);
  });
}

function escapeHtml(str) {
  return str.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}

// Modal de Configurações
btnSettings.addEventListener("click", () => {
  openSettingsModal(false);
});
btnCloseModal.addEventListener("click", () => {
  settingsModal.classList.remove("active");
});
btnCancelSettings.addEventListener("click", () => {
  settingsModal.classList.remove("active");
});

async function openSettingsModal(showApiKeyAlert) {
  if (window.getConfig) {
    const jsonStr = await window.getConfig();
    currentConfig = JSON.parse(jsonStr);
    populateSettings(currentConfig);
  }
  await refreshMicrophones();

  if (showApiKeyAlert) {
    apiKeyAlert.style.display = "block";
    setTimeout(() => { cfgAPIKey.focus(); }, 150);
  } else {
    apiKeyAlert.style.display = "none";
  }

  settingsModal.classList.add("active");
}

function populateSettings(cfg) {
  cfgProvider.value = (cfg.provider === "anthropic") ? "anthropic" : "openai";
  cfgBaseURL.value = cfg.base_url || "";
  cfgAPIKey.value = cfg.api_key || "";
  cfgAudioModel.value = cfg.last_audio_model || "google/gemini-2.5-flash";
  cfgRewriteModel.value = cfg.last_rewrite_model || "google/gemini-2.5-flash";
  chunkThresholdDisplay.innerText = ` / ${cfg.min_chunk_seconds || 20}s`;

  updateProviderHints(cfgProvider.value);
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
  cfgMicrophone.innerHTML = '<option value="default">Microfone Padrão do Sistema</option>';
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

btnFetchModels.addEventListener("click", async () => {
  btnFetchModels.innerText = "...";
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
    btnFetchModels.innerText = "🔍 Buscar";
  }
});

btnSaveSettings.addEventListener("click", async () => {
  const newCfg = {
    provider: cfgProvider.value,
    base_url: cfgBaseURL.value.trim(),
    api_key: cfgAPIKey.value.trim(),
    selected_microphone: cfgMicrophone.value,
    last_audio_model: cfgAudioModel.value.trim(),
    last_rewrite_model: cfgRewriteModel.value.trim(),
    min_chunk_seconds: (currentConfig && currentConfig.min_chunk_seconds) || 20,
    max_chunk_seconds: (currentConfig && currentConfig.max_chunk_seconds) || 35,
    silence_pause_seconds: (currentConfig && currentConfig.silence_pause_seconds) || 0.45,
    silence_threshold: (currentConfig && currentConfig.silence_threshold) || 0.008,
    sample_rate: (currentConfig && currentConfig.sample_rate) || 16000
  };

  if (window.saveConfig) {
    await window.saveConfig(JSON.stringify(newCfg));
  }
  currentConfig = newCfg;
  settingsModal.classList.remove("active");
  apiKeyAlert.style.display = "none";
});

// Inicialização
window.addEventListener("DOMContentLoaded", async () => {
  drawWaveform();
  if (window.getConfig) {
    const jsonStr = await window.getConfig();
    currentConfig = JSON.parse(jsonStr);
    chunkThresholdDisplay.innerText = ` / ${currentConfig.min_chunk_seconds || 20}s`;
  }
  if (window.getAppInfo) {
    const info = JSON.parse(await window.getAppInfo());
    if (info.build) {
      buildBadge.innerText = `b${info.build}`;
    }
  }
});
