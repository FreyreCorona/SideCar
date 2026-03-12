/**
 * i18n.js — Lightweight internationalization for SideCar UI
 *
 * Supported locales: "en" (English, default), "pt-BR" (Brazilian Portuguese)
 *
 * Usage in HTML:  <span data-i18n="key"></span>
 * Usage in JS:    i18n.t("key")
 *
 * Language is auto-detected from the browser locale and can be overridden
 * by the user. The preference is saved in memory for the current session.
 */

const i18n = (() => {

  const translations = {
    en: {
      // Sidebar / navigation
      "nav.monitor":      "Monitor",
      "nav.upload":       "Upload",
      "nav.settings":     "Settings",
      "nav.disconnected": "Disconnected",
      "nav.connected":    "Connected",
      "nav.lightMode":    "Light mode",
      "nav.darkMode":     "Dark mode",
      "nav.startDaemon":  "▶ Start Daemon",
      "nav.stopDaemon":   "■ Stop Daemon",

      // Monitor tab
      "monitor.title":      "System Monitor",
      "monitor.subtitle":   "Live metrics pushed to the mini screen",
      "monitor.brightness": "Brightness",
      "monitor.cpu":        "CPU",
      "monitor.cpuUsage":   "Usage",
      "monitor.ram":        "RAM",
      "monitor.ramUsed":    "Used / Total",
      "monitor.battery":    "Battery",
      "monitor.batStatus":  "Status",
      "monitor.temp":       "Temp",
      "monitor.tempSub":    "CPU Temperature",
      "monitor.network":    "Network",
      "monitor.uptime":     "Uptime",
      "monitor.uptimeSub":  "Since last boot",
      "monitor.pageCPU":    "CPU & RAM",
      "monitor.pageNet":    "Network",
      "monitor.pagePower":  "Power",

      // Upload tab
      "upload.title":       "Upload",
      "upload.subtitle":    "Send a texture image or firmware file to the device",
      "upload.dropZone":    "Drop file here",
      "upload.dropSub":     ".png · .jpg · .acf · .bin — or click to browse",
      "upload.typeLabel":   "Type",
      "upload.convertBtn":  "⟳ Convert to ACF (RGB565)",
      "upload.uploadBtn":   "Upload to Device",
      "upload.imgReq":      "Image requirements:",
      "upload.imgSize":     "Recommended: 240 × 240 px",
      "upload.imgFmt":      "Formats accepted: PNG, JPEG",
      "upload.imgConvert":  "Convert first → uploads as RGB565 raw",
      "upload.acfDirect":   ".acf/.bin files upload directly",

      // Settings tab
      "settings.title":     "Settings",
      "settings.subtitle":  "Device and daemon configuration",
      "settings.port":      "Serial Port",
      "settings.portAuto":  "Auto-detect",
      "settings.refresh":   "Refresh",
      "settings.interval":  "Sync Interval",
      "settings.seconds":   "seconds",
      "settings.page":      "Device Page",
      "settings.setPage":   "Set",
      "settings.actions":   "Device Actions",
      "settings.wake":      "Wake",
      "settings.sleep":     "Sleep",
      "settings.reboot":    "Reboot",

      // Language selector
      "lang.label":         "Language",
    },

    "pt-BR": {
      // Barra lateral / navegação
      "nav.monitor":      "Monitor",
      "nav.upload":       "Enviar",
      "nav.settings":     "Configurações",
      "nav.disconnected": "Desconectado",
      "nav.connected":    "Conectado",
      "nav.lightMode":    "Modo claro",
      "nav.darkMode":     "Modo escuro",
      "nav.startDaemon":  "▶ Iniciar Daemon",
      "nav.stopDaemon":   "■ Parar Daemon",

      // Aba Monitor
      "monitor.title":      "Monitor do Sistema",
      "monitor.subtitle":   "Métricas ao vivo enviadas para a mini tela",
      "monitor.brightness": "Brilho",
      "monitor.cpu":        "CPU",
      "monitor.cpuUsage":   "Uso",
      "monitor.ram":        "RAM",
      "monitor.ramUsed":    "Usado / Total",
      "monitor.battery":    "Bateria",
      "monitor.batStatus":  "Status",
      "monitor.temp":       "Temp",
      "monitor.tempSub":    "Temperatura da CPU",
      "monitor.network":    "Rede",
      "monitor.uptime":     "Tempo Ativo",
      "monitor.uptimeSub":  "Desde o último boot",
      "monitor.pageCPU":    "CPU e RAM",
      "monitor.pageNet":    "Rede",
      "monitor.pagePower":  "Energia",

      // Aba Enviar
      "upload.title":       "Enviar Arquivo",
      "upload.subtitle":    "Envie uma imagem ou firmware para o dispositivo",
      "upload.dropZone":    "Solte o arquivo aqui",
      "upload.dropSub":     ".png · .jpg · .acf · .bin — ou clique para selecionar",
      "upload.typeLabel":   "Tipo",
      "upload.convertBtn":  "⟳ Converter para ACF (RGB565)",
      "upload.uploadBtn":   "Enviar para o Dispositivo",
      "upload.imgReq":      "Requisitos de imagem:",
      "upload.imgSize":     "Recomendado: 240 × 240 px",
      "upload.imgFmt":      "Formatos aceitos: PNG, JPEG",
      "upload.imgConvert":  "Converter primeiro → envia como RGB565 raw",
      "upload.acfDirect":   "Arquivos .acf/.bin são enviados diretamente",

      // Aba Configurações
      "settings.title":     "Configurações",
      "settings.subtitle":  "Configuração do dispositivo e daemon",
      "settings.port":      "Porta Serial",
      "settings.portAuto":  "Detecção automática",
      "settings.refresh":   "Atualizar",
      "settings.interval":  "Intervalo de Sincronização",
      "settings.seconds":   "segundos",
      "settings.page":      "Página do Dispositivo",
      "settings.setPage":   "Definir",
      "settings.actions":   "Ações do Dispositivo",
      "settings.wake":      "Ligar",
      "settings.sleep":     "Desligar",
      "settings.reboot":    "Reiniciar",

      // Seletor de idioma
      "lang.label":         "Idioma",
    },
  };

  // Detect locale from the browser; fall back to "en"
  let currentLocale = (navigator.language || "en").startsWith("pt") ? "pt-BR" : "en";


  /*
   * Translate a key. Falls back to English, then to the key itself.
   * @param {string} key
   * @returns {string}
   */
  function t(key) {
    return (translations[currentLocale] && translations[currentLocale][key])
      || (translations["en"] && translations["en"][key])
      || key;
  }

  /*
   * Switch the active locale and re-render all data-i18n elements.
   * @param {"en"|"pt-BR"} locale
   */
  function setLocale(locale) {
    if (!translations[locale]) return;
    currentLocale = locale;
    applyToDOM();
  }

  //Returns the currently active locale string.
  function getLocale() {
    return currentLocale;
  }

  
  //Apply translations to all elements that have a data-i18n attribute.
  //Call this once after the DOM is ready, and again after setLocale().
  function applyToDOM() {
    document.querySelectorAll("[data-i18n]").forEach(el => {
      const key = el.getAttribute("data-i18n");
      el.textContent = t(key);
    });
  }

  return { t, setLocale, getLocale, applyToDOM };
})();
