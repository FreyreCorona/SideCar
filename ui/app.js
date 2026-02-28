// ── State ────────────────────────────────────────────────────
const state = {
  daemonRunning: false,
  connected: false,
  currentView: 0,
  totalViews: 3,
  selectedFile: null,
  brightness: 100,
};

// ── DOM refs ─────────────────────────────────────────────────
const statusDot    = document.getElementById('statusDot');
const statusLabel  = document.getElementById('statusLabel');
const devicePill   = document.getElementById('devicePill');
const daemonBtn    = document.getElementById('daemonBtn');
const pageLabel    = document.getElementById('pageLabel');
const canvas       = document.getElementById('previewCanvas');
const ctx          = canvas ? canvas.getContext('2d') : null;

// ── Tab switching ─────────────────────────────────────────────
document.querySelectorAll('.nav-item').forEach(item => {
  item.addEventListener('click', () => {
    document.querySelectorAll('.nav-item').forEach(n => n.classList.remove('active'));
    document.querySelectorAll('.tab-panel').forEach(p => p.classList.remove('active'));
    item.classList.add('active');
    document.getElementById('tab-' + item.dataset.tab).classList.add('active');
  });
});

// ── Device status ─────────────────────────────────────────────
function setConnected(ok) {
  state.connected = ok;
  statusDot.className = 'dot' + (ok ? ' active' : '');
  statusLabel.textContent = ok ? 'Connected' : 'Disconnected';
  devicePill.className = 'device-pill' + (ok ? ' connected' : '');
}

// ── Daemon ────────────────────────────────────────────────────
daemonBtn.addEventListener('click', async () => {
  if (!state.daemonRunning) {
    const ok = await callGo('startDaemon');
    if (ok) {
      state.daemonRunning = true;
      daemonBtn.textContent = '■ Stop Daemon';
      daemonBtn.classList.add('running');
      setConnected(true);
      startStatsLoop();
    } else {
      appendLog('Failed to start daemon', true);
    }
  } else {
    await callGo('stopDaemon');
    state.daemonRunning = false;
    daemonBtn.textContent = '▶ Start Daemon';
    daemonBtn.classList.remove('running');
    setConnected(false);
    stopStatsLoop();
  }
});

// ── View navigation ───────────────────────────────────────────
document.getElementById('nextViewBtn').addEventListener('click', async () => {
  await callGo('nextView');
  state.currentView = (state.currentView + 1) % state.totalViews;
  updatePageLabel();
  renderCurrentView();
});
document.getElementById('prevViewBtn').addEventListener('click', async () => {
  state.currentView = (state.currentView - 1 + state.totalViews) % state.totalViews;
  await callGo('setView', state.currentView);
  updatePageLabel();
  renderCurrentView();
});

function updatePageLabel() {
  const names = ['CPU & RAM', 'Network', 'Power'];
  pageLabel.textContent = names[state.currentView] || `View ${state.currentView + 1}`;
}

// called from Go when view changes externally
window.onViewChanged = function() {
  renderCurrentView();
};

// ── Canvas preview ────────────────────────────────────────────
async function renderCurrentView() {
  if (!ctx) return;
  try {
    const frame = await window.getCurrentFrame();
    ctx.fillStyle = '#000';
    ctx.fillRect(0, 0, 240, 240);
    if (frame && frame.texts) renderTextFrame(ctx, canvas, frame);
    if (frame && frame.images) await renderImages(ctx, frame);
  } catch (e) {
    // silent - device may not be connected
  }
}

// ── Stats loop ────────────────────────────────────────────────
let statsInterval = null;
function startStatsLoop() {
  renderCurrentView();
  updateStats();
  statsInterval = setInterval(updateStats, 3000);
}
function stopStatsLoop() {
  clearInterval(statsInterval);
  statsInterval = null;
}

async function updateStats() {
  try {
    const stats = await callGo('getStats');
    if (!stats) return;
    setBar('barCPU', stats.cpu);
    setText('valCPU', fmt(stats.cpu, '%'));
    setBar('barRAM', stats.ramPct);
    setText('valRAM', fmt(stats.ramPct, '%'));
    setBar('barBAT', stats.battery);
    setText('valBAT', fmt(stats.battery, '%'));
    setText('valNET', `${stats.netIface} ↑${fmtBytes(stats.txBytes)}/s ↓${fmtBytes(stats.rxBytes)}/s`);
    setText('valTEMP', stats.temp > 0 ? fmt(stats.temp, '°C', 1) : '—');
    setText('valUPTIME', fmtUptime(stats.uptime));
  } catch (e) {}
}

function setBar(id, pct) {
  const el = document.getElementById(id);
  if (el) el.style.width = Math.min(100, Math.max(0, pct)) + '%';
}
function setText(id, val) {
  const el = document.getElementById(id);
  if (el) el.textContent = val;
}
function fmt(v, unit, dec = 0) { return (v || 0).toFixed(dec) + unit; }
function fmtBytes(b) {
  if (!b) return '0B';
  if (b < 1024) return b + 'B';
  if (b < 1024 * 1024) return (b / 1024).toFixed(1) + 'K';
  return (b / 1024 / 1024).toFixed(1) + 'M';
}
function fmtUptime(s) {
  if (!s) return '—';
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  return `${h}h ${m}m`;
}

// ── Brightness ────────────────────────────────────────────────
const slider = document.getElementById('brightnessSlider');
const bVal   = document.getElementById('brightnessVal');
let brightnessTimer = null;
slider.addEventListener('input', () => {
  bVal.textContent = slider.value;
  clearTimeout(brightnessTimer);
  brightnessTimer = setTimeout(() => {
    callGo('setBrightness', parseInt(slider.value));
  }, 250);
});

// ── Upload ────────────────────────────────────────────────────
const uploadZone  = document.getElementById('uploadZone');
const fileInput   = document.getElementById('fileInput');
const uploadBtn   = document.getElementById('uploadBtn');
const uploadMeta  = document.getElementById('uploadMeta');
const progressWrap = document.getElementById('progressWrap');
const progressFill = document.getElementById('progressFill');
const progressPct  = document.getElementById('progressPct');
const uploadLog    = document.getElementById('uploadLog');

uploadZone.addEventListener('click', () => fileInput.click());
uploadZone.addEventListener('dragover', e => { e.preventDefault(); uploadZone.classList.add('drag-over'); });
uploadZone.addEventListener('dragleave', () => uploadZone.classList.remove('drag-over'));
uploadZone.addEventListener('drop', e => {
  e.preventDefault();
  uploadZone.classList.remove('drag-over');
  const file = e.dataTransfer.files[0];
  if (file) selectFile(file);
});

fileInput.addEventListener('change', () => {
  if (fileInput.files[0]) selectFile(fileInput.files[0]);
});

function selectFile(file) {
  state.selectedFile = file;
  document.getElementById('uploadFileName').textContent = file.name;
  document.getElementById('uploadFileSize').textContent = fmtFileSize(file.size);
  uploadMeta.style.display = 'flex';
  uploadBtn.disabled = false;
  appendLog(`Selected: ${file.name} (${fmtFileSize(file.size)})`);
}

function fmtFileSize(bytes) {
  if (bytes < 1024) return bytes + ' B';
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
  return (bytes / 1024 / 1024).toFixed(2) + ' MB';
}

uploadBtn.addEventListener('click', async () => {
  if (!state.selectedFile) return;
  const fileType = document.getElementById('fileTypeSelect').value;

  uploadBtn.disabled = true;
  progressWrap.style.display = 'flex';
  setProgress(0);
  appendLog(`Uploading ${state.selectedFile.name} as [${fileType}]...`);

  try {
    const buf = await state.selectedFile.arrayBuffer();
    const arr = Array.from(new Uint8Array(buf));

    // Use the Go upload binding which handles progress internally
    const result = await callGo('uploadFile', arr, fileType);
    if (result && result.error) {
      appendLog(`Error: ${result.error}`, true);
    } else {
      setProgress(100);
      appendLog('Upload complete ✓');
    }
  } catch (e) {
    appendLog(`Error: ${e}`, true);
  } finally {
    uploadBtn.disabled = false;
  }
});

// Progress is pushed from Go via onUploadProgress
window.onUploadProgress = function(pct) {
  setProgress(pct);
};

function setProgress(pct) {
  progressFill.style.width = pct + '%';
  progressPct.textContent = pct + '%';
}

function appendLog(msg, isErr = false) {
  const line = document.createElement('div');
  line.textContent = `[${new Date().toLocaleTimeString()}] ${msg}`;
  if (isErr) line.style.color = 'var(--red)';
  uploadLog.appendChild(line);
  uploadLog.scrollTop = uploadLog.scrollHeight;
}

// ── Settings ──────────────────────────────────────────────────
document.getElementById('wakeBtn').addEventListener('click', () => callGo('wake'));
document.getElementById('sleepBtn').addEventListener('click', () => callGo('sleep'));
document.getElementById('rebootBtn').addEventListener('click', async () => {
  if (confirm('Reboot the device?')) callGo('reboot');
});
document.getElementById('setPageBtn').addEventListener('click', () => {
  const page = parseInt(document.getElementById('pageInput').value);
  callGo('showPage', page);
});
document.getElementById('refreshPortsBtn').addEventListener('click', async () => {
  const ports = await callGo('listPorts');
  const sel = document.getElementById('portSelect');
  sel.innerHTML = '<option value="auto">Auto-detect</option>';
  (ports || []).forEach(p => {
    const opt = document.createElement('option');
    opt.value = p;
    opt.textContent = p;
    sel.appendChild(opt);
  });
});

// ── callGo helper ─────────────────────────────────────────────
// Wraps the webview.Bind() bindings exposed from Go.
// If a binding doesn't exist yet, it returns null gracefully.
async function callGo(name, ...args) {
  if (typeof window[name] === 'function') {
    try {
      return await window[name](...args);
    } catch (e) {
      console.error(`callGo(${name}):`, e);
      return null;
    }
  }
  return null;
}

// ── Init ──────────────────────────────────────────────────────
updatePageLabel();

// draw initial black canvas
if (ctx) {
  ctx.fillStyle = '#000';
  ctx.fillRect(0, 0, 240, 240);
  ctx.fillStyle = 'rgba(255,255,255,0.1)';
  ctx.font = '12px Space Mono, monospace';
  ctx.textAlign = 'center';
  ctx.fillText('No signal', 120, 124);
}
