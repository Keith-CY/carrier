// Carrier WebUI — vanilla JS, no external dependencies
(function () {
  'use strict';

  const $ = (s, p) => (p || document).querySelector(s);
  const $$ = (s, p) => [...(p || document).querySelectorAll(s)];

  // --- State ---
  let token = localStorage.getItem('carrier_token') || '';
  let selectedAgent = '';
  let selectedProvider = null; // { id, name, auth_mode, env_var, example_model }
  let providerApiKey = '';
  let addTargetAgent = '';
  let addChannel = '';
  let addChannelToken = '';
  let addChannelChatId = '';
  let addCarrierPairedUser = null;
  let addCarrierPairedUserCount = 0;
  let addWebhookSecret = '';
  let addPairSessionId = '';
  let addPairCode = '';
  let addPairPollRunID = 0;
  let lastAddResult = null;
  let logSource = null; // EventSource for SSE logs
  let logEntries = [];
  let logBuffer = [];
  let logPaused = false;
  let logSearchQuery = '';
  let logHandlersBound = false;
  let logEntrySeq = 1;
  let logLastPolledLines = [];
  let logStatusBase = 'Select an agent and click Connect.';
  const logLevelFilters = { DEBUG: true, INFO: true, WARN: true, ERROR: true };
  const LOG_FILTER_LEVELS = ['DEBUG', 'INFO', 'WARN', 'ERROR'];
  const LOG_ENTRY_LIMIT = 2000;

  // --- Helpers ---
  function escapeHtml(s) {
    const d = document.createElement('div');
    d.textContent = s;
    return d.innerHTML;
  }

  function api(method, path, body) {
    const opts = {
      method,
      headers: { 'Content-Type': 'application/json' },
    };
    if (token) opts.headers['Authorization'] = 'Bearer ' + token;
    if (body) opts.body = JSON.stringify(body);
    return fetch(path, opts).then(async r => {
      if (r.status === 401) {
        clearToken();
        throw new Error('Unauthorized');
      }
      const raw = await r.text();
      let data = {};
      if (raw) {
        try {
          data = JSON.parse(raw);
        } catch (e) {
          if (!r.ok) throw new Error(raw || 'Request failed (' + r.status + ')');
          return raw;
        }
      }
      if (!r.ok) {
        const errMsg =
          (data && data.message) ||
          (data && data.errorCode) ||
          (data && data.error && (data.error.message || data.error.code)) ||
          data.error ||
          ('Request failed (' + r.status + ')');
        throw new Error(errMsg);
      }
      return data;
    });
  }

  function clearToken() {
    token = '';
    localStorage.removeItem('carrier_token');
    showLogin();
  }

  function setMsg(id, text, type) {
    const el = $(id);
    if (!el) return;
    el.textContent = '';
    if (!text) return;
    const p = document.createElement('p');
    p.className = 'msg-' + (type || 'info');
    p.textContent = text;
    el.appendChild(p);
  }

  function isAddMode() {
    return !!addTargetAgent;
  }

  function resetAddMode() {
    addTargetAgent = '';
    addChannel = '';
    addChannelToken = '';
    addChannelChatId = '';
    addCarrierPairedUser = null;
    addCarrierPairedUserCount = 0;
    addWebhookSecret = '';
    addPairSessionId = '';
    addPairCode = '';
    addPairPollRunID += 1;
    lastAddResult = null;
  }

  function currentWizardTotalSteps() {
    return isAddMode() ? 3 : 5;
  }

  function addAgentSetupProfile() {
    const agentID = String(addTargetAgent || '').trim().toLowerCase();
    switch (agentID) {
      case 'picoclaw':
        return { displayName: 'PicoClaw', requiresPairing: true, hideWebhook: true };
      case 'openclaw':
        return { displayName: 'OpenClaw', requiresPairing: false, hideWebhook: true };
      case 'zeroclaw':
        return { displayName: 'ZeroClaw', requiresPairing: false, hideWebhook: true };
      default:
        return null;
    }
  }

  function collectEnvVars() {
    const vars = {};
    $$('#env-fields .env-row').forEach(row => {
      const inputs = row.querySelectorAll('input');
      if (inputs.length < 2) return;
      const key = (inputs[0].value || '').trim();
      const value = (inputs[1].value || '').trim();
      if (!key) return;
      vars[key] = value;
    });
    return vars;
  }

  function normalizeAgentCatalog(data) {
    if (Array.isArray(data)) return data;
    if (data && Array.isArray(data.agents)) return data.agents;
    return [];
  }

  function normalizeInstances(data) {
    if (Array.isArray(data)) return data;
    if (data && Array.isArray(data.instances)) return data.instances;
    return [];
  }

  function canContinueWithProvider() {
    if (!selectedProvider) return false;
    const mode = selectedProvider.auth_mode;
    if (mode === 'none') return true;
    if (providerApiKey) return true;
    if (isAddMode()) return true;
    if (mode !== 'api_key') return true;
    return false;
  }

  function refreshProviderNextButton() {
    const nextBtn = $('#provider-next');
    if (!nextBtn) return;
    nextBtn.disabled = !canContinueWithProvider();
  }

  // --- Health ---
  function checkHealth() {
    const opts = { headers: {} };
    if (token) opts.headers['Authorization'] = 'Bearer ' + token;
    fetch('/healthz', opts)
      .then(r => r.json())
      .then(d => {
        const b = $('#health-badge');
        if (d.status === 'ok') {
          b.textContent = '🟢 healthy';
          b.className = 'badge badge-ok';
        } else {
          b.textContent = '🔴 error';
          b.className = 'badge badge-error';
        }
      })
      .catch(() => {
        const b = $('#health-badge');
        b.textContent = '🔴 offline';
        b.className = 'badge badge-error';
      });
  }

  // --- Steps indicator ---
  function renderSteps(containerId, current, total) {
    const el = $(containerId);
    if (!el) return;
    el.textContent = '';
    for (let i = 0; i < total; i++) {
      const dot = document.createElement('div');
      dot.className = 'step-dot';
      if (i < current) dot.classList.add('done');
      if (i === current) dot.classList.add('active');
      el.appendChild(dot);
    }
  }

  // --- Routing ---
  const routes = [
    'welcome', 'setup', 'agents', 'provider', 'config', 'install', 'complete',
    'dashboard', 'agent-detail', 'logs', 'chat', 'settings',
  ];

  function showView(name) {
    routes.forEach(r => {
      const el = $('#view-' + r);
      if (el) el.classList.toggle('hidden', r !== name);
    });
    // Update nav active state
    $$('.nav-link').forEach(a => {
      a.classList.toggle('active', a.dataset.route === name);
    });
  }

  function navigate(hash) {
    if (!hash || hash === '#' || hash === '#/') hash = '#/welcome';
    const route = hash.replace('#/', '');

    if (route.startsWith('add/')) {
      const agent = decodeURIComponent(route.slice(4)).trim().toLowerCase();
      if (!agent) {
        location.hash = '#/welcome';
        return;
      }
      addTargetAgent = agent;
      selectedAgent = agent;
      lastAddResult = null;
      initSetup();
      return;
    }

    if (isAddMode()) {
      const keepAddRoutes = new Set(['setup', 'provider', 'install', 'complete']);
      if (!keepAddRoutes.has(route)) {
        resetAddMode();
      }
    }
    if (route !== 'dashboard') {
      closeAddAgentModal();
    }

    // Management views require auth
    const mgmtRoutes = ['dashboard', 'logs', 'chat', 'settings'];
    if (mgmtRoutes.includes(route)) {
      $('#nav').classList.remove('hidden');
    }

    switch (route) {
      case 'welcome': initWelcome(); break;
      case 'setup': initSetup(); break;
      case 'agents': initAgents(); break;
      case 'provider': initProvider(); break;
      case 'config': initConfig(); break;
      case 'install': initInstall(); break;
      case 'complete': initComplete(); break;
      case 'dashboard': initDashboard(); break;
      case 'logs': initLogs(); break;
      case 'chat': initChat(); break;
      case 'settings': initSettings(); break;
      default:
        if (route.startsWith('agents/')) {
          initAgentDetail(route.split('/')[1]);
        } else {
          location.hash = '#/welcome';
        }
    }
  }

  // --- Login ---
  function showLogin() {
    $('#login-overlay').classList.remove('hidden');
    $('#nav').classList.add('hidden');
    $('#logout-btn').classList.add('hidden');
  }

  function hideLogin() {
    $('#login-overlay').classList.add('hidden');
    $('#logout-btn').classList.remove('hidden');
  }

  function initLogin() {
    $('#login-btn').onclick = async () => {
      const t = $('#login-token').value.trim();
      if (!t) { setMsg('#login-msg', 'Please enter a token.', 'error'); return; }
      token = t;
      try {
        const r = await fetch('/healthz', {
          headers: { 'Authorization': 'Bearer ' + t },
        });
        if (r.ok) {
          localStorage.setItem('carrier_token', t);
          hideLogin();
          navigate(location.hash || '#/welcome');
        } else {
          token = '';
          setMsg('#login-msg', 'Invalid token or connection failed.', 'error');
        }
      } catch (e) {
        token = '';
        setMsg('#login-msg', 'Connection error: ' + e.message, 'error');
      }
    };

    $('#login-token').onkeydown = e => {
      if (e.key === 'Enter') $('#login-btn').click();
    };

    $('#logout-btn').onclick = clearToken;
    $('#settings-logout').onclick = clearToken;
  }

  // --- Welcome ---
  function initWelcome() {
    resetAddMode();
    showView('welcome');
    const status = $('#welcome-status');
    const btn = $('#welcome-continue');
    status.textContent = '';
    btn.classList.add('hidden');

    checkHealth();
    fetch('/healthz', { headers: token ? { 'Authorization': 'Bearer ' + token } : {} })
      .then(r => r.json())
      .then(d => {
        if (d.status === 'ok') {
          status.textContent = '🟢 Daemon connected.';
          btn.classList.remove('hidden');
        } else {
          status.textContent = '🔴 Daemon not responding.';
        }
      })
      .catch(() => {
        status.textContent = '🔴 Cannot reach daemon.';
      });

    btn.onclick = () => { location.hash = '#/setup'; };
  }

  // --- Setup ---
  function initSetup() {
    showView('setup');
    const stepTotal = currentWizardTotalSteps();
    renderSteps('#steps-indicator', 0, stepTotal);

    const title = $('#setup-title');
    const providerLabel = $('#setup-provider-label');
    const tokenLabel = $('#setup-token-label');
    const providerSelect = $('#provider');
    const tokenInput = $('#provider-token');
    const webhookInput = $('#webhook-secret');
    const webhookLabel = $('label[for="webhook-secret"]');
    const setupBtn = $('#setup-btn');
    const pairSection = $('#setup-telegram-pair');
    const pairInstruction = $('#setup-pair-instruction');
    const pairUseCarrierBtn = $('#setup-pair-use-carrier');
    const pairStartBtn = $('#setup-pair-start');
    tokenInput.value = isAddMode() ? addChannelToken : '';
    webhookInput.value = isAddMode() ? addWebhookSecret : '';
    setMsg('#setup-msg', '', 'info');
    setMsg('#setup-pair-msg', '', 'info');
    const addProfile = isAddMode() ? addAgentSetupProfile() : null;
    const addRequiresPairing = !!(addProfile && addProfile.requiresPairing);

    function updatePairInstruction() {
      if (!pairInstruction) return;
      if (!isAddMode() || providerSelect.value.trim().toLowerCase() !== 'telegram') {
        pairInstruction.textContent = '';
        return;
      }
      if (addChannelChatId) {
        pairInstruction.textContent = 'Paired chat id: ' + addChannelChatId;
        return;
      }
      if (addPairCode) {
        pairInstruction.textContent = 'Send `/pair ' + addPairCode + '` in your Telegram bot chat. Pairing will be detected automatically.';
        return;
      }
      pairInstruction.textContent = 'Click Start Pairing to get a code, then send it in your Telegram bot chat.';
    }

    function renderCarrierPairShortcut(channel) {
      if (!pairUseCarrierBtn) return;
      const isTelegramAdd = isAddMode() && String(channel || '').toLowerCase() === 'telegram';
      if (!isTelegramAdd || !addCarrierPairedUser || !addCarrierPairedUser.chatId) {
        pairUseCarrierBtn.classList.add('hidden');
        pairUseCarrierBtn.textContent = 'Use Carrier paired user (Recommended)';
        return;
      }
      pairUseCarrierBtn.classList.remove('hidden');
      const chatID = String(addCarrierPairedUser.chatId).trim();
      if (addCarrierPairedUserCount === 1) {
        pairUseCarrierBtn.textContent = 'Use Carrier paired user (Recommended): ' + chatID;
        return;
      }
      pairUseCarrierBtn.textContent = 'Use last Carrier paired user (Recommended): ' + chatID;
    }

    const delay = ms => new Promise(resolve => setTimeout(resolve, ms));

    async function autoWaitTelegramPairing(sessionID) {
      const sid = (sessionID || '').trim();
      if (!sid) return;
      const pollRunID = ++addPairPollRunID;
      setMsg('#setup-pair-msg', 'Pair code ready. Waiting for Telegram `/pair` command…', 'info');
      for (;;) {
        if (pollRunID !== addPairPollRunID) return;
        try {
          const resp = await api('POST', '/api/v1/telegram/pair/wait', { sessionId: sid });
          if (pollRunID !== addPairPollRunID) return;
          if (resp && resp.paired && resp.chatId) {
            addChannelChatId = String(resp.chatId).trim();
            addPairSessionId = '';
            addPairCode = '';
            updatePairInstruction();
            refreshSetupContinueState();
            setMsg('#setup-pair-msg', 'Pairing complete. Chat id: ' + addChannelChatId, 'info');
            return;
          }
          setMsg('#setup-pair-msg', 'Still waiting for Telegram `/pair` command…', 'info');
          continue;
        } catch (e) {
          if (pollRunID !== addPairPollRunID) return;
          const msg = (e && e.message ? String(e.message) : '').toLowerCase();
          if (msg.includes('expired') || msg.includes('session') || msg.includes('not found') || msg.includes('invalid')) {
            setMsg('#setup-pair-msg', 'Pair session expired. Click Start Pairing to generate a new code.', 'error');
            refreshSetupContinueState();
            return;
          }
          setMsg('#setup-pair-msg', 'Pair check failed, retrying…', 'error');
          await delay(1500);
        }
      }
    }

    async function loadCarrierPairedUser(channel) {
      if (!isAddMode() || channel !== 'telegram') {
        addCarrierPairedUser = null;
        addCarrierPairedUserCount = 0;
        renderCarrierPairShortcut(channel);
        return;
      }
      try {
        const resp = await api('GET', '/api/v1/pairing/sessions?provider=telegram');
        const sessions = (resp && Array.isArray(resp.sessions)) ? resp.sessions : [];
        const validSessions = sessions.filter(s => s && s.chatId && /^[0-9]+$/.test(String(s.chatId).trim()));
        addCarrierPairedUserCount = validSessions.length;
        if (validSessions.length > 0) {
          const latest = validSessions[0];
          addCarrierPairedUser = { chatId: String(latest.chatId).trim() };
        } else {
          addCarrierPairedUser = null;
        }
        if (validSessions.length === 1 && addCarrierPairedUser && !addChannelChatId) {
          addChannelChatId = addCarrierPairedUser.chatId;
          setMsg('#setup-pair-msg', 'Auto-selected Carrier paired Telegram user: ' + addChannelChatId, 'info');
        }
      } catch (_e) {
        addCarrierPairedUser = null;
        addCarrierPairedUserCount = 0;
      }
      renderCarrierPairShortcut(channel);
      updatePairInstruction();
      refreshSetupContinueState();
    }

    function refreshSetupContinueState() {
      if (!isAddMode()) {
        setupBtn.disabled = false;
        return;
      }
      const channel = providerSelect.value.trim().toLowerCase();
      const channelToken = tokenInput.value.trim();
      if (channel === 'telegram' && addRequiresPairing) {
        setupBtn.disabled = !channelToken || !addChannelChatId;
        return;
      }
      setupBtn.disabled = !channelToken;
    }

    if (addProfile) {
      title.textContent = 'Step 1 — Choose Chat Channel for ' + addProfile.displayName;
      providerLabel.textContent = 'Channel';
      tokenLabel.textContent = 'Channel Bot Token';
      providerSelect.value = addChannel || 'telegram';
      [...providerSelect.options].forEach(opt => {
        opt.disabled = opt.value !== '' && opt.value !== 'telegram';
      });
      if (addProfile.hideWebhook) {
        webhookInput.disabled = true;
        webhookInput.placeholder = 'Not required for ' + addProfile.displayName + ' add flow';
        webhookInput.classList.add('hidden');
        if (webhookLabel) webhookLabel.classList.add('hidden');
      } else {
        webhookInput.disabled = false;
        webhookInput.placeholder = 'Webhook verification secret';
        webhookInput.classList.remove('hidden');
        if (webhookLabel) webhookLabel.classList.remove('hidden');
      }
      if (addRequiresPairing) {
        pairSection.classList.remove('hidden');
        pairStartBtn.disabled = false;
        renderCarrierPairShortcut('telegram');
        updatePairInstruction();
        refreshSetupContinueState();
        loadCarrierPairedUser((providerSelect.value || '').trim().toLowerCase());
        if (addPairSessionId && !addChannelChatId) {
          autoWaitTelegramPairing(addPairSessionId);
        }
      } else {
        addChannelChatId = '';
        addPairSessionId = '';
        addPairCode = '';
        addPairPollRunID += 1;
        pairSection.classList.add('hidden');
        pairStartBtn.disabled = true;
        renderCarrierPairShortcut('');
        updatePairInstruction();
        refreshSetupContinueState();
      }
    } else {
      title.textContent = 'Step 1 — Configure Chat Channel';
      providerLabel.textContent = 'Chat Channel';
      tokenLabel.textContent = 'Channel Bot Token';
      providerSelect.value = '';
      [...providerSelect.options].forEach(opt => { opt.disabled = false; });
      webhookInput.disabled = false;
      webhookInput.placeholder = 'Webhook verification secret';
      webhookInput.classList.remove('hidden');
      if (webhookLabel) webhookLabel.classList.remove('hidden');
      pairSection.classList.add('hidden');
      renderCarrierPairShortcut('');
      setupBtn.disabled = false;
    }

    providerSelect.onchange = () => {
      if (!isAddMode()) return;
      const channel = providerSelect.value.trim().toLowerCase();
      if (channel !== 'telegram') {
        addChannelChatId = '';
        addPairSessionId = '';
        addPairCode = '';
        addPairPollRunID += 1;
      }
      if (addRequiresPairing) {
        renderCarrierPairShortcut(channel);
        loadCarrierPairedUser(channel);
        updatePairInstruction();
      } else {
        renderCarrierPairShortcut('');
      }
      refreshSetupContinueState();
    };

    tokenInput.oninput = () => {
      if (!isAddMode()) return;
      const inputToken = tokenInput.value.trim();
      if (inputToken !== addChannelToken) {
        addChannelChatId = '';
        addPairSessionId = '';
        addPairCode = '';
        addPairPollRunID += 1;
        setMsg('#setup-pair-msg', '', 'info');
      }
      if (addRequiresPairing && providerSelect.value.trim().toLowerCase() === 'telegram') {
        renderCarrierPairShortcut('telegram');
        loadCarrierPairedUser('telegram');
      }
      if (addRequiresPairing) {
        updatePairInstruction();
      }
      refreshSetupContinueState();
    };

    if (pairUseCarrierBtn) {
      pairUseCarrierBtn.onclick = () => {
        if (!addCarrierPairedUser || !addCarrierPairedUser.chatId) {
          setMsg('#setup-pair-msg', 'No reusable Carrier paired Telegram user found.', 'error');
          return;
        }
        addPairPollRunID += 1;
        addChannelChatId = String(addCarrierPairedUser.chatId).trim();
        addPairSessionId = '';
        addPairCode = '';
        updatePairInstruction();
        refreshSetupContinueState();
        setMsg('#setup-pair-msg', 'Using Carrier paired Telegram user: ' + addChannelChatId, 'info');
      };
    }

    pairStartBtn.onclick = async () => {
      const channel = providerSelect.value.trim().toLowerCase();
      const channelToken = tokenInput.value.trim();
      if (!addRequiresPairing) return;
      if (channel !== 'telegram') {
        setMsg('#setup-pair-msg', 'Pairing is only required for Telegram channel.', 'error');
        return;
      }
      if (!channelToken) {
        setMsg('#setup-pair-msg', 'Please enter channel bot token first.', 'error');
        return;
      }
      pairStartBtn.disabled = true;
      addPairPollRunID += 1;
      setMsg('#setup-pair-msg', 'Creating pair code…', 'info');
      try {
        const resp = await api('POST', '/api/v1/telegram/pair/init', { token: channelToken });
        addChannel = channel;
        addChannelToken = channelToken;
        addPairSessionId = (resp && resp.sessionId) || '';
        addPairCode = (resp && resp.pairCode) || '';
        addChannelChatId = '';
        renderCarrierPairShortcut(channel);
        updatePairInstruction();
        refreshSetupContinueState();
        autoWaitTelegramPairing(addPairSessionId);
      } catch (e) {
        setMsg('#setup-pair-msg', 'Pair init failed: ' + e.message, 'error');
      } finally {
        pairStartBtn.disabled = false;
        refreshSetupContinueState();
      }
    };

    $('#setup-btn').onclick = async () => {
      const channel = providerSelect.value.trim().toLowerCase();
      const channelToken = tokenInput.value.trim();
      const webhookSecret = webhookInput.value.trim();
      if (addTargetAgent) {
        if (!channel) {
          setMsg('#setup-msg', 'Please choose a channel.', 'error');
          return;
        }
        if (!channelToken) {
          setMsg('#setup-msg', 'Please enter channel bot token.', 'error');
          return;
        }
        if (channel === 'telegram' && addRequiresPairing && !addChannelChatId) {
          setMsg('#setup-msg', 'Please complete Telegram pairing first to capture your chat id.', 'error');
          return;
        }
        addChannel = channel;
        addChannelToken = channelToken;
        addWebhookSecret = webhookSecret;
        location.hash = '#/provider';
        return;
      }

      if (!channel) {
        setMsg('#setup-msg', 'Please choose a chat channel.', 'error');
        return;
      }
      if (!channelToken) {
        setMsg('#setup-msg', 'Please enter channel bot token.', 'error');
        return;
      }

      // Reuse channel fields for non-add wizard install path as well.
      addChannel = channel;
      addChannelToken = channelToken;
      addWebhookSecret = webhookSecret;
      location.hash = '#/agents';
    };
  }

  // --- Agents selection ---
  async function initAgents() {
    resetAddMode();
    showView('agents');
    renderSteps('#steps-indicator-2', 1, currentWizardTotalSteps());

    const list = $('#agent-pick');
    list.textContent = '';
    setMsg('#agents-msg', 'Loading agents…', 'info');

    try {
      const agents = normalizeAgentCatalog(await api('GET', '/api/v1/agents'));
      setMsg('#agents-msg', '', 'info');

      (agents || []).forEach(a => {
        const name = a.id || a.ID || a.name;
        const li = document.createElement('li');
        li.textContent = name;
        li.onclick = () => {
          $$('li', list).forEach(x => x.classList.remove('selected'));
          li.classList.add('selected');
          selectedAgent = name;
          $('#agents-next').disabled = false;
        };
        list.appendChild(li);
      });

      if (agents.length === 0) {
        setMsg('#agents-msg', 'No agents found.', 'error');
      }
    } catch (e) {
      setMsg('#agents-msg', 'Error loading agents: ' + e.message, 'error');
    }

    $('#agents-back').onclick = () => { location.hash = '#/setup'; };
    $('#agents-next').onclick = async () => {
      if (!selectedAgent) return;
      location.hash = '#/provider';
    };
  }

  function parseAgentList(text) {
    const lines = text.split('\n').filter(l => l.trim());
    const result = [];
    for (const line of lines) {
      const m = line.match(/^\s*(?:\d+\.\s+|[-•*]\s*)(\S+)/);
      if (m) result.push(m[1]);
    }
    if (result.length === 0 && text.trim()) {
      // Fallback: try space/comma separated
      text.trim().split(/[,\s]+/).forEach(w => {
        if (w && w.length > 1) result.push(w);
      });
    }
    return result;
  }

  // --- Provider selection ---
  async function initProvider() {
    showView('provider');
    renderSteps('#steps-indicator-p', isAddMode() ? 1 : 2, currentWizardTotalSteps());

    const heading = $('#view-provider h3');
    if (heading) heading.textContent = isAddMode() ? 'Step 2 — Select LLM Provider' : 'Step 3 — Select LLM Provider';
    $('#provider-agent-name').textContent = isAddMode() ? ('Adding: ' + selectedAgent) : ('Configuring: ' + selectedAgent);
    selectedProvider = null;
    providerApiKey = '';
    refreshProviderNextButton();
    $('#provider-auth-section').classList.add('hidden');

    const skipBtn = $('#provider-skip');
    skipBtn.classList.add('hidden');

    const addChoice = $('#provider-add-choice');
    const defaultSummary = $('#provider-default-summary');
    const useDefaultContinueBtn = $('#provider-use-default-continue');
    const otherWrap = $('#provider-other-wrap');

    const loading = $('#provider-loading');
    const catEl = $('#provider-categories');
    loading.classList.remove('hidden');
    addChoice.classList.add('hidden');
    otherWrap.classList.add('hidden');
    catEl.classList.add('hidden');
    catEl.textContent = '';
    setMsg('#provider-msg', '', 'info');

    const providerById = new Map();
    const providerItemById = new Map();
    let carrierDefaultProvider = null;

    function selectProvider(p, opts) {
      if (!p || !p.id) return;
      $$('.provider-item').forEach(x => x.classList.remove('selected'));
      const item = providerItemById.get(p.id);
      if (item) item.classList.add('selected');

      selectedProvider = p;
      showProviderAuth(p);
      refreshProviderNextButton();
    }

    function renderCarrierDefaultChoice() {
      if (!isAddMode()) return;
      addChoice.classList.remove('hidden');
      useDefaultContinueBtn.classList.add('hidden');
      useDefaultContinueBtn.disabled = true;

      if (!carrierDefaultProvider || !carrierDefaultProvider.configured) {
        defaultSummary.textContent = 'Carrier default provider is not configured.';
        return;
      }
      const pid = carrierDefaultProvider.id || '';
      const providerInfo = carrierDefaultProvider.provider || {};
      const pname = providerInfo.name || pid || 'unknown';

      if (!carrierDefaultProvider.available) {
        defaultSummary.textContent = 'Carrier default provider `' + pid + '` is not available in current gateway catalog.';
        return;
      }
      if (carrierDefaultProvider.reusable) {
        const backend = carrierDefaultProvider.credential_backend || 'saved store';
        defaultSummary.textContent = 'Using Carrier default: ' + pname + ' (`' + pid + '`) · credential: ' + backend + '.';
        const mapped = providerById.get(String(pid).trim().toLowerCase());
        if (mapped) {
          useDefaultContinueBtn.classList.remove('hidden');
          useDefaultContinueBtn.disabled = false;
          useDefaultContinueBtn.textContent = 'Use Carrier Provider (Recommended) →';
        }
        return;
      }
      if (carrierDefaultProvider.reason) {
        defaultSummary.textContent = 'Carrier default: ' + pname + ' (`' + pid + '`), but cannot reuse now: ' + carrierDefaultProvider.reason + '.';
      } else {
        defaultSummary.textContent = 'Carrier default provider is available but no reusable credential was found.';
      }
    }

    try {
      const data = await api('GET', '/api/v1/providers');
      loading.classList.add('hidden');
      carrierDefaultProvider = data && data.carrier_default_provider ? data.carrier_default_provider : null;

      const categoryOrder = [
        { key: 'builtin', label: '☁️ Built-in (API key)' },
        { key: 'custom',  label: '🔐 Custom / OAuth' },
        { key: 'local',   label: '🖥️ Local (no auth)' },
      ];

      categoryOrder.forEach(({ key, label }) => {
        const providers = (data.by_category || {})[key] || [];
        if (!providers.length) return;

        const section = document.createElement('div');
        section.className = 'provider-category';
        const h = document.createElement('h4');
        h.textContent = label;
        section.appendChild(h);

        const ul = document.createElement('ul');
        ul.className = 'provider-list';
        providers.forEach(p => {
          providerById.set(p.id, p);
          const li = document.createElement('li');
          li.className = 'provider-item';

          const badge = authModeBadgeText(p.auth_mode);
          li.innerHTML = '<strong>' + escapeHtml(p.name) + '</strong>' +
            ' <code>' + escapeHtml(p.id) + '</code>' +
            ' <span class="auth-badge">' + escapeHtml(badge) + '</span>';
          if (p.example_model) {
            li.innerHTML += '<br><span class="text-dim">e.g. ' + escapeHtml(p.example_model) + '</span>';
          }

          li.onclick = () => {
            selectProvider(p, {});
          };
          providerItemById.set(p.id, li);
          ul.appendChild(li);
        });
        section.appendChild(ul);
        catEl.appendChild(section);
      });

      if (isAddMode()) {
        renderCarrierDefaultChoice();
        const defaultProviderID = ((carrierDefaultProvider && carrierDefaultProvider.id) || '').trim().toLowerCase();
        const canAutoUseDefault = !!(carrierDefaultProvider && carrierDefaultProvider.reusable && defaultProviderID && providerById.has(defaultProviderID));
        if (canAutoUseDefault) {
          const p = providerById.get(defaultProviderID);
          selectProvider(p, {});
          otherWrap.classList.add('hidden');
          catEl.classList.add('hidden');
        } else {
          otherWrap.classList.remove('hidden');
          catEl.classList.remove('hidden');
        }
      } else {
        addChoice.classList.add('hidden');
        otherWrap.classList.remove('hidden');
        catEl.classList.remove('hidden');
      }
    } catch (e) {
      loading.classList.add('hidden');
      addChoice.classList.add('hidden');
      otherWrap.classList.add('hidden');
      setMsg('#provider-msg', 'Error loading providers: ' + e.message, 'error');
    }

    if (useDefaultContinueBtn) {
      useDefaultContinueBtn.onclick = () => {
        if (!isAddMode()) return;
        if (!carrierDefaultProvider || !carrierDefaultProvider.reusable) {
          setMsg('#provider-msg', 'Carrier default provider is not reusable right now.', 'error');
          return;
        }
        const providerID = String(carrierDefaultProvider.id || '').trim().toLowerCase();
        const p = providerById.get(providerID);
        if (!p) {
          setMsg('#provider-msg', 'Carrier default provider is not available in current provider list.', 'error');
          return;
        }
        selectProvider(p, {});
        location.hash = '#/install';
      };
    }

    $('#provider-back').onclick = () => { location.hash = isAddMode() ? '#/setup' : '#/agents'; };
    $('#provider-skip').onclick = () => {
      selectedProvider = null;
      providerApiKey = '';
      location.hash = '#/config';
    };
    $('#provider-next').onclick = () => {
      if (!selectedProvider) return;
      if (isAddMode()) {
        location.hash = '#/install';
        return;
      }
      location.hash = '#/config';
    };
  }

  function authModeBadgeText(mode) {
    switch (mode) {
      case 'api_key':            return '[API key]';
      case 'oauth_device_code':  return '[OAuth device code]';
      case 'oauth_plugin':       return '[OAuth plugin]';
      case 'gcloud_adc':         return '[gcloud ADC]';
      case 'none':               return '[no auth]';
      default:                   return '';
    }
  }

  function showProviderAuth(p) {
    const section = $('#provider-auth-section');
    const label = $('#provider-auth-label');
    const keyInput = $('#provider-api-key');
    const instructions = $('#provider-auth-instructions');

    section.classList.remove('hidden');
    keyInput.classList.add('hidden');
    instructions.classList.add('hidden');
    keyInput.value = '';
    providerApiKey = '';

    if (p.auth_mode === 'api_key') {
      if (isAddMode()) {
        label.textContent = 'Paste API key for ' + p.name + ' (' + p.env_var + ') if you are not reusing Carrier credential:';
      } else {
        label.textContent = 'Paste your API key for ' + p.name + ' (' + p.env_var + '):';
      }
      keyInput.classList.remove('hidden');
      keyInput.placeholder = 'API key';
      keyInput.oninput = () => {
        providerApiKey = keyInput.value.trim();
        refreshProviderNextButton();
      };
    } else if (p.auth_mode === 'none') {
      label.textContent = p.name + ' requires no authentication.';
      providerApiKey = '';
    } else {
      // OAuth / plugin / ADC
      label.textContent = p.name + ' requires external authentication.';
      instructions.classList.remove('hidden');
      if (isAddMode()) {
        instructions.innerHTML =
          'Paste access token below if you are not reusing Carrier credential.';
        keyInput.classList.remove('hidden');
        keyInput.placeholder = 'Access token (optional)';
        keyInput.oninput = () => {
          providerApiKey = keyInput.value.trim();
          refreshProviderNextButton();
        };
      } else {
        const cmd = 'openclaw models auth login --provider ' + p.id;
        instructions.innerHTML = 'Run: <code>' + escapeHtml(cmd) + '</code><br>Then click Continue.';
      }
    }
    refreshProviderNextButton();
  }

  // --- Config ---
  function initConfig() {
    if (isAddMode()) {
      location.hash = '#/install';
      return;
    }
    showView('config');
    renderSteps('#steps-indicator-3', isAddMode() ? 2 : 3, currentWizardTotalSteps());

    const heading = $('#view-config h3');
    if (heading) heading.textContent = isAddMode() ? 'Step 3 — Environment Variables' : 'Step 4 — Environment Variables';

    let agentLabel = (isAddMode() ? 'Adding: ' : 'Configuring: ') + selectedAgent;
    if (selectedProvider) {
      agentLabel += ' · Provider: ' + selectedProvider.name;
    }
    $('#config-agent-name').textContent = agentLabel;
    const fields = $('#env-fields');
    fields.textContent = '';

    // Pre-populate API key env var if provider was selected and key provided
    if (selectedProvider && selectedProvider.auth_mode === 'api_key' && providerApiKey && selectedProvider.env_var) {
      const row = document.createElement('div');
      row.className = 'env-row';
      const k = document.createElement('input');
      k.type = 'text'; k.value = selectedProvider.env_var; k.placeholder = 'KEY';
      const v = document.createElement('input');
      v.type = 'password'; v.value = providerApiKey; v.placeholder = 'VALUE';
      row.appendChild(k);
      row.appendChild(v);
      fields.appendChild(row);
    }

    addEnvRow();

    $('#add-env').onclick = addEnvRow;
    $('#config-back').onclick = () => { location.hash = '#/provider'; };
    $('#config-next').onclick = async () => {
      // Config is stored locally; proceed to install.
      location.hash = '#/install';
    };
  }

  function addEnvRow() {
    const row = document.createElement('div');
    row.className = 'env-row';
    const k = document.createElement('input');
    k.type = 'text'; k.placeholder = 'KEY';
    const v = document.createElement('input');
    v.type = 'text'; v.placeholder = 'VALUE';
    row.appendChild(k);
    row.appendChild(v);
    $('#env-fields').appendChild(row);
  }

  // --- Install ---
  function initInstall() {
    showView('install');
    renderSteps('#steps-indicator-4', isAddMode() ? 2 : 4, currentWizardTotalSteps());

    const heading = $('#view-install h3');
    if (heading) heading.textContent = isAddMode() ? 'Step 3 — Confirm Installation' : 'Step 5 — Confirm Installation';

    let summary = 'Agent: ' + selectedAgent;
    if (isAddMode()) summary += '\nChannel: ' + addChannel;
    if (selectedProvider) summary += '\nProvider: ' + selectedProvider.name;
    $('#install-summary').textContent = summary;

    $('#install-back').onclick = () => { location.hash = isAddMode() ? '#/provider' : '#/config'; };
    $('#install-confirm').onclick = async () => {
      setMsg('#install-msg', 'Installing…', 'info');
      try {
        if (isAddMode()) {
          if (!selectedProvider) {
            throw new Error('Please select a provider.');
          }
          const envVars = collectEnvVars();
          if (addWebhookSecret) {
            envVars.CARRIER_TELEGRAM_WEBHOOK_SECRET = addWebhookSecret;
          }
          const resp = await api('POST', '/api/v1/add', {
            agentId: selectedAgent,
            channel: addChannel,
            channelToken: addChannelToken,
            channelChatId: addChannelChatId,
            providerId: selectedProvider.id,
            providerToken: providerApiKey,
            reuseCredential: providerApiKey ? false : true,
            envVars,
          });
          lastAddResult = resp || null;
        } else {
          if (!selectedProvider && selectedAgent === 'picoclaw') {
            throw new Error('Please select an LLM provider for picoclaw.');
          }
          const envVars = collectEnvVars();
          if (addWebhookSecret) {
            envVars.CARRIER_TELEGRAM_WEBHOOK_SECRET = addWebhookSecret;
          }
          const resp = await api('POST', '/api/v1/add', {
            agentId: selectedAgent,
            channel: addChannel,
            channelToken: addChannelToken,
            channelChatId: addChannelChatId,
            providerId: selectedProvider ? selectedProvider.id : '',
            providerToken: providerApiKey,
            reuseCredential: true,
            envVars,
          });
          lastAddResult = resp || null;
        }
        location.hash = '#/complete';
      } catch (e) {
        setMsg('#install-msg', 'Error: ' + e.message, 'error');
      }
    };
  }

  // --- Complete ---
  function initComplete() {
    showView('complete');
    const title = $('#complete-title');
    const detail = $('#complete-detail');
    if (title) title.textContent = '✅ Setup Complete!';
    if (isAddMode() && lastAddResult) {
      const lines = [];
      const pairRequired = !!lastAddResult.pairRequired;
      if (pairRequired) {
        if (title) title.textContent = '⚠️ One step left: Pair your PicoClaw bot';
        lines.push('Action required: complete Telegram pairing before first use.');
      }
      if (lastAddResult.instanceId) lines.push('Instance: ' + lastAddResult.instanceId);
      if (lastAddResult.pairCode) lines.push('Pair code: ' + lastAddResult.pairCode);
      if (pairRequired && lastAddResult.pairCode) {
        lines.push('Send in your PicoClaw bot chat: /pair ' + lastAddResult.pairCode);
      }
      if (lastAddResult.pairedChatId) lines.push('Paired chat: ' + lastAddResult.pairedChatId);
      if (lastAddResult.workspacePath) lines.push('Workspace: ' + lastAddResult.workspacePath);
      if (lastAddResult.configPath) lines.push('Config: ' + lastAddResult.configPath);
      detail.textContent = lines.join('\n');
    } else {
      detail.textContent = '';
    }
    $('#complete-dashboard').onclick = () => {
      resetAddMode();
      location.hash = '#/dashboard';
    };
  }

  // --- Dashboard ---
  async function initDashboard() {
    resetAddMode();
    showView('dashboard');
    $('#nav').classList.remove('hidden');
    await refreshInstances();
    $('#refresh-instances').onclick = refreshInstances;
    $('#dashboard-add-agent').onclick = openAddAgentModal;
    $('#add-agent-cancel').onclick = closeAddAgentModal;
  }

  async function refreshInstances() {
    const el = $('#instance-list');
    const summary = $('#instance-summary');
    el.textContent = 'Loading…';
    summary.textContent = '';
    try {
      const instances = normalizeInstances(await api('GET', '/api/v1/instances'));

      el.textContent = '';
      if (instances.length === 0) {
        el.textContent = 'No installed agent instances.';
        summary.textContent = 'Use Add Agent to install and configure a new instance.';
        return;
      }
      const running = instances.filter(i => {
        const runtime = (i.runtime_state || i.runtimeState || i.runtime || '').toLowerCase();
        return runtime === 'running' || runtime === 'healthy';
      }).length;
      summary.textContent = 'Total: ' + instances.length + ' · Running: ' + running;

      instances.forEach(a => {
        const card = document.createElement('div');
        card.className = 'agent-card';
        const instanceID = a.id || a.ID;
        const agentID = a.agent_id || a.agentID || a.agent || a.type || 'unknown';
        const runtime = a.runtime_state || a.runtimeState || a.runtime || 'unknown';
        const provider = a.provider || 'n/a';
        const channel = a.channel || 'n/a';
        const pairRequired = !!(a.pair_required || a.pairRequired);
        const pairedChatId = a.paired_chat_id || a.pairedChatId || '';

        const h = document.createElement('h4');
        h.textContent = instanceID;

        const status = document.createElement('div');
        status.className = 'agent-status';
        status.textContent = statusIcon(runtime) + ' ' + runtime;

        const meta = document.createElement('div');
        meta.className = 'instance-meta';
        let metaText = 'Type: ' + agentID + ' · Channel: ' + channel + ' · Provider: ' + provider;
        if (pairRequired) metaText += ' · Pair: required';
        else if (pairedChatId) metaText += ' · Paired chat: ' + pairedChatId;
        meta.textContent = metaText;

        const btnRow = document.createElement('div');
        btnRow.className = 'btn-row';

        const startBtn = document.createElement('button');
        startBtn.className = 'btn-sm';
        startBtn.textContent = 'Start';
        startBtn.onclick = () => { instanceAction(instanceID, 'start'); };

        const stopBtn = document.createElement('button');
        stopBtn.className = 'btn-sm btn-secondary';
        stopBtn.textContent = 'Stop';
        stopBtn.onclick = () => { instanceAction(instanceID, 'stop'); };

        const uninstallBtn = document.createElement('button');
        uninstallBtn.className = 'btn-sm btn-danger';
        uninstallBtn.textContent = 'Uninstall';
        uninstallBtn.onclick = () => { instanceAction(instanceID, 'uninstall'); };

        btnRow.appendChild(startBtn);
        btnRow.appendChild(stopBtn);
        btnRow.appendChild(uninstallBtn);
        card.appendChild(h);
        card.appendChild(status);
        card.appendChild(meta);
        card.appendChild(btnRow);
        el.appendChild(card);
      });
    } catch (e) {
      el.textContent = 'Error: ' + e.message;
    }
  }

  async function instanceAction(instanceID, action) {
    if (action === 'uninstall') {
      const confirmed = window.confirm('Uninstall instance ' + instanceID + '?');
      if (!confirmed) return;
    }
    try {
      await api('POST', '/api/v1/instances/' + encodeURIComponent(instanceID) + '/' + action, {});
      await refreshInstances();
    } catch (e) {
      // Keep dashboard responsive and show a simple inline summary on top.
      const summary = $('#instance-summary');
      summary.textContent = 'Action failed: ' + e.message;
    }
  }

  async function openAddAgentModal() {
    const overlay = $('#add-agent-overlay');
    const list = $('#add-agent-options');
    overlay.classList.remove('hidden');
    list.textContent = '';
    setMsg('#add-agent-msg', 'Loading agents…', 'info');

    try {
      const agents = normalizeAgentCatalog(await api('GET', '/api/v1/agents'));
      setMsg('#add-agent-msg', '', 'info');
      if (agents.length === 0) {
        setMsg('#add-agent-msg', 'No agents available.', 'error');
        return;
      }
      agents.forEach(a => {
        const id = (a.id || a.ID || a.name || '').toLowerCase();
        if (!id) return;
        const li = document.createElement('li');
        li.innerHTML = '<strong>' + escapeHtml(id) + '</strong>';
        li.onclick = () => {
          closeAddAgentModal();
          location.hash = '#/add/' + encodeURIComponent(id);
        };
        list.appendChild(li);
      });
    } catch (e) {
      setMsg('#add-agent-msg', 'Error loading agents: ' + e.message, 'error');
    }
  }

  function closeAddAgentModal() {
    $('#add-agent-overlay').classList.add('hidden');
    setMsg('#add-agent-msg', '', 'info');
  }

  function parseAgentStatus(text) {
    const lines = text.split('\n').filter(l => l.trim());
    const result = [];
    for (const line of lines) {
      const m = line.match(/(\S+)\s*[:\-–]\s*(running|stopped|error|healthy|unknown|installed)/i);
      if (m) result.push({ name: m[1], status: m[2].toLowerCase() });
    }
    return result;
  }

  function statusIcon(s) {
    switch (s) {
      case 'running': case 'healthy': return '🟢';
      case 'error': return '🔴';
      default: return '⚪';
    }
  }

  async function agentAction(name, action) {
    try {
      await api('POST', '/api/v1/agents/' + encodeURIComponent(name) + '/' + action, {});
      await refreshInstances();
    } catch (e) {
      // silent
    }
  }

  // --- Agent detail ---
  async function initAgentDetail(id) {
    showView('agent-detail');
    const el = $('#agent-detail-content');
    el.textContent = 'Loading ' + id + '…';

    try {
      const state = await api('GET', '/api/v1/agents/' + encodeURIComponent(id) + '/status');
      el.textContent = '';

      const card = document.createElement('div');
      card.className = 'card';

      const h = document.createElement('h3');
      h.textContent = 'Agent: ' + id;
      card.appendChild(h);

      const pre = document.createElement('pre');
      pre.className = 'log-box';
      pre.textContent = JSON.stringify(state, null, 2);
      card.appendChild(pre);

      const btnRow = document.createElement('div');
      btnRow.className = 'btn-row';

      const backBtn = document.createElement('button');
      backBtn.className = 'btn-secondary';
      backBtn.textContent = '← Back';
      backBtn.onclick = () => { location.hash = '#/dashboard'; };

      const startBtn = document.createElement('button');
      startBtn.textContent = '▶ Start';
      startBtn.onclick = () => agentAction(id, 'start');

      const stopBtn = document.createElement('button');
      stopBtn.className = 'btn-secondary';
      stopBtn.textContent = '⏹ Stop';
      stopBtn.onclick = () => agentAction(id, 'stop');

      btnRow.appendChild(backBtn);
      btnRow.appendChild(startBtn);
      btnRow.appendChild(stopBtn);
      card.appendChild(btnRow);
      el.appendChild(card);
    } catch (e) {
      el.textContent = 'Error: ' + e.message;
    }
  }

  // --- Logs (SSE with polling fallback) ---
  function initLogs() {
    showView('logs');
    $('#nav').classList.remove('hidden');
    bindLogsControls();
    syncLogControls();
    renderLogRows(false);

    const agentSelect = $('#log-agent');
    agentSelect.textContent = '';

    // Populate instance list (mapped to runtime agent id for logs)
    api('GET', '/api/v1/instances').then(payload => {
      const instances = normalizeInstances(payload);
      instances.forEach(a => {
        const instanceID = a.id || a.ID || '';
        const name = a.agent_id || a.agentID || a.type || a.id || a.ID || a.name;
        if (!name) return;
        const opt = document.createElement('option');
        opt.value = name;
        opt.textContent = instanceID ? (instanceID + ' (' + name + ')') : name;
        agentSelect.appendChild(opt);
      });
    }).catch(() => {});
  }

  function bindLogsControls() {
    if (logHandlersBound) return;
    logHandlersBound = true;

    $('#log-clear').addEventListener('click', () => {
      clearLogs();
      renderLogRows(true);
    });

    $('#log-connect').addEventListener('click', () => {
      connectLogs($('#log-agent').value);
    });

    $('#log-pause').addEventListener('click', toggleLogPause);

    $('#log-search').addEventListener('input', (e) => {
      logSearchQuery = (e.target.value || '').trim().toLowerCase();
      renderLogRows(false);
    });

    LOG_FILTER_LEVELS.forEach(level => {
      const input = $('#log-filter-' + level.toLowerCase());
      if (!input) return;
      input.addEventListener('change', () => {
        logLevelFilters[level] = !!input.checked;
        renderLogRows(false);
      });
    });
  }

  function syncLogControls() {
    LOG_FILTER_LEVELS.forEach(level => {
      const input = $('#log-filter-' + level.toLowerCase());
      if (!input) return;
      input.checked = !!logLevelFilters[level];
    });
    const searchInput = $('#log-search');
    if (searchInput) searchInput.value = logSearchQuery;
    updateLogPauseButton();
    refreshLogStatus(getVisibleLogEntries().length);
  }

  function clearLogs() {
    logEntries = [];
    logBuffer = [];
    logEntrySeq = 1;
    logLastPolledLines = [];
  }

  function connectLogs(agentId) {
    if (!agentId) {
      logStatusBase = 'Select an agent and click Connect.';
      refreshLogStatus(getVisibleLogEntries().length);
      return;
    }
    if (logSource) {
      logSource.close();
      logSource = null;
    }

    clearLogs();
    logPaused = false;
    updateLogPauseButton();
    logStatusBase = 'Connecting to ' + agentId + '…';
    renderLogRows(true);

    // Try SSE first
    let sseUrl = '/api/v1/logs/stream?agent=' + encodeURIComponent(agentId);
    if (token) sseUrl += '&token=' + encodeURIComponent(token);
    try {
      const es = new EventSource(sseUrl);
      logSource = es;

      es.onopen = () => {
        logStatusBase = 'Connected to ' + agentId + ' via SSE.';
        refreshLogStatus(getVisibleLogEntries().length);
      };

      es.onmessage = (e) => {
        ingestLogLines([e.data], true);
      };

      es.onerror = () => {
        if (logSource !== es) return;
        es.close();
        logSource = null;
        addSystemLog('SSE disconnected, falling back to polling.', 'WARN');
        pollLogs(agentId);
      };
    } catch (_) {
      addSystemLog('SSE unavailable, using polling.', 'WARN');
      pollLogs(agentId);
    }
  }

  function pollLogs(agentId) {
    let running = true;

    // Store cancel function
    const cancel = () => { running = false; };
    logSource = { close: cancel };
    logStatusBase = 'Connected to ' + agentId + ' via polling.';
    refreshLogStatus(getVisibleLogEntries().length);

    const poll = async () => {
      if (!running) return;
      try {
        const res = await api('GET', '/api/v1/agents/' + encodeURIComponent(agentId) + '/logs');
        const lines = Array.isArray(res.lines) ? res.lines : [];
        const normalized = normalizeLineList(lines);
        const appended = diffAppendedLines(logLastPolledLines, normalized);
        logLastPolledLines = normalized;
        ingestLogLines(appended, true);
      } catch (e) {
        addSystemLog('poll error: ' + e.message, 'ERROR');
      }
      if (running) setTimeout(poll, 2000);
    };
    poll();
  }

  function normalizeLineList(lines) {
    return lines.map(line => String(line == null ? '' : line)).filter(line => line.trim().length > 0);
  }

  function diffAppendedLines(previous, next) {
    if (!next.length) return [];
    if (!previous.length) return next;

    const maxOverlap = Math.min(previous.length, next.length);
    for (let overlap = maxOverlap; overlap >= 1; overlap--) {
      let same = true;
      for (let i = 0; i < overlap; i++) {
        if (previous[previous.length - overlap + i] !== next[i]) {
          same = false;
          break;
        }
      }
      if (same) return next.slice(overlap);
    }
    return next;
  }

  function normalizeLogLevel(level) {
    const raw = String(level || '').trim().toUpperCase();
    if (!raw) return 'UNKNOWN';
    if (raw === 'WARNING') return 'WARN';
    if (raw === 'ERR') return 'ERROR';
    if (raw === 'TRACE') return 'DEBUG';
    return LOG_FILTER_LEVELS.includes(raw) ? raw : 'UNKNOWN';
  }

  function createLogEntry(level, message, timestamp) {
    return {
      id: logEntrySeq++,
      timestamp: timestamp && String(timestamp).trim() ? String(timestamp).trim() : new Date().toISOString(),
      level: normalizeLogLevel(level),
      message: String(message == null ? '' : message),
    };
  }

  function addSystemLog(message, level) {
    appendLogEntries([createLogEntry(level || 'INFO', message)]);
  }

  function parseLogLine(line) {
    const rawLine = String(line == null ? '' : line);
    const trimmed = rawLine.trim();
    if (!trimmed) return null;
    if (/^returned \d+ log lines for /i.test(trimmed)) return null;

    let timestamp = '';
    let level = 'UNKNOWN';
    let message = trimmed;

    if (trimmed.startsWith('{') && trimmed.endsWith('}')) {
      try {
        const parsed = JSON.parse(trimmed);
        if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
          timestamp = String(parsed.time || parsed.timestamp || parsed.ts || '').trim();
          level = normalizeLogLevel(parsed.level || parsed.severity || parsed.lvl);
          const msg = parsed.message !== undefined ? parsed.message : (parsed.msg !== undefined ? parsed.msg : parsed.text);
          if (msg !== undefined) {
            message = typeof msg === 'string' ? msg : JSON.stringify(msg);
          }
        }
      } catch (_) {
        // keep parsing with non-JSON heuristics
      }
    }

    if (level === 'UNKNOWN') {
      const bracketMatch = trimmed.match(/^\[([A-Za-z]+)\]\s*(.*)$/);
      if (bracketMatch) {
        level = normalizeLogLevel(bracketMatch[1]);
        message = (bracketMatch[2] || '').trim();
      }
    }

    if (!timestamp) {
      const timedMatch = trimmed.match(/^([0-9]{4}-[0-9]{2}-[0-9]{2}[T ][^\s]+)\s+([A-Za-z]+)\s*(.*)$/);
      if (timedMatch) {
        timestamp = timedMatch[1].trim();
        level = normalizeLogLevel(timedMatch[2]);
        message = (timedMatch[3] || '').trim();
      }
    }

    return createLogEntry(level, message || trimmed, timestamp);
  }

  function ingestLogLines(lines, stickToBottom) {
    if (!lines || !lines.length) return;
    if (logPaused) {
      lines.forEach(line => {
        if (line == null) return;
        logBuffer.push(String(line));
      });
      refreshLogStatus(getVisibleLogEntries().length);
      return;
    }
    const parsed = [];
    lines.forEach(line => {
      const entry = parseLogLine(line);
      if (entry) parsed.push(entry);
    });
    appendLogEntries(parsed, stickToBottom);
  }

  function appendLogEntries(entries, stickToBottom) {
    if (!entries || !entries.length) return;
    const output = $('#log-output');
    if (!output) return;
    const shouldStick = stickToBottom || (output.scrollHeight - output.scrollTop - output.clientHeight < 24);
    logEntries = logEntries.concat(entries);
    if (logEntries.length > LOG_ENTRY_LIMIT) {
      logEntries = logEntries.slice(logEntries.length - LOG_ENTRY_LIMIT);
    }
    renderLogRows(shouldStick);
  }

  function toggleLogPause() {
    logPaused = !logPaused;
    updateLogPauseButton();

    if (!logPaused && logBuffer.length) {
      const buffered = logBuffer.slice();
      logBuffer = [];
      ingestLogLines(buffered, true);
      return;
    }
    refreshLogStatus(getVisibleLogEntries().length);
  }

  function updateLogPauseButton() {
    const btn = $('#log-pause');
    if (!btn) return;
    btn.textContent = logPaused ? 'Resume' : 'Pause';
  }

  function getVisibleLogEntries() {
    return logEntries.filter(entry => {
      if (Object.prototype.hasOwnProperty.call(logLevelFilters, entry.level) && !logLevelFilters[entry.level]) {
        return false;
      }
      if (!logSearchQuery) return true;
      const haystack = (entry.timestamp + ' ' + entry.level + ' ' + entry.message).toLowerCase();
      return haystack.includes(logSearchQuery);
    });
  }

  function highlightLogText(text, query) {
    const source = String(text == null ? '' : text);
    if (!query) return escapeHtml(source);

    const lower = source.toLowerCase();
    let i = 0;
    let html = '';
    while (i < source.length) {
      const idx = lower.indexOf(query, i);
      if (idx === -1) {
        html += escapeHtml(source.slice(i));
        break;
      }
      html += escapeHtml(source.slice(i, idx));
      html += '<mark class="log-highlight">' + escapeHtml(source.slice(idx, idx + query.length)) + '</mark>';
      i = idx + query.length;
    }
    return html;
  }

  function renderLogRows(stickToBottom) {
    const output = $('#log-output');
    if (!output) return;

    const visible = getVisibleLogEntries();
    if (!visible.length) {
      output.textContent = '';
      refreshLogStatus(0);
      return;
    }

    const query = logSearchQuery;
    output.innerHTML = visible.map(entry =>
      '<div class="log-row log-row-data" data-level="' + escapeHtml(entry.level) + '">' +
        '<span class="log-cell-time">' + highlightLogText(entry.timestamp, query) + '</span>' +
        '<span class="log-cell-level"><span class="log-level-pill">' + highlightLogText(entry.level, query) + '</span></span>' +
        '<span class="log-cell-message">' + highlightLogText(entry.message, query) + '</span>' +
      '</div>'
    ).join('');

    if (stickToBottom) output.scrollTop = output.scrollHeight;
    refreshLogStatus(visible.length);
  }

  function refreshLogStatus(visibleCount) {
    const status = $('#log-status');
    if (!status) return;
    const visible = typeof visibleCount === 'number' ? visibleCount : getVisibleLogEntries().length;
    const parts = [logStatusBase];
    if (logEntries.length > 0) {
      parts.push('showing ' + visible + '/' + logEntries.length);
    }
    if (logPaused) parts.push('paused');
    if (logBuffer.length) parts.push('buffered ' + logBuffer.length);
    status.textContent = parts.filter(Boolean).join(' · ');
  }

  // --- Chat ---
  function initChat() {
    showView('chat');
    $('#nav').classList.remove('hidden');

    const send = () => {
      const input = $('#chat-input');
      const text = input.value.trim();
      if (!text) return;
      appendChat('You', text);
      input.value = '';
      // Chat is not supported in daemon mode; show helpful message.
      appendChat('Carrier', 'Chat is not available in daemon mode. Use the Dashboard to manage agents.');
    };

    $('#chat-send').onclick = send;
    $('#chat-input').onkeydown = e => { if (e.key === 'Enter') send(); };
  }

  function appendChat(sender, text) {
    const el = $('#chat-messages');
    const div = document.createElement('div');
    div.className = 'chat-msg';

    const senderSpan = document.createElement('span');
    senderSpan.className = 'sender';
    senderSpan.textContent = sender + ':';

    const bodySpan = document.createElement('span');
    bodySpan.className = 'body';
    bodySpan.textContent = ' ' + text;

    div.appendChild(senderSpan);
    div.appendChild(bodySpan);
    el.appendChild(div);
    el.scrollTop = el.scrollHeight;
  }

  // --- Settings ---
  async function initSettings() {
    showView('settings');
    $('#nav').classList.remove('hidden');
    const el = $('#settings-provider');
    const lines = ['Daemon mode — provider settings managed via config.json.'];
    try {
      const transport = await api('GET', '/api/v1/telegram/transport');
      const info = transport && transport.transport ? transport.transport : null;
      if (info && info.selected_mode) {
        let summary = 'Telegram transport: ' + info.selected_mode;
        if (info.reason_code) summary += ' (reason: ' + info.reason_code + ')';
        lines.push(summary);
        if (info.hint) lines.push('Hint: ' + info.hint);
      }
    } catch (_) {
      lines.push('Telegram transport status unavailable.');
    }
    el.textContent = lines.join(' ');
  }

  // --- Init ---
  function init() {
    initLogin();
    checkHealth();
    setInterval(checkHealth, 30000);

    window.addEventListener('hashchange', () => navigate(location.hash));

    // Check if token is valid
    if (token) {
      fetch('/healthz', { headers: { 'Authorization': 'Bearer ' + token } })
        .then(r => {
          if (r.ok) {
            hideLogin();
            navigate(location.hash || '#/dashboard');
          } else {
            showLogin();
          }
        })
        .catch(() => showLogin());
    } else {
      // No token — check if auth is required
      fetch('/healthz')
        .then(r => {
          if (r.ok) {
            // No auth required
            hideLogin();
            navigate(location.hash || '#/welcome');
          } else {
            showLogin();
          }
        })
        .catch(() => showLogin());
    }
  }

  init();
})();
