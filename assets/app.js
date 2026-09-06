// WhisperGo UI Logic

let isRecording = false;
let currentConfig = null;
let currentVolume = 0;
let animFrameId = null;
let chunks = [];

// Elementos DOM
const btnRecord = document.getElementById("btnRecord");
const statusPill = document.getElementById("statusPill");
const statusText = document.getElementById("statusText");
const timerDisplay = document.getElementById("timerDisplay");
const chunkThresholdDisplay = document.getElementById("chunkThresholdDisplay");
const pauseNotice = document.getElementById("pauseNotice");
const micNameDisplay = document.getElementById("micNameDisplay");
const finalText = document.getElementById("finalText");
const fileHint = document.getElementById("fileHint");
const btnCopy = document.getElementById("btnCopy");
const btnClear = document.getElementById("btnClear");
const chunksToggle = document.getElementById("chunksToggle");
const chunksList = document.getElementById("chunksList");
const chunksCount = document.getElementById("chunksCount");
const chunksChevron = document.getElementById("chunksChevron");
const buildBadge = document.getElementById("buildBadge");

// Modal
const btnSettings = document.getElementById("btnSettings");
const settingsModal = document.getElementById("settingsModal");
const btnCloseModal = document.getElementById("btnCloseModal");
const btnCancelSettings = document.getElementById("btnCancelSettings");
const btnSaveSettings = document.getElementById("btnSaveSettings");
const cfgProvider = document.getElementById("cfgProvider");
const cfgBaseURL = document.getElementById("cfgBaseURL");
const cfgAPIKey = document.getElementById("cfgAPIKey");
const btnToggleKey = document.getElementById("btnToggleKey");
const cfgMicrophone = document.getElementById("cfgMicrophone");
const btnRefreshMics = document.getElementById("btnRefreshMics");
const cfgAudioModel = document.getElementById("cfgAudioModel");
const cfgRewriteModel = document.getElementById("cfgRewriteModel");
const btnFetchModels = document.getElementById("btnFetchModels");
const cfgMinChunk = document.getElementById("cfgMinChunk");
const cfgMaxChunk = document.getElementById("cfgMaxChunk");
const audioModelList = document.getElementById("audioModelList");
const rewriteModelList = document.getElementById("rewriteModelList");

// Canvas de Áudio
const canvas = document.getElementById("waveform");
const ctx = canvas.getContext("2d");

// Animação de Onda Contínua
function drawWaveform() {
  const width = canvas.width;
  const height = canvas.height;
  ctx.clearRect(0, 0, width, height);

  const numBars = 32;
  const barWidth = 6;
  const spacing = (width - numBars * barWidth) / (numBars + 1);
  const time = Date.now() * 0.005;

  for (let i = 0; i < numBars; i++) {
    const x = spacing + i * (barWidth + spacing);
    let barHeight = 4;

    if (isRecording) {
      const norm = Math.min(1.0, currentVolume);
      const centerFactor = Math.sin((i / (numBars - 1)) * Math.PI);
      const osc = 0.5 + 0.5 * Math.sin(time * 3 + i * 0.4);
      barHeight = 4 + (norm * centerFactor * osc * (height - 8));
      barHeight = Math.max(4, Math.min(height - 6, barHeight));

      const gradient = ctx.createLinearGradient(0, height - barHeight, 0, height);
      gradient.addColorStop(0, "#ef4444");
      gradient.addColorStop(1, "#f97316");
      ctx.fillStyle = gradient;
    } else {
      // Idle pulse sutil
      const pulse = 2 + Math.sin(time + i * 0.2) * 1.5;
      barHeight = Math.max(3, pulse);
      ctx.fillStyle = "#334155";
    }

    const y = (height - barHeight) / 2;
    ctx.beginPath();
    ctx.roundRect(x, y, barWidth, barHeight, 3);
    ctx.fill();
  }

  animFrameId = requestAnimationFrame(drawWaveform);
}

// Alterna gravação
btnRecord.addEventListener("click", async () => {
  if (!isRecording) {
    if (window.startRecording) {
      const res = await window.startRecording();
      if (res && res.error) {
        alert("Erro ao iniciar gravação: " + res.error);
        return;
      }
    }
    setRecordingState(true);
  } else {
    setRecordingState(false);
    updateStatus("Encerrando e polindo...", "processing");
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
    chunks = [];
    renderChunks();
  } else {
    btnRecord.className = "btn-primary btn-record";
    btnRecord.innerHTML = "<span>▶</span> Iniciar Gravação";
  }
}

function updateStatus(text, type) {
  statusText.innerText = text;
  statusPill.className = "status-pill " + (type || "");
}

// Callbacks chamados pelo backend Go
window.onVolumeUpdate = function(vol, sec, isPauseWait) {
  currentVolume = vol;
  const m = Math.floor(sec / 60);
  const s = Math.floor(sec % 60);
  timerDisplay.innerText = `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`;
  
  if (isPauseWait) {
    pauseNotice.style.display = "inline";
  } else {
    pauseNotice.style.display = "none";
  }
};

window.onChunkTranscribed = function(chunkIndex, text) {
  if (text && text.trim()) {
    chunks.push({ index: chunkIndex, text: text.trim() });
    renderChunks();
  }
};

window.onStatusChange = function(status, type) {
  updateStatus(status, type);
};

window.onFinalTextReady = function(rawText, polishedText) {
  finalText.value = polishedText;
  fileHint.style.display = "flex";
  updateStatus("Concluído (Copiado)", "success");
  setRecordingState(false);
};

function renderChunks() {
  chunksCount.innerText = chunks.length;
  chunksList.innerHTML = "";
  chunks.forEach(c => {
    const item = document.createElement("div");
    item.className = "chunk-item";
    item.innerHTML = `
      <div class="chunk-meta">Bloco ${c.index}</div>
      <div class="chunk-text">"${escapeHtml(c.text)}"</div>
    `;
    chunksList.appendChild(item);
  });
  if (chunks.length > 0) {
    chunksList.scrollTop = chunksList.scrollHeight;
  }
}

function escapeHtml(str) {
  return str.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}

// Accordion de blocos
chunksToggle.addEventListener("click", () => {
  const isHidden = chunksList.style.display === "none";
  chunksList.style.display = isHidden ? "flex" : "none";
  chunksChevron.innerText = isHidden ? "▲" : "▼";
});

// Copiar texto
btnCopy.addEventListener("click", async () => {
  const text = finalText.value;
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
  }, 1800);
});

// Limpar
btnClear.addEventListener("click", () => {
  finalText.value = "";
  fileHint.style.display = "none";
  chunks = [];
  renderChunks();
  updateStatus("Pronto", "");
});

// Toggle senha API Key
btnToggleKey.addEventListener("click", () => {
  cfgAPIKey.type = cfgAPIKey.type === "password" ? "text" : "password";
});

// Modal de Configurações
btnSettings.addEventListener("click", () => {
  openSettingsModal();
});
btnCloseModal.addEventListener("click", () => {
  settingsModal.classList.remove("active");
});
btnCancelSettings.addEventListener("click", () => {
  settingsModal.classList.remove("active");
});

async function openSettingsModal() {
  if (window.getConfig) {
    const jsonStr = await window.getConfig();
    currentConfig = JSON.parse(jsonStr);
    populateSettings(currentConfig);
  }
  await refreshMicrophones();
  settingsModal.classList.add("active");
}

function populateSettings(cfg) {
  cfgProvider.value = cfg.provider || "openrouter";
  cfgBaseURL.value = cfg.base_url || "https://openrouter.ai/api/v1";
  cfgAPIKey.value = cfg.api_key || "";
  cfgAudioModel.value = cfg.last_audio_model || "google/gemini-2.5-flash";
  cfgRewriteModel.value = cfg.last_rewrite_model || "google/gemini-2.5-flash";
  cfgMinChunk.value = cfg.min_chunk_seconds || 20;
  cfgMaxChunk.value = cfg.max_chunk_seconds || 35;
  chunkThresholdDisplay.innerText = ` / ${cfg.min_chunk_seconds || 20}s`;
}

cfgProvider.addEventListener("change", () => {
  const p = cfgProvider.value;
  if (p === "openrouter") {
    cfgBaseURL.value = "https://openrouter.ai/api/v1";
    if (!cfgAudioModel.value || cfgAudioModel.value.includes("high")) {
      cfgAudioModel.value = "google/gemini-2.5-flash";
      cfgRewriteModel.value = "google/gemini-2.5-flash";
    }
  } else if (p === "proxyai") {
    cfgBaseURL.value = "https://proxyai.targetfollow.pro/v1";
    cfgAudioModel.value = "gemini-3.7-flash-high";
    cfgRewriteModel.value = "gemini-3.7-flash-high";
  }
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
  btnFetchModels.innerText = "Buscando...";
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
    btnFetchModels.innerText = "🔍 Buscar Modelos";
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
    min_chunk_seconds: parseInt(cfgMinChunk.value) || 20,
    max_chunk_seconds: parseInt(cfgMaxChunk.value) || 35,
    silence_pause_seconds: (currentConfig && currentConfig.silence_pause_seconds) || 0.45,
    silence_threshold: (currentConfig && currentConfig.silence_threshold) || 0.012,
    sample_rate: (currentConfig && currentConfig.sample_rate) || 16000
  };

  if (window.saveConfig) {
    await window.saveConfig(JSON.stringify(newCfg));
  }
  currentConfig = newCfg;
  chunkThresholdDisplay.innerText = ` / ${newCfg.min_chunk_seconds}s`;
  micNameDisplay.innerText = "Microfone: " + (newCfg.selected_microphone === "default" ? "Padrão do Sistema" : newCfg.selected_microphone);
  settingsModal.classList.remove("active");
});

// Inicialização
window.addEventListener("DOMContentLoaded", async () => {
  drawWaveform();
  if (window.getConfig) {
    const jsonStr = await window.getConfig();
    currentConfig = JSON.parse(jsonStr);
    micNameDisplay.innerText = "Microfone: " + (currentConfig.selected_microphone === "default" ? "Padrão do Sistema" : currentConfig.selected_microphone);
    chunkThresholdDisplay.innerText = ` / ${currentConfig.min_chunk_seconds || 20}s`;
  }
  if (window.getAppInfo) {
    const info = JSON.parse(await window.getAppInfo());
    if (info.build) {
      buildBadge.innerText = `b${info.build}`;
    }
  }
});
