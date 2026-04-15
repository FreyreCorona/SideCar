// Theme
const themeToggle = document.getElementById('themeToggle');
const themeIcon   = document.getElementById('themeIcon');
const themeLabel  = document.getElementById('themeLabel');

let isDark = true;

function applyTheme(dark) {
  isDark = dark;
  document.documentElement.classList.toggle('light', !dark);
  themeIcon.textContent  = dark ? '☀' : '☾';
  
  const labelKey = dark ? 'nav.lightMode' : 'nav.darkMode';
  themeLabel.setAttribute('data-i18n', labelKey);
  themeLabel.textContent = i18n.t(labelKey);

  // Redraw sparklines with new colors.
  Object.values(sparklines).forEach(s => s.draw());
}

themeToggle.addEventListener('click', () => applyTheme(!isDark));

// Persist across page reloads (within same session).
const savedTheme = sessionStorage.getItem('theme');
if (savedTheme === 'light') applyTheme(false);

themeToggle.addEventListener('click', () => {
  sessionStorage.setItem('theme', isDark ? 'dark' : 'light');
});

// State 
const state = {
  daemonRunning : false,
  connected     : false,
  currentView   : 0,
  totalViews    : 3,
  devicePage    : 1,
  selectedFile  : null,
  isImageFile   : false,
  convertedACF  : null,  // []int after conversion
  brightness    : 100,
};

// DOM refs 
const statusDot   = document.getElementById('statusDot');
const statusLabel = document.getElementById('statusLabel');
const devicePill  = document.getElementById('devicePill');
const daemonBtn   = document.getElementById('daemonBtn');
const pageLabel   = document.getElementById('pageLabel');
const devicePageLabel = document.getElementById('devicePageLabel');
const pageInput   = document.getElementById('pageInput');
const canvas      = document.getElementById('previewCanvas');
const ctx         = canvas ? canvas.getContext('2d') : null;

// Tab switching
document.querySelectorAll('.nav-item').forEach(item => {
  item.addEventListener('click', () => {
    document.querySelectorAll('.nav-item').forEach(n => n.classList.remove('active'));
    document.querySelectorAll('.tab-panel').forEach(p => p.classList.remove('active'));
    item.classList.add('active');
    document.getElementById('tab-' + item.dataset.tab).classList.add('active');
  });
});

// Device status 
function setConnected(ok) {
  state.connected = ok;
  statusDot.className   = 'dot' + (ok ? ' active' : '');
  statusLabel.textContent = ok ? 'Connected' : 'Disconnected';
  devicePill.className  = 'device-pill' + (ok ? ' connected' : '');
}

// Called from Go when the daemon stops unexpectedly (device error).
window.onDaemonStopped = function() {
  if (state.daemonRunning) {
    state.daemonRunning = false;
    daemonBtn.textContent = i18n.t('nav.startDaemon');
    daemonBtn.classList.remove('running');
    setConnected(false);
    stopStatsLoop();
    appendLog('Daemon stopped — device disconnected', 'err');
  }
};

// Daemon
daemonBtn.addEventListener('click', async () => {
  if (daemonBtn.disabled) return;
  daemonBtn.disabled = true;

  if (!state.daemonRunning) {
    daemonBtn.textContent = '… Connecting';
    const ok = await callGo('startDaemon');
    if (ok) {
      state.daemonRunning = true;
      daemonBtn.textContent = i18n.t('nav.stopDaemon');
      daemonBtn.classList.add('running');
      setConnected(true);
      startStatsLoop();
    } else {
      daemonBtn.textContent = i18n.t('nav.startDaemon');
      appendLog('Failed to connect to device', 'err');
    }
  } else {
    daemonBtn.textContent = '… Stopping';
    await callGo('stopDaemon');
    state.daemonRunning = false;
    daemonBtn.textContent = i18n.t('nav.startDaemon');
    daemonBtn.classList.remove('running');
    setConnected(false);
    stopStatsLoop();
  }

  daemonBtn.disabled = false;
});

// View navigation 
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

// Device Page navigation
document.getElementById('nextPageBtn').addEventListener('click', () => {
  setDevicePage(state.devicePage + 1);
});
document.getElementById('prevPageBtn').addEventListener('click', () => {
  setDevicePage(state.devicePage - 1);
});

async function setDevicePage(page) {
  if (page < 1) page = 1;
  if (page > 10) page = 10;
  state.devicePage = page;
  
  if (devicePageLabel) devicePageLabel.textContent = state.devicePage;
  if (pageInput) pageInput.value = state.devicePage;
  
  await callGo('showPage', state.devicePage);
}

window.onViewChanged = function() { renderCurrentView(); };

// Canvas preview
async function renderCurrentView() {
  if (!ctx) return;
  try {
    const frame = await window.getCurrentFrame();
    ctx.fillStyle = '#000';
    ctx.fillRect(0, 0, 240, 240);
    if (frame && frame.texts)  renderTextFrame(ctx, canvas, frame);
    if (frame && frame.images) await renderImages(ctx, frame);
  } catch (e) { /* device may not be connected */ }
}

// Sparklines
const SPARK_MAX = 20; // data points kept

class Sparkline {
  constructor(canvasId, color) {
    this.el     = document.getElementById(canvasId);
    this.color  = color;
    this.data   = [];
    this.maxVal = 100;
  }
  push(val) {
    this.data.push(val);
    if (this.data.length > SPARK_MAX) this.data.shift();
    this.draw();
  }
  draw() {
    if (!this.el) return;
    const w = this.el.clientWidth  || 200;
    const h = this.el.clientHeight || 26;
    this.el.width  = w;
    this.el.height = h;
    const c = this.el.getContext('2d');
    c.clearRect(0, 0, w, h);
    if (this.data.length < 2) return;

    const accent = getComputedStyle(document.documentElement).getPropertyValue('--accent').trim();
    const muted  = getComputedStyle(document.documentElement).getPropertyValue('--surface2').trim();

    const pad = 2;
    const step = (w - pad * 2) / (SPARK_MAX - 1);
    const maxV = Math.max(this.maxVal, ...this.data) || 1;

    c.beginPath();
    this.data.forEach((v, i) => {
      const x = pad + i * step;
      const y = h - pad - (v / maxV) * (h - pad * 2);
      i === 0 ? c.moveTo(x, y) : c.lineTo(x, y);
    });

    // Fill under line.
    const grad = c.createLinearGradient(0, 0, 0, h);
    grad.addColorStop(0, accent + '55');
    grad.addColorStop(1, accent + '00');

    c.lineTo(pad + (this.data.length - 1) * step, h);
    c.lineTo(pad, h);
    c.closePath();
    c.fillStyle = grad;
    c.fill();

    c.beginPath();
    this.data.forEach((v, i) => {
      const x = pad + i * step;
      const y = h - pad - (v / maxV) * (h - pad * 2);
      i === 0 ? c.moveTo(x, y) : c.lineTo(x, y);
    });
    c.strokeStyle = accent;
    c.lineWidth   = 1.5;
    c.stroke();
  }
}

const sparklines = {
  cpu : new Sparkline('spkCPU',  '#5b6ef5'),
  ram : new Sparkline('spkRAM',  '#8b5cf6'),
  bat : new Sparkline('spkBAT',  '#22d3a0'),
  temp: new Sparkline('spkTEMP', '#f5a623'),
};
sparklines.temp.maxVal = 120;

// Stats loop 
let statsInterval = null;

function startStatsLoop() {
  renderCurrentView();
  updateStats();
  statsInterval = setInterval(updateStats, 3000);
}
function stopStatsLoop() {
  clearInterval(statsInterval);
  statsInterval = null;
  // Reset bars and sparklines.
  ['CPU','RAM','BAT','TEMP'].forEach(k => setBar('bar'+k, 0));
}

async function updateStats() {
  try {
    const s = await callGo('getStats');
    if (!s) return;

    // CPU
    const cpuPct = +(s.cpu || 0).toFixed(1);
    setBar('barCPU', cpuPct, cpuBarClass(cpuPct));
    setText('valCPU', cpuPct + '%');
    sparklines.cpu.push(cpuPct);

    // RAM
    const ramPct = s.ramPct || 0;
    setBar('barRAM', ramPct, cpuBarClass(ramPct));
    setText('valRAM', s.ramUsed + ' MB');
    setText('subRAM', `${s.ramUsed} / ${s.ramTotal} MB`);
    sparklines.ram.push(ramPct);

    // Battery
    const batPct = s.battery || 0;
    setBar('barBAT', batPct);
    setText('valBAT', batPct + '%');
    setText('subBAT', s.batStatus || '—');
    sparklines.bat.push(batPct);

    // Temperature
    const temp = +(s.temp || 0).toFixed(1);
    const tempPct = Math.min(100, temp / 100 * 100);
    setBar('barTEMP', tempPct, temp > 80 ? 'crit' : temp > 60 ? 'warn' : '');
    setText('valTEMP', temp > 0 ? temp + ' °C' : '—');
    sparklines.temp.push(temp);

    // Network
    setText('valNETIFACE', s.netIface || '—');
    setText('valTX', fmtBytes(s.txBytes) + '/s');
    setText('valRX', fmtBytes(s.rxBytes) + '/s');

    // Uptime
    setText('valUPTIME', fmtUptime(s.uptime));
  } catch (e) {}
}

function cpuBarClass(pct) {
  if (pct > 90) return 'crit';
  if (pct > 70) return 'warn';
  return '';
}

function setBar(id, pct, cls) {
  const el = document.getElementById(id);
  if (!el) return;
  el.style.width = Math.min(100, Math.max(0, pct)) + '%';
  el.className = 'stat-bar' + (cls ? ' ' + cls : '');
}
function setText(id, val) {
  const el = document.getElementById(id);
  if (el) el.textContent = val;
}
function fmtBytes(b) {
  if (!b || b === 0) return '0 B';
  if (b < 1024)         return b + ' B';
  if (b < 1024 * 1024)  return (b / 1024).toFixed(1) + ' KB';
  return (b / 1024 / 1024).toFixed(1) + ' MB';
}
function fmtUptime(s) {
  if (!s) return '—';
  const d = Math.floor(s / 86400);
  const h = Math.floor((s % 86400) / 3600);
  const m = Math.floor((s % 3600) / 60);
  if (d > 0) return `${d}d ${h}h ${m}m`;
  return `${h}h ${m}m`;
}

// Brightness 
const slider = document.getElementById('brightnessSlider');
const bVal   = document.getElementById('brightnessVal');
let bTimer   = null;
slider.addEventListener('input', () => {
  bVal.textContent = slider.value;
  clearTimeout(bTimer);
  bTimer = setTimeout(() => callGo('setBrightness', parseInt(slider.value)), 250);
});

// Upload 
const uploadZone    = document.getElementById('uploadZone');
const fileInput     = document.getElementById('fileInput');
const uploadBtn     = document.getElementById('uploadBtn');
const convertBtn    = document.getElementById('convertBtn');
const uploadMeta    = document.getElementById('uploadMeta');
const imgPreview    = document.getElementById('imgPreviewPanel');
const imgCanvas     = document.getElementById('imgPreviewCanvas');
const imgInfo       = document.getElementById('imgPreviewInfo');
const imgStatus     = document.getElementById('imgStatus');
const progressWrap  = document.getElementById('progressWrap');
const progressFill  = document.getElementById('progressFill');
const progressPct   = document.getElementById('progressPct');
const uploadLog     = document.getElementById('uploadLog');

const IMAGE_EXTS = ['png', 'jpg', 'jpeg'];

uploadZone.addEventListener('click', () => fileInput.click());
uploadZone.addEventListener('dragover', e => { e.preventDefault(); uploadZone.classList.add('drag-over'); });
uploadZone.addEventListener('dragleave', () => uploadZone.classList.remove('drag-over'));
uploadZone.addEventListener('drop', e => {
  e.preventDefault();
  uploadZone.classList.remove('drag-over');
  const f = e.dataTransfer.files[0];
  if (f) selectFile(f);
});
fileInput.addEventListener('change', () => {
  if (fileInput.files[0]) selectFile(fileInput.files[0]);
});

function selectFile(file) {
  state.selectedFile  = file;
  state.convertedACF  = null;
  const ext = file.name.split('.').pop().toLowerCase();
  state.isImageFile   = IMAGE_EXTS.includes(ext);

  document.getElementById('uploadFileName').textContent = file.name;
  document.getElementById('uploadFileSize').textContent = fmtFileSize(file.size);
  uploadMeta.classList.add('visible');

  if (state.isImageFile) {
    convertBtn.style.display = '';
    convertBtn.disabled      = false;
    uploadBtn.disabled       = true;   // must convert first
    showImagePreview(file);
    // Auto-select texture type for images.
    document.getElementById('fileTypeSelect').value = 'texture';
    appendLog(`Image selected: ${file.name} (${fmtFileSize(file.size)})`, 'ok');
  } else {
    convertBtn.style.display = 'none';
    uploadBtn.disabled       = false;
    imgPreview.classList.remove('visible');
    appendLog(`File selected: ${file.name} (${fmtFileSize(file.size)})`);
  }
}

// Show a 120×120 preview of the selected image and size validation.
function showImagePreview(file) {
  imgPreview.classList.add('visible');
  const reader = new FileReader();
  reader.onload = e => {
    const img = new Image();
    img.onload = () => {
      const W = img.width, H = img.height;
      const pctx = imgCanvas.getContext('2d');
      imgCanvas.width  = 120;
      imgCanvas.height = 120;
      pctx.drawImage(img, 0, 0, 120, 120);

      imgInfo.textContent = `${W} × ${H} px`;

      const ok = W === 240 && H === 240;
      imgStatus.className   = 'img-status ' + (ok ? 'ok' : 'warn');
      imgStatus.textContent = ok
        ? '✓ 240×240 — ready to convert'
        : `⚠ ${W}×${H} — will be scaled to 240×240`;
    };
    img.src = e.target.result;
  };
  reader.readAsDataURL(file);
}

// Convert image → ACF via Go binding, then enable upload.
convertBtn.addEventListener('click', async () => {
  if (!state.selectedFile || !state.isImageFile) return;

  convertBtn.disabled = true;
  convertBtn.textContent = '⟳ Converting…';
  appendLog('Converting image to RGB565 (ACF)…');

  try {
    const buf    = await state.selectedFile.arrayBuffer();
    const bytes  = new Uint8Array(buf);
    const b64    = btoa(String.fromCharCode(...bytes));

    const result = await callGo('convertImageToACF', b64, 240, 240);
    if (result && result.error) {
      appendLog('Conversion error: ' + result.error, 'err');
    } else {
      state.convertedACF  = result.data;
      uploadBtn.disabled  = false;
      appendLog(`Converted: ${result.width}×${result.height} → ${fmtFileSize(result.size)}`, 'ok');
      imgStatus.className   = 'img-status ok';
      imgStatus.textContent = `✓ Converted — ${fmtFileSize(result.size)} ready`;
    }
  } catch (e) {
    appendLog('Conversion failed: ' + e, 'err');
  }

  convertBtn.textContent = '⟳ Convert to ACF (RGB565)';
  convertBtn.disabled    = false;
});

// Upload to device.
uploadBtn.addEventListener('click', async () => {
  if (!state.selectedFile) return;
  if (!state.connected) {
    appendLog('Device not connected — start daemon first', 'err');
    return;
  }

  const fileType = document.getElementById('fileTypeSelect').value;
  uploadBtn.disabled = true;
  progressWrap.classList.add('visible');
  setProgress(0);

  try {
    let arr;
    if (state.isImageFile && state.convertedACF) {
      // Use the pre-converted ACF data.
      arr = state.convertedACF;
      appendLog(`Uploading converted ACF as [${fileType}]…`);
    } else {
      // Raw file (already an .acf or .bin).
      const buf = await state.selectedFile.arrayBuffer();
      arr = Array.from(new Uint8Array(buf));
      appendLog(`Uploading ${state.selectedFile.name} as [${fileType}]…`);
    }

    const result = await callGo('uploadFile', arr, fileType);
    if (result && result.error) {
      appendLog('Upload error: ' + result.error, 'err');
    } else {
      setProgress(100);
      appendLog('Upload complete ✓', 'ok');
    }
  } catch (e) {
    appendLog('Upload failed: ' + e, 'err');
  } finally {
    uploadBtn.disabled = false;
  }
});

// Progress pushed from Go.
window.onUploadProgress = function(pct) { setProgress(pct); };

function setProgress(pct) {
  progressFill.style.width = pct + '%';
  progressPct.textContent  = pct + '%';
}

function fmtFileSize(bytes) {
  if (bytes < 1024)           return bytes + ' B';
  if (bytes < 1024 * 1024)    return (bytes / 1024).toFixed(1) + ' KB';
  return (bytes / 1024 / 1024).toFixed(2) + ' MB';
}

function appendLog(msg, kind) {
  const line = document.createElement('div');
  line.textContent = `[${new Date().toLocaleTimeString()}] ${msg}`;
  if (kind === 'err')  line.className = 'log-err';
  if (kind === 'ok')   line.className = 'log-ok';
  if (kind === 'warn') line.className = 'log-warn';
  uploadLog.appendChild(line);
  uploadLog.scrollTop = uploadLog.scrollHeight;
}

// Settings
document.getElementById('wakeBtn').addEventListener('click', () => callGo('wake'));
document.getElementById('sleepBtn').addEventListener('click', () => callGo('sleep'));
document.getElementById('rebootBtn').addEventListener('click', async () => {
  if (confirm('Reboot the device?')) callGo('reboot');
});
document.getElementById('refreshPortsBtn').addEventListener('click', async () => {
  const ports = await callGo('listPorts');
  const sel = document.getElementById('portSelect');
  sel.innerHTML = '<option value="auto">Auto-detect</option>';
  (ports || []).forEach(p => {
    const opt = document.createElement('option');
    opt.value = opt.textContent = p;
    sel.appendChild(opt);
  });
});

// callGo helper
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

// Init
updatePageLabel();

if (ctx) {
  ctx.fillStyle = '#000';
  ctx.fillRect(0, 0, 240, 240);
  ctx.fillStyle = 'rgba(255,255,255,0.12)';
  ctx.font = '11px Space Mono, monospace';
  ctx.textAlign = 'center';
  ctx.fillText('No signal', 120, 124);
}
