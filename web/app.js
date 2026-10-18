// ONVIF-Inspector Web Client
(function() {
  'use strict';

  // State
  let devices = new Map();
  let events = [];
  let isPaused = false;
  let activeFilterText = '';
  let activeCategoryFilter = 'ALL';
  let eventSource = null;

  // DOM Elements - Main
  const elCamerasStat = document.getElementById('stat-cameras');
  const elEventsStat = document.getElementById('stat-events');
  const elSSEIndicator = document.getElementById('sse-indicator');
  const elSSEStatus = document.getElementById('sse-status');
  const elBtnPause = document.getElementById('btn-pause');
  const elBtnClear = document.getElementById('btn-clear');
  const elDeviceList = document.getElementById('device-list');
  const elEventsTbody = document.getElementById('events-tbody');
  const elFilterInput = document.getElementById('filter-input');

  // DOM Elements - Inspector Modal
  const elModal = document.getElementById('inspector-modal');
  const elModalClose = document.getElementById('modal-close');
  const elBtnCopyXML = document.getElementById('btn-copy-xml');

  // DOM Elements - Config Modal
  const elBtnConfig = document.getElementById('btn-config');
  const elBtnAddCamera = document.getElementById('btn-add-camera');
  const elConfigModal = document.getElementById('config-modal');
  const elConfigModalClose = document.getElementById('config-modal-close');
  const elConfigFilePath = document.getElementById('config-filepath');
  const elConfigAlert = document.getElementById('config-alert');

  const elCfgDefUsername = document.getElementById('cfg-def-username');
  const elCfgDefPassword = document.getElementById('cfg-def-password');
  const elBtnSaveDefault = document.getElementById('btn-save-default');

  const elCfgCamName = document.getElementById('cfg-cam-name');
  const elCfgCamTarget = document.getElementById('cfg-cam-target');
  const elCfgCamUsername = document.getElementById('cfg-cam-username');
  const elCfgCamPassword = document.getElementById('cfg-cam-password');
  const elBtnSaveCamera = document.getElementById('btn-save-camera');
  const elBtnClearCamForm = document.getElementById('btn-clear-cam-form');
  const elCfgCamerasList = document.getElementById('cfg-cameras-list');

  // DOM Elements - About Modal
  const elBtnAbout = document.getElementById('btn-about');
  const elAboutModal = document.getElementById('about-modal');
  const elAboutModalClose = document.getElementById('about-modal-close');
  const elAboutAppVersion = document.getElementById('about-app-version');

  // Initialize
  init();

  async function init() {
    initTheme();
    setupEventListeners();
    await loadInitialData();
    connectSSE();
  }

  function getDeviceKey(d) {
    if (d && d.endpoint) return d.endpoint;
    return `${d.ip}:${d.port || 80}`;
  }

  function setupEventListeners() {
    elBtnPause.addEventListener('click', () => {
      isPaused = !isPaused;
      elBtnPause.textContent = isPaused ? 'Resume Stream' : 'Pause Stream';
      elBtnPause.classList.toggle('bg-amber-600', isPaused);
    });

    elBtnClear.addEventListener('click', () => {
      events = [];
      renderEventsTable();
      updateStats();
    });

    elFilterInput.addEventListener('input', (e) => {
      activeFilterText = e.target.value.trim().toLowerCase();
      renderEventsTable();
    });

    document.querySelectorAll('.cat-filter').forEach(btn => {
      btn.addEventListener('click', () => {
        document.querySelectorAll('.cat-filter').forEach(b => b.classList.remove('active'));
        btn.classList.add('active');
        activeCategoryFilter = btn.dataset.cat;
        renderEventsTable();
      });
    });

    // Inspector modal
    elModalClose.addEventListener('click', closeModal);
    elModal.addEventListener('click', (e) => {
      if (e.target === elModal) closeModal();
    });

    elBtnCopyXML.addEventListener('click', () => {
      const xml = document.getElementById('modal-xml').textContent;
      navigator.clipboard.writeText(xml).then(() => {
        elBtnCopyXML.textContent = 'Copied!';
        setTimeout(() => elBtnCopyXML.textContent = 'Copy XML', 1500);
      });
    });

    // Configuration modal
    elBtnConfig.addEventListener('click', () => openConfigModal());
    elBtnAddCamera.addEventListener('click', () => openConfigModal('', 'admin'));
    elConfigModalClose.addEventListener('click', closeConfigModal);
    elConfigModal.addEventListener('click', (e) => {
      if (e.target === elConfigModal) closeConfigModal();
    });

    elBtnSaveDefault.addEventListener('click', saveDefaultCredentials);
    elBtnSaveCamera.addEventListener('click', saveCameraCredentials);
    elBtnClearCamForm.addEventListener('click', () => {
      elCfgCamName.value = '';
      elCfgCamTarget.value = '';
      elCfgCamUsername.value = 'admin';
      elCfgCamPassword.value = '';
    });

    // About modal
    if (elBtnAbout) {
      elBtnAbout.addEventListener('click', openAboutModal);
    }
    if (elAboutModalClose) {
      elAboutModalClose.addEventListener('click', closeAboutModal);
    }
    if (elAboutModal) {
      elAboutModal.addEventListener('click', (e) => {
        if (e.target === elAboutModal) closeAboutModal();
      });
    }

    document.addEventListener('keydown', (e) => {
      if (e.key === 'Escape') {
        closeModal();
        closeConfigModal();
        closeAboutModal();
      }
    });
  }

  async function loadInitialData() {
    try {
      const [devResp, evResp] = await Promise.all([
        fetch('/api/devices'),
        fetch('/api/events')
      ]);

      if (devResp.ok) {
        const devs = await devResp.json();
        if (Array.isArray(devs)) {
          devs.forEach(d => devices.set(getDeviceKey(d), d));
          renderDeviceList();
        }
      }

      if (evResp.ok) {
        const evs = await evResp.json();
        if (Array.isArray(evs)) {
          events = evs;
          renderEventsTable();
        }
      }

      updateStats();
    } catch (err) {
      console.error('Failed to load initial data:', err);
    }
  }

  function connectSSE() {
    if (eventSource) {
      eventSource.close();
    }

    eventSource = new EventSource('/api/events/stream');

    eventSource.onopen = () => {
      elSSEIndicator.className = 'w-2.5 h-2.5 rounded-full bg-emerald-500 animate-pulse';
      elSSEStatus.textContent = 'Live SSE';
    };

    eventSource.onerror = () => {
      elSSEIndicator.className = 'w-2.5 h-2.5 rounded-full bg-red-500';
      elSSEStatus.textContent = 'Reconnecting...';
    };

    eventSource.addEventListener('onvif-event', (e) => {
      try {
        const ev = JSON.parse(e.data);
        handleIncomingEvent(ev);
      } catch (err) {
        console.error('Failed to parse event JSON:', err);
      }
    });

    eventSource.addEventListener('device-update', (e) => {
      try {
        const dev = JSON.parse(e.data);
        devices.set(getDeviceKey(dev), dev);
        renderDeviceList();
        updateStats();
      } catch (err) {
        console.error('Failed to parse device JSON:', err);
      }
    });
  }

  function handleIncomingEvent(ev) {
    if (!isPaused) {
      events.push(ev);
      if (events.length > 1000) {
        events.shift();
      }
      renderEventsTable();
      updateStats();
    }
  }

  function updateStats() {
    elCamerasStat.textContent = devices.size;
    elEventsStat.textContent = events.length;
    const badge = document.getElementById('badge-camera-count');
    if (badge) badge.textContent = devices.size;
  }

  function renderDeviceList() {
    if (devices.size === 0) {
      elDeviceList.innerHTML = `<div class="text-center py-8 text-xs text-slate-500">Scanning network for ONVIF cameras...</div>`;
      return;
    }

    let html = '';
    devices.forEach((dev) => {
      const stateBadge = getStateBadge(dev.state);
      const name = dev.name || dev.model || 'ONVIF Camera';
      const addr = `${dev.ip}:${dev.port || 80}`;
      const latency = dev.ping_latency_ms ? ` • ${(dev.ping_latency_ms / 1e6).toFixed(1)}ms` : '';

      html += `
        <div class="bg-slate-800/60 p-3 rounded-lg border border-slate-700/60 space-y-2">
          <div class="flex items-center justify-between">
            <span class="font-semibold text-xs text-white truncate max-w-[170px]" title="${escapeHTML(name)}">${escapeHTML(name)}</span>
            <div class="flex items-center gap-1.5">
              ${stateBadge}
              <button class="btn-config-cam text-[11px] text-slate-400 hover:text-white px-1.5 py-0.5 rounded hover:bg-slate-700 transition"
                      data-target="${escapeHTML(addr)}" data-ip="${escapeHTML(dev.ip)}" data-name="${escapeHTML(dev.name || '')}" title="Configure Camera & Credentials">
                ⚙️
              </button>
            </div>
          </div>
          <div class="text-[11px] font-mono text-slate-400 truncate">${escapeHTML(addr)}</div>
          <div class="flex items-center justify-between text-[11px] text-slate-400">
            <span>${escapeHTML(dev.manufacturer || 'Generic')}${latency}</span>
            <span class="bg-slate-900 px-1.5 py-0.5 rounded text-indigo-400 font-mono">Ev: ${dev.event_count || 0}</span>
          </div>
        </div>
      `;
    });

    elDeviceList.innerHTML = html;

    // Attach configure buttons
    elDeviceList.querySelectorAll('.btn-config-cam').forEach(btn => {
      btn.addEventListener('click', () => {
        const target = btn.dataset.target || btn.dataset.ip;
        openConfigModal(target, 'admin', btn.dataset.name || '');
      });
    });
  }

  function renderEventsTable() {
    const filtered = events.filter(e => {
      if (activeCategoryFilter !== 'ALL' && e.category !== activeCategoryFilter) {
        return false;
      }
      if (activeFilterText) {
        const full = `${e.topic} ${e.camera_ip} ${e.camera_name || ''} ${e.category} ${e.property_state}`.toLowerCase();
        if (!full.includes(activeFilterText)) {
          return false;
        }
      }
      return true;
    });

    if (filtered.length === 0) {
      elEventsTbody.innerHTML = `
        <tr>
          <td colspan="6" class="text-center py-12 text-slate-500 font-sans">
            ${events.length === 0 ? 'Waiting for incoming ONVIF PullPoint events...' : 'No events match active filter'}
          </td>
        </tr>
      `;
      return;
    }

    // Render newest events first
    const displayList = [...filtered].reverse().slice(0, 200);
    let html = '';

    displayList.forEach(ev => {
      const timeStr = formatTime(ev.timestamp);
      const catBadge = getCategoryBadge(ev.category);
      const shortTopic = ev.topic ? ev.topic.replace(/^tns1:/, '') : '-';
      const camDisplay = ev.camera_name ?
        `<div><span class="font-semibold text-white text-xs">${escapeHTML(ev.camera_name)}</span> <span class="text-[10px] text-slate-400 font-mono">(${escapeHTML(ev.camera_ip)})</span></div>` :
        `<span class="font-mono text-slate-200">${escapeHTML(ev.camera_ip)}</span>`;

      html += `
        <tr class="hover:bg-slate-800/40 transition">
          <td class="py-2.5 px-4 text-slate-400 whitespace-nowrap">${timeStr}</td>
          <td class="py-2.5 px-4 whitespace-nowrap">${camDisplay}</td>
          <td class="py-2.5 px-4 whitespace-nowrap">${catBadge}</td>
          <td class="py-2.5 px-4 text-slate-300 break-all">${escapeHTML(shortTopic)}</td>
          <td class="py-2.5 px-4 font-bold text-emerald-400 whitespace-nowrap">${escapeHTML(ev.property_state)}</td>
          <td class="py-2.5 px-4 text-right whitespace-nowrap">
            <button class="btn-inspect px-2 py-1 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-[11px] border border-slate-700 transition"
                    data-id="${escapeHTML(ev.id)}">
              Inspect
            </button>
          </td>
        </tr>
      `;
    });

    elEventsTbody.innerHTML = html;

    // Attach inspect listeners
    elEventsTbody.querySelectorAll('.btn-inspect').forEach(btn => {
      btn.addEventListener('click', () => {
        const evId = btn.dataset.id;
        const ev = events.find(item => item.id === evId);
        if (ev) openModal(ev);
      });
    });
  }

  function openModal(ev) {
    document.getElementById('modal-id').textContent = ev.id || '-';
    document.getElementById('modal-timestamp').textContent = ev.timestamp || '-';
    document.getElementById('modal-camera').textContent = `${ev.camera_ip} (${ev.camera_id})`;
    document.getElementById('modal-state').textContent = ev.property_state || '-';
    document.getElementById('modal-topic').textContent = ev.topic || '-';

    const catBadge = document.getElementById('modal-category-badge');
    catBadge.textContent = (ev.category || 'OTHER').toUpperCase();

    // Payload properties
    const propsContainer = document.getElementById('modal-properties');
    if (ev.data && Object.keys(ev.data).length > 0) {
      let propsHTML = '';
      for (const [k, v] of Object.entries(ev.data)) {
        propsHTML += `<div><span class="text-slate-400">• ${escapeHTML(k)}:</span> <span class="text-white">${escapeHTML(v)}</span></div>`;
      }
      propsContainer.innerHTML = propsHTML;
    } else {
      propsContainer.innerHTML = '<span class="text-slate-500">(No properties)</span>';
    }

    // Raw XML
    document.getElementById('modal-xml').textContent = ev.raw_xml || '(No raw XML available)';

    elModal.classList.remove('hidden');
  }

  function closeModal() {
    elModal.classList.add('hidden');
  }

  // --- About Modal ---

  async function openAboutModal() {
    if (!elAboutModal) return;
    elAboutModal.classList.remove('hidden');
    try {
      const resp = await fetch('/api/about');
      if (resp.ok) {
        const info = await resp.json();
        if (elAboutAppVersion && info.version) {
          elAboutAppVersion.textContent = info.version;
        }
      }
    } catch (_) {}
  }

  function closeAboutModal() {
    if (elAboutModal) {
      elAboutModal.classList.add('hidden');
    }
  }

  // --- Configuration Management ---

  async function openConfigModal(prefillTarget = '', prefillUser = 'admin', prefillName = '') {
    hideConfigAlert();
    if (prefillTarget) {
      elCfgCamName.value = prefillName;
      elCfgCamTarget.value = prefillTarget;
      elCfgCamUsername.value = prefillUser;
      elCfgCamPassword.value = '';
    } else {
      elCfgCamName.value = '';
      elCfgCamTarget.value = '';
      elCfgCamUsername.value = 'admin';
      elCfgCamPassword.value = '';
    }
    elConfigModal.classList.remove('hidden');
    await loadConfigData();
    if (prefillTarget) {
      setTimeout(() => elCfgCamPassword.focus(), 50);
    } else {
      setTimeout(() => elCfgCamName.focus(), 50);
    }
  }

  function closeConfigModal() {
    elConfigModal.classList.add('hidden');
  }

  async function loadConfigData() {
    try {
      const resp = await fetch('/api/config');
      if (!resp.ok) return;

      const data = await resp.json();
      elConfigFilePath.textContent = `Target file: ${data.file_path || 'config.yaml'}`;

      if (data.default) {
        elCfgDefUsername.value = data.default.username || '';
        elCfgDefPassword.placeholder = data.default.has_password ? '•••••••• (configured)' : 'No password set';
      }

      renderConfiguredCameras(data.cameras || []);
    } catch (err) {
      console.error('Failed to load config:', err);
    }
  }

  function renderConfiguredCameras(cams) {
    if (!cams || cams.length === 0) {
      elCfgCamerasList.innerHTML = `<div class="text-slate-500 py-3 text-center">No specific cameras configured in config.yaml yet.</div>`;
      return;
    }

    let html = '';
    cams.forEach(cam => {
      const ident = cam.target || cam.ip || cam.endpoint;
      const pwdStatus = cam.has_password ? 'Password set' : 'No password';
      const camLabel = cam.name ?
        `<span class="font-semibold text-white text-xs">${escapeHTML(cam.name)}</span> <span class="font-mono text-slate-400 text-[11px]">(${escapeHTML(ident)})</span>` :
        `<span class="font-mono font-semibold text-slate-200 text-xs">${escapeHTML(ident)}</span>`;

      html += `
        <div class="bg-slate-900 p-3 rounded border border-slate-800 flex items-center justify-between gap-3">
          <div class="flex-1 truncate">
            <div>${camLabel}</div>
            <div class="text-[11px] text-slate-400">User: <span class="text-white">${escapeHTML(cam.username || 'admin')}</span> • ${pwdStatus}</div>
          </div>
          <div class="flex items-center gap-2">
            <button class="btn-edit-cam px-2 py-1 bg-slate-800 hover:bg-slate-700 text-slate-300 rounded text-[11px] transition"
                    data-target="${escapeHTML(ident)}" data-user="${escapeHTML(cam.username || 'admin')}" data-name="${escapeHTML(cam.name || '')}">
              Edit
            </button>
            <button class="btn-del-cam px-2 py-1 bg-red-950 hover:bg-red-900 text-red-300 rounded text-[11px] border border-red-800 transition"
                    data-target="${escapeHTML(ident)}">
              Delete
            </button>
          </div>
        </div>
      `;
    });

    elCfgCamerasList.innerHTML = html;

    // Bind Edit and Delete
    elCfgCamerasList.querySelectorAll('.btn-edit-cam').forEach(btn => {
      btn.addEventListener('click', () => {
        elCfgCamName.value = btn.dataset.name || '';
        elCfgCamTarget.value = btn.dataset.target;
        elCfgCamUsername.value = btn.dataset.user;
        elCfgCamPassword.value = '';
        elCfgCamPassword.focus();
      });
    });

    elCfgCamerasList.querySelectorAll('.btn-del-cam').forEach(btn => {
      btn.addEventListener('click', () => {
        deleteCameraConfig(btn.dataset.target);
      });
    });
  }

  async function saveDefaultCredentials() {
    hideConfigAlert();
    const username = elCfgDefUsername.value.trim();
    const password = elCfgDefPassword.value;

    try {
      const resp = await fetch('/api/config/default', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username, password })
      });

      const res = await resp.json();
      if (resp.ok) {
        showConfigAlert(res.message || 'Default credentials saved to config.yaml', false);
        await loadConfigData();
      } else {
        showConfigAlert(res.message || 'Failed to save default credentials', true);
      }
    } catch (err) {
      showConfigAlert('Network error saving default credentials', true);
    }
  }

  async function saveCameraCredentials() {
    hideConfigAlert();
    const name = elCfgCamName.value.trim();
    const target = elCfgCamTarget.value.trim();
    const username = elCfgCamUsername.value.trim();
    const password = elCfgCamPassword.value;

    if (!target) {
      showConfigAlert('Please provide a Camera Target or IP', true);
      return;
    }

    try {
      const resp = await fetch('/api/config/camera', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name, target, username, password })
      });

      const res = await resp.json();
      if (resp.ok) {
        showConfigAlert(res.message || 'Camera settings saved to config.yaml and reconnected!', false);
        elCfgCamPassword.value = '';
        await loadConfigData();
      } else {
        showConfigAlert(res.message || 'Failed to save camera credentials', true);
      }
    } catch (err) {
      showConfigAlert('Network error saving camera credentials', true);
    }
  }

  async function deleteCameraConfig(target) {
    if (!confirm(`Remove configuration for ${target} from config.yaml?`)) return;

    try {
      const resp = await fetch(`/api/config/camera?target=${encodeURIComponent(target)}`, {
        method: 'DELETE'
      });
      const res = await resp.json();
      if (resp.ok) {
        showConfigAlert(res.message || 'Camera removed from config.yaml', false);
        await loadConfigData();
      } else {
        showConfigAlert(res.message || 'Failed to delete camera config', true);
      }
    } catch (err) {
      showConfigAlert('Network error deleting camera config', true);
    }
  }

  function showConfigAlert(msg, isError) {
    elConfigAlert.textContent = msg;
    elConfigAlert.className = `mx-6 mt-4 p-3 rounded-lg text-xs ${isError ? 'alert-error' : 'alert-success'}`;
    elConfigAlert.classList.remove('hidden');
  }

  function hideConfigAlert() {
    elConfigAlert.classList.add('hidden');
  }

  function getStateBadge(state) {
    switch (state) {
      case 'Active':
        return '<span class="badge-state badge-active">Active</span>';
      case 'Reconnecting':
        return '<span class="badge-state badge-reconnect">Reconnect</span>';
      case 'Failed':
        return '<span class="badge-state badge-failed">Failed</span>';
      default:
        return `<span class="badge-state badge-closed">${escapeHTML(state || 'Unknown')}</span>`;
    }
  }

  function getCategoryBadge(cat) {
    switch (cat) {
      case 'Motion':
        return '<span class="badge-state badge-cat-motion">Motion</span>';
      case 'Input':
        return '<span class="badge-state badge-cat-input">Input</span>';
      case 'Tamper':
        return '<span class="badge-state badge-cat-tamper">Tamper</span>';
      case 'Audio':
        return '<span class="badge-state badge-cat-audio">Audio</span>';
      case 'System':
        return '<span class="badge-state badge-cat-system">System</span>';
      default:
        return `<span class="badge-state badge-cat-other">${escapeHTML(cat || 'Other')}</span>`;
    }
  }

  function formatTime(isoStr) {
    if (!isoStr) return '--:--:--';
    const d = new Date(isoStr);
    if (isNaN(d.getTime())) return isoStr;
    return d.toTimeString().split(' ')[0] + '.' + String(d.getMilliseconds()).padStart(3, '0');
  }

  function escapeHTML(str) {
    if (str == null) return '';
    return String(str)
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#39;');
  }

  function initTheme() {
    let saved = null;
    try {
      saved = localStorage.getItem('onvif_theme');
    } catch (_) {}
    const prefersDark = window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches;
    const initialTheme = saved || (prefersDark ? 'dark' : 'light');
    setTheme(initialTheme);

    const btnTheme = document.getElementById('btn-theme-toggle');
    if (btnTheme) {
      btnTheme.addEventListener('click', () => {
        const current = document.documentElement.getAttribute('data-theme') || 'dark';
        const next = current === 'dark' ? 'light' : 'dark';
        setTheme(next);
      });
    }
  }

  function setTheme(theme) {
    document.documentElement.setAttribute('data-theme', theme);
    document.documentElement.className = theme;
    try {
      localStorage.setItem('onvif_theme', theme);
    } catch (_) {}

    const icon = document.getElementById('theme-icon');
    const label = document.getElementById('theme-label');
    if (icon) icon.textContent = theme === 'dark' ? '🌙' : '☀️';
    if (label) label.textContent = theme === 'dark' ? 'Dark' : 'Light';
  }
})();
