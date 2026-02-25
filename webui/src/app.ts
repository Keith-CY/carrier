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
  let remoteHostsCache = [];
  let providerProfilesCache = [];
  let serverManageHostID = '';
  let serverManageOperationRunning = false;
  let serverManageLastOperation = null;
  let serverHostLastOperationByID = {};
  let serverEditingHostID = '';
  let profileEditingProfileID = '';
  let remoteChatSessionID = '';
  let remoteChatAbortController = null;
  let remoteChatLastInput = '';
  let remoteChatActiveAssistantNode = null;
  let remoteChatMessages = [];
  let remoteChatMessageSeq = 0;
  let remoteObservabilityData = null;
  const DEFAULT_FEATURE_FLAGS = {
    remoteControlPlaneEnabled: false,
    remoteChatEnabled: false,
    providerBindingEnabled: false,
  };
  let featureFlags = { ...DEFAULT_FEATURE_FLAGS };

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

  function toFeatureBool(value, fallback) {
    if (typeof value === 'boolean') return value;
    if (typeof value === 'string') {
      const lowered = value.trim().toLowerCase();
      if (lowered === 'true' || lowered === '1' || lowered === 'yes' || lowered === 'on') return true;
      if (lowered === 'false' || lowered === '0' || lowered === 'no' || lowered === 'off') return false;
    }
    if (typeof value === 'number') return value !== 0;
    return fallback;
  }

  function normalizeFeatureFlags(payload) {
    const source = payload && typeof payload === 'object' && payload.features && typeof payload.features === 'object'
      ? payload.features
      : payload;
    return {
      remoteControlPlaneEnabled: toFeatureBool(source && source.remoteControlPlaneEnabled, DEFAULT_FEATURE_FLAGS.remoteControlPlaneEnabled),
      remoteChatEnabled: toFeatureBool(source && source.remoteChatEnabled, DEFAULT_FEATURE_FLAGS.remoteChatEnabled),
      providerBindingEnabled: toFeatureBool(source && source.providerBindingEnabled, DEFAULT_FEATURE_FLAGS.providerBindingEnabled),
    };
  }

  function setNavRouteVisible(route, visible) {
    const link = $('.nav-link[data-route="' + route + '"]');
    if (!link) return;
    link.classList.toggle('hidden', !visible);
  }

  function isRouteEnabled(route) {
    if (route === 'servers' || route === 'profiles' || route === 'remote-observability') {
      return !!featureFlags.remoteControlPlaneEnabled;
    }
    if (route === 'remote-chat') {
      return !!featureFlags.remoteControlPlaneEnabled && !!featureFlags.remoteChatEnabled;
    }
    return true;
  }

  function applyFeatureFlags() {
    const remoteControlVisible = !!featureFlags.remoteControlPlaneEnabled;
    const remoteChatVisible = remoteControlVisible && !!featureFlags.remoteChatEnabled;
    setNavRouteVisible('servers', remoteControlVisible);
    setNavRouteVisible('profiles', remoteControlVisible);
    setNavRouteVisible('remote-chat', remoteChatVisible);
    setNavRouteVisible('remote-observability', remoteControlVisible);
  }

  async function refreshFeatureFlags() {
    const previous = { ...featureFlags };
    try {
      const payload = await api('GET', '/api/v1/features');
      featureFlags = normalizeFeatureFlags(payload);
    } catch (_e) {
      // Rollout safeguard: keep prior known-good flags instead of failing open.
      featureFlags = previous;
    }
    applyFeatureFlags();
    return featureFlags;
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

  const MANAGED_AGENT_PROFILES = {
    picoclaw:  { displayName: 'PicoClaw',  requiresPairing: true,  hideWebhook: true },
    openclaw:  { displayName: 'OpenClaw',  requiresPairing: false, hideWebhook: true },
    zeroclaw:  { displayName: 'ZeroClaw',  requiresPairing: false, hideWebhook: true },
  };

  function addAgentSetupProfile() {
    const agentID = String(addTargetAgent || '').trim().toLowerCase();
    return MANAGED_AGENT_PROFILES[agentID] || null;
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
    'servers', 'profiles', 'remote-chat', 'remote-observability',
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
    const mgmtRoutes = ['dashboard', 'logs', 'chat', 'settings', 'servers', 'profiles', 'remote-chat', 'remote-observability'];
    if (mgmtRoutes.includes(route)) {
      $('#nav').classList.remove('hidden');
    }

    if (!isRouteEnabled(route)) {
      location.hash = '#/dashboard';
      return;
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
      case 'servers': initServers(); break;
      case 'profiles': initProfiles(); break;
      case 'remote-chat': initRemoteChat(); break;
      case 'remote-observability': initRemoteObservability(); break;
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
          await refreshFeatureFlags();
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
        { key: 'generic', label: '🖥️ Generic (no auth)' },
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

    // Linear-time overlap detection: longest prefix of `next` that is a suffix of `previous`.
    const separator = Symbol('log-overlap-separator');
    const sequence = next.concat([separator], previous);
    const prefix = new Array(sequence.length).fill(0);

    for (let i = 1; i < sequence.length; i++) {
      let j = prefix[i - 1];
      while (j > 0 && sequence[i] !== sequence[j]) {
        j = prefix[j - 1];
      }
      if (sequence[i] === sequence[j]) {
        j += 1;
      }
      prefix[i] = j;
    }
    const overlap = Math.min(next.length, prefix[prefix.length - 1] || 0);
    return next.slice(overlap);
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

  function appendHighlightedText(container, text, query) {
    const source = String(text == null ? '' : text);
    if (!query) {
      container.textContent = source;
      return;
    }

    const lower = source.toLowerCase();
    let i = 0;
    while (i < source.length) {
      const idx = lower.indexOf(query, i);
      if (idx === -1) {
        container.appendChild(document.createTextNode(source.slice(i)));
        break;
      }
      if (idx > i) {
        container.appendChild(document.createTextNode(source.slice(i, idx)));
      }
      const mark = document.createElement('mark');
      mark.className = 'log-highlight';
      mark.textContent = source.slice(idx, idx + query.length);
      container.appendChild(mark);
      i = idx + query.length;
    }
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
    output.textContent = '';
    const fragment = document.createDocumentFragment();
    visible.forEach(entry => {
      const row = document.createElement('div');
      row.className = 'log-row log-row-data';
      row.dataset.level = entry.level;

      const timeCell = document.createElement('span');
      timeCell.className = 'log-cell-time';
      appendHighlightedText(timeCell, entry.timestamp, query);

      const levelCell = document.createElement('span');
      levelCell.className = 'log-cell-level';
      const levelPill = document.createElement('span');
      levelPill.className = 'log-level-pill';
      appendHighlightedText(levelPill, entry.level, query);
      levelCell.appendChild(levelPill);

      const messageCell = document.createElement('span');
      messageCell.className = 'log-cell-message';
      appendHighlightedText(messageCell, entry.message, query);

      row.appendChild(timeCell);
      row.appendChild(levelCell);
      row.appendChild(messageCell);
      fragment.appendChild(row);
    });
    output.appendChild(fragment);

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

  async function fetchRemoteHosts() {
    const payload = await api('GET', '/api/v1/remote/hosts');
    const hosts = payload && Array.isArray(payload.hosts) ? payload.hosts : [];
    remoteHostsCache = hosts;
    pruneServerHostOperationCache(hosts);
    return hosts;
  }

  async function fetchProviderProfiles() {
    const payload = await api('GET', '/api/v1/provider-profiles');
    const profiles = payload && Array.isArray(payload.profiles) ? payload.profiles : [];
    providerProfilesCache = profiles;
    return profiles;
  }

  function getServerManageHostID() {
    return String(serverManageHostID || '').trim();
  }

  function syncServerAuthModeInputs() {
    const authMode = $('#server-auth-mode');
    const hostInput = $('#server-host');
    const keyInput = $('#server-key-path');
    const sshConfigInput = $('#server-ssh-config-host');
    if (!authMode || !hostInput || !keyInput || !sshConfigInput) return;
    const mode = String(authMode.value || '').trim().toLowerCase();
    const privateKey = mode === 'private_key';
    keyInput.disabled = !privateKey;
    hostInput.disabled = false;
    sshConfigInput.disabled = privateKey;
  }

  function updateServerEditorUI() {
    const saveBtn = $('#server-save');
    const cancelBtn = $('#server-cancel-edit');
    const stateEl = $('#server-editor-state');
    const editing = String(serverEditingHostID || '').trim();
    if (saveBtn) {
      saveBtn.textContent = editing ? 'Update Host' : 'Save Host';
    }
    if (cancelBtn) {
      cancelBtn.classList.toggle('hidden', !editing);
    }
    if (stateEl) {
      stateEl.textContent = editing ? ('Editing host: ' + editing) : '';
    }
  }

  function clearServerFormValues() {
    const defaults = {
      '#server-name': '',
      '#server-host': '',
      '#server-port': '22',
      '#server-user': '',
      '#server-key-path': '',
      '#server-ssh-config-host': '',
      '#server-runtime-mode': 'on_demand',
      '#server-auth-mode': 'private_key',
    };
    Object.keys(defaults).forEach(selector => {
      const el = $(selector);
      if (!el) return;
      el.value = defaults[selector];
    });
    syncServerAuthModeInputs();
  }

  function resetServerEditor(clearForm) {
    serverEditingHostID = '';
    updateServerEditorUI();
    if (clearForm) {
      clearServerFormValues();
    }
  }

  function beginServerEdit(hostID) {
    const key = String(hostID || '').trim();
    if (!key) return;
    const host = remoteHostsCache.find(item => String(item && item.id ? item.id : '') === key);
    if (!host) return;
    serverEditingHostID = key;
    const map = {
      '#server-name': host.name || host.id || '',
      '#server-host': host.host || '',
      '#server-port': String(host.port || 22),
      '#server-user': host.user || '',
      '#server-key-path': host.keyPath || '',
      '#server-ssh-config-host': host.sshConfigHost || '',
      '#server-runtime-mode': host.runtimeMode || 'on_demand',
      '#server-auth-mode': host.authMode || 'private_key',
    };
    Object.keys(map).forEach(selector => {
      const el = $(selector);
      if (!el) return;
      el.value = map[selector];
    });
    syncServerAuthModeInputs();
    updateServerEditorUI();
  }

  function updateProfileEditorUI() {
    const saveBtn = $('#profile-save');
    const cancelBtn = $('#profile-cancel-edit');
    const stateEl = $('#profile-editor-state');
    const editing = String(profileEditingProfileID || '').trim();
    if (saveBtn) {
      saveBtn.textContent = editing ? 'Update Profile' : 'Save Profile';
    }
    if (cancelBtn) {
      cancelBtn.classList.toggle('hidden', !editing);
    }
    if (stateEl) {
      stateEl.textContent = editing ? ('Editing profile: ' + editing) : '';
    }
  }

  function clearProfileFormValues() {
    const defaults = {
      '#profile-name': '',
      '#profile-provider': '',
      '#profile-model': '',
      '#profile-base-url': '',
      '#profile-auth-ref': '',
      '#profile-enabled': 'true',
    };
    Object.keys(defaults).forEach(selector => {
      const el = $(selector);
      if (!el) return;
      el.value = defaults[selector];
    });
  }

  function resetProfileEditor(clearForm) {
    profileEditingProfileID = '';
    updateProfileEditorUI();
    if (clearForm) {
      clearProfileFormValues();
    }
  }

  function beginProfileEdit(profileID) {
    const key = String(profileID || '').trim();
    if (!key) return;
    const profile = providerProfilesCache.find(item => String(item && item.id ? item.id : '') === key);
    if (!profile) return;
    profileEditingProfileID = key;
    const map = {
      '#profile-name': profile.name || '',
      '#profile-provider': profile.provider || '',
      '#profile-model': profile.model || '',
      '#profile-base-url': profile.baseUrl || '',
      '#profile-auth-ref': profile.authRef || '',
      '#profile-enabled': String(!!profile.enabled),
    };
    Object.keys(map).forEach(selector => {
      const el = $(selector);
      if (!el) return;
      el.value = map[selector];
    });
    updateProfileEditorUI();
  }

  function syncProfileTestHostOptions(hosts) {
    const select = $('#profile-test-host');
    if (!select) return;
    const current = String(select.value || '').trim();
    select.textContent = '';
    const auto = document.createElement('option');
    auto.value = '';
    auto.textContent = 'auto (first host)';
    select.appendChild(auto);
    const list = Array.isArray(hosts) ? hosts : [];
    list.forEach(host => {
      const id = String(host && host.id ? host.id : '').trim();
      if (!id) return;
      const option = document.createElement('option');
      option.value = id;
      option.textContent = String(host && (host.name || host.id) ? (host.name || host.id) : id);
      select.appendChild(option);
    });
    const hasCurrent = list.some(host => String(host && host.id ? host.id : '') === current);
    select.value = hasCurrent ? current : '';
  }

  function resolveSelectedProfileTestHostID() {
    const select = $('#profile-test-host');
    const selected = String(select && select.value ? select.value : '').trim();
    if (selected) return selected;
    if (remoteHostsCache.length) {
      return String(remoteHostsCache[0].id || '').trim();
    }
    return '';
  }

  function extractHostIDFromRemotePath(path) {
    const value = String(path || '').trim();
    const match = /^\/api\/v1\/remote\/hosts\/([^/]+)/.exec(value);
    if (!match || !match[1]) return '';
    try {
      return decodeURIComponent(match[1]);
    } catch (_e) {
      return match[1];
    }
  }

  function pruneServerHostOperationCache(hosts) {
    const list = Array.isArray(hosts) ? hosts : [];
    const next = {};
    list.forEach(host => {
      const key = String(host && host.id ? host.id : '').trim();
      if (!key) return;
      if (serverHostLastOperationByID[key]) {
        next[key] = serverHostLastOperationByID[key];
      }
    });
    serverHostLastOperationByID = next;
  }

  function isServersViewVisible() {
    const view = $('#view-servers');
    return !!(view && !view.classList.contains('hidden'));
  }

  function formatServerHostOperationMetaLines(hostID) {
    const key = String(hostID || '').trim();
    if (!key) return [];
    const op = serverHostLastOperationByID[key];
    if (!op || typeof op !== 'object') return [];
    const lines = [];
    if (op.operation) {
      lines.push('last op: ' + String(op.operation) + ' (' + String(op.success ? 'ok' : 'error') + ')');
    }
    if (op.requestId) {
      lines.push('requestId: ' + String(op.requestId));
    }
    if (op.durationMs != null) {
      const duration = Math.round(Number(op.durationMs) || 0);
      lines.push('duration: ' + String(duration) + 'ms');
    }
    if (!op.success && op.error) {
      lines.push('last error: ' + String(op.error));
    }
    if (op.at) {
      lines.push('updated at: ' + String(op.at));
    }
    return lines;
  }

  function renderServerManageOperationMeta() {
    const el = $('#server-manage-op-meta');
    if (!el) return;
    const meta = serverManageLastOperation;
    if (!meta || typeof meta !== 'object') {
      el.textContent = '';
      return;
    }
    const pieces = [];
    pieces.push('operation=' + String(meta.operation || '-'));
    pieces.push('status=' + String(meta.success ? 'ok' : 'error'));
    if (meta.requestId) {
      pieces.push('requestId=' + String(meta.requestId));
    }
    if (meta.durationMs != null) {
      pieces.push('duration=' + String(Math.round(Number(meta.durationMs) || 0)) + 'ms');
    }
    if (meta.path) {
      pieces.push('path=' + String(meta.path));
    }
    if (meta.error) {
      pieces.push('error=' + String(meta.error));
    }
    if (meta.at) {
      pieces.push('at=' + String(meta.at));
    }
    el.textContent = pieces.join(' · ');
  }

  function setServerManageOperationMeta(meta) {
    if (!meta || typeof meta !== 'object') {
      serverManageLastOperation = null;
      renderServerManageOperationMeta();
      return;
    }
    serverManageLastOperation = {
      operation: String(meta.operation || ''),
      success: !!meta.success,
      requestId: String(meta.requestId || ''),
      durationMs: Number(meta.durationMs || 0),
      path: String(meta.path || ''),
      error: String(meta.error || ''),
      at: new Date().toISOString(),
    };
    const hostID = extractHostIDFromRemotePath(serverManageLastOperation.path);
    if (hostID) {
      serverHostLastOperationByID[hostID] = {
        operation: serverManageLastOperation.operation,
        success: serverManageLastOperation.success,
        requestId: serverManageLastOperation.requestId,
        durationMs: serverManageLastOperation.durationMs,
        error: serverManageLastOperation.error,
        at: serverManageLastOperation.at,
      };
      if (isServersViewVisible()) {
        renderServersList(remoteHostsCache);
      }
    }
    renderServerManageOperationMeta();
  }

  async function serverManageAPI(method, path, body, operationName) {
    const startedAt = performance.now();
    try {
      const payload = await api(method, path, body);
      const requestID = payload && (payload.requestId || payload.requestID) ? (payload.requestId || payload.requestID) : '';
      setServerManageOperationMeta({
        operation: operationName,
        success: true,
        requestId: requestID,
        durationMs: performance.now() - startedAt,
        path: path,
      });
      return payload;
    } catch (e) {
      setServerManageOperationMeta({
        operation: operationName,
        success: false,
        durationMs: performance.now() - startedAt,
        path: path,
        error: e && e.message ? e.message : String(e),
      });
      throw e;
    }
  }

  function getServerManageAgentID() {
    const input = $('#server-manage-agent-id');
    const value = input && input.value ? input.value.trim() : '';
    return value || 'main';
  }

  function getServerManageLogTail() {
    const input = $('#server-manage-log-tail');
    const raw = input && input.value ? input.value.trim() : '';
    const parsed = parseInt(raw || '200', 10);
    if (!Number.isFinite(parsed) || parsed <= 0) return 200;
    if (parsed > 4000) return 4000;
    return parsed;
  }

  function pickRemoteInstanceAgentID(instance) {
    if (!instance || typeof instance !== 'object') return 'main';
    const fromAgentID = String(instance.agentId || instance.agentID || '').trim();
    if (fromAgentID) return fromAgentID;
    const rawID = String(instance.id || instance.ID || '').trim();
    if (rawID.includes(':')) {
      const tail = rawID.split(':').pop();
      if (tail && tail.trim()) return tail.trim();
    }
    if (rawID) return rawID;
    return 'main';
  }

  function renderServerManageInstances(entries) {
    const out = $('#server-manage-instances');
    if (!out) return;
    const list = Array.isArray(entries) ? entries : [];
    if (!list.length) {
      out.textContent = 'No instances found.';
      return;
    }
    out.textContent = list.map(item => {
      const agentID = pickRemoteInstanceAgentID(item);
      const runtimeState = String(item && (item.runtimeState || item.runtime_state || item.status) ? (item.runtimeState || item.runtime_state || item.status) : 'unknown');
      const health = String(item && item.health ? item.health : 'unknown');
      return agentID + ' (runtime=' + runtimeState + ', health=' + health + ')';
    }).join('\n');
  }

  function renderServerManageInstanceStatus(instance, steps) {
    const out = $('#server-manage-instance-status-out');
    if (!out) return;
    const current = instance && typeof instance === 'object' ? instance : {};
    const list = Array.isArray(steps) ? steps : [];
    out.textContent = '';

    if (!Object.keys(current).length && !list.length) {
      const empty = document.createElement('div');
      empty.className = 'text-dim';
      empty.textContent = 'No instance status loaded.';
      out.appendChild(empty);
      return;
    }

    const agentID = pickRemoteInstanceAgentID(current) || getServerManageAgentID();
    const runtimeState = String(current.runtimeState || current.runtime_state || current.status || '').trim();
    const health = String(current.health || '').trim();
    const installed = typeof current.installed === 'boolean' ? current.installed : null;
    const repaired = typeof current.repaired === 'boolean' ? current.repaired : null;
    const gatewayHealthy = typeof current.gatewayHealthy === 'boolean' ? current.gatewayHealthy : null;

    let badgeState = 'warn';
    let badgeText = 'unknown';
    if (health.toLowerCase() === 'healthy' || runtimeState.toLowerCase() === 'running') {
      badgeState = 'ok';
      badgeText = 'healthy';
    } else if (
      health.toLowerCase() === 'unhealthy' ||
      runtimeState.toLowerCase() === 'error' ||
      runtimeState.toLowerCase() === 'stopped'
    ) {
      badgeState = 'bad';
      badgeText = 'unhealthy';
    }
    if (installed === true) {
      badgeState = 'ok';
      badgeText = 'installed';
    }
    if (repaired === true) {
      badgeState = 'ok';
      badgeText = 'repaired';
    }

    const statusCard = document.createElement('div');
    statusCard.className = 'manage-card';
    const header = document.createElement('div');
    header.className = 'manage-card-header';
    const title = document.createElement('div');
    title.className = 'manage-card-title';
    title.textContent = 'Instance ' + agentID;
    const pill = document.createElement('span');
    pill.className = 'manage-pill manage-pill-' + badgeState;
    pill.textContent = badgeText;
    header.appendChild(title);
    header.appendChild(pill);
    statusCard.appendChild(header);

    function appendKV(label, value) {
      if (String(value || '').trim() === '') return;
      const row = document.createElement('div');
      row.className = 'manage-kv';
      const labelSpan = document.createElement('span');
      labelSpan.className = 'manage-kv-label';
      labelSpan.textContent = label + ': ';
      const valueSpan = document.createElement('span');
      valueSpan.textContent = String(value);
      row.appendChild(labelSpan);
      row.appendChild(valueSpan);
      statusCard.appendChild(row);
    }

    appendKV('id', current.id || current.ID || '');
    appendKV('runtime', runtimeState || 'unknown');
    appendKV('health', health || 'unknown');
    if (installed !== null) appendKV('install status', installed ? 'installed' : 'not installed');
    if (repaired !== null) appendKV('repair status', repaired ? 'repaired' : 'not repaired');
    if (gatewayHealthy !== null) appendKV('gateway', gatewayHealthy ? 'healthy' : 'unhealthy');
    out.appendChild(statusCard);

    const stepsCard = document.createElement('div');
    stepsCard.className = 'manage-card';
    const stepsHeader = document.createElement('div');
    stepsHeader.className = 'manage-card-header';
    const stepsTitle = document.createElement('div');
    stepsTitle.className = 'manage-card-title';
    stepsTitle.textContent = 'Execution Steps';
    const stepsCount = document.createElement('span');
    stepsCount.className = 'manage-pill ' + (list.length ? 'manage-pill-warn' : 'manage-pill-ok');
    stepsCount.textContent = String(list.length);
    stepsHeader.appendChild(stepsTitle);
    stepsHeader.appendChild(stepsCount);
    stepsCard.appendChild(stepsHeader);

    if (!list.length) {
      const empty = document.createElement('div');
      empty.className = 'text-dim';
      empty.textContent = 'No execution steps returned.';
      stepsCard.appendChild(empty);
    } else {
      list.forEach((step, index) => {
        const item = step && typeof step === 'object' ? step : {};
        const command = String(item.command || item.Command || '').trim();
        const exitCode = Number(item.exitCode != null ? item.exitCode : item.ExitCode);
        const durationMs = Number(item.durationMs != null ? item.durationMs : item.DurationMs);
        const stdout = String(item.stdout || item.Stdout || '').trim();
        const stderr = String(item.stderr || item.Stderr || '').trim();

        const block = document.createElement('div');
        block.className = 'manage-step';

        const cmd = document.createElement('div');
        cmd.className = 'manage-step-command';
        cmd.textContent = command || '(no command)';
        block.appendChild(cmd);

        const meta = document.createElement('div');
        meta.className = 'manage-step-meta';
        const indexTag = document.createElement('span');
        indexTag.textContent = '#' + String(index + 1);
        const exitTag = document.createElement('span');
        exitTag.textContent = 'exit=' + (Number.isFinite(exitCode) ? String(exitCode) : '-');
        const durationTag = document.createElement('span');
        durationTag.textContent = 'duration=' + (Number.isFinite(durationMs) ? String(Math.round(durationMs)) + 'ms' : '-');
        meta.appendChild(indexTag);
        meta.appendChild(exitTag);
        meta.appendChild(durationTag);
        block.appendChild(meta);

        if (stdout || stderr) {
          const details = document.createElement('details');
          details.className = 'manage-step-details';
          const summary = document.createElement('summary');
          summary.textContent = 'Output';
          const pre = document.createElement('pre');
          pre.className = 'code-block';
          pre.textContent = (stdout ? '[stdout]\n' + stdout + '\n' : '') + (stderr ? '[stderr]\n' + stderr : '');
          details.appendChild(summary);
          details.appendChild(pre);
          block.appendChild(details);
        }

        stepsCard.appendChild(block);
      });
    }
    out.appendChild(stepsCard);
  }

  function renderServerManageLogs(logs) {
    const out = $('#server-manage-logs');
    if (!out) return;
    const text = String(logs || '').trim();
    out.textContent = '';
    if (!text) {
      const empty = document.createElement('div');
      empty.className = 'text-dim';
      empty.textContent = 'No logs available.';
      out.appendChild(empty);
      return;
    }

    const sections = [];
    const lines = text.split('\n');
    let current = { title: 'combined.log', lines: [] };
    const heading = /^---\s+(.+?)\s+---$/;
    lines.forEach(line => {
      const match = line.match(heading);
      if (match) {
        if (current.lines.length) {
          sections.push(current);
        }
        current = { title: match[1], lines: [] };
        return;
      }
      current.lines.push(line);
    });
    if (current.lines.length || !sections.length) {
      sections.push(current);
    }

    sections.forEach((section, index) => {
      const wrapper = document.createElement('div');
      wrapper.className = 'manage-card';

      const details = document.createElement('details');
      details.className = 'manage-log-section';
      if (index === 0) details.open = true;
      const summary = document.createElement('summary');
      const lineCount = section.lines.filter(Boolean).length;
      summary.textContent = section.title + ' (' + String(lineCount) + ' lines)';

      const body = document.createElement('pre');
      body.className = 'code-block';
      body.textContent = section.lines.join('\n').trim() || '(empty)';

      details.appendChild(summary);
      details.appendChild(body);
      wrapper.appendChild(details);
      out.appendChild(wrapper);
    });
  }

  function renderServerManageSessions(entries) {
    const out = $('#server-manage-sessions');
    if (!out) return;
    const list = Array.isArray(entries) ? entries : [];
    if (!list.length) {
      out.textContent = 'No sessions found.';
      return;
    }
    out.textContent = list.map(item => {
      const sid = item && (item.sessionId || item.sessionID) ? (item.sessionId || item.sessionID) : '-';
      const kind = item && item.kind ? item.kind : '-';
      const size = item && item.sizeBytes != null ? item.sizeBytes : 0;
      const modifiedAt = item && item.modifiedAt != null ? item.modifiedAt : 0;
      const path = item && item.path ? item.path : '-';
      return String(sid) + ' [' + String(kind) + '] size=' + String(size) + ' modified=' + String(modifiedAt) + '\n' + String(path);
    }).join('\n\n');
  }

  function renderServerManageMemory(entries) {
    const out = $('#server-manage-memory');
    if (!out) return;
    const list = Array.isArray(entries) ? entries : [];
    if (!list.length) {
      out.textContent = 'No memory files found.';
      return;
    }
    out.textContent = list.map(item => {
      const path = item && item.path ? item.path : '-';
      const size = item && item.sizeBytes != null ? item.sizeBytes : 0;
      const modifiedAt = item && item.modifiedAt != null ? item.modifiedAt : 0;
      return String(path) + ' (size=' + String(size) + ', modified=' + String(modifiedAt) + ')';
    }).join('\n');
  }

  function showServerManagePanel(hostID) {
    const card = $('#server-manage-card');
    const hostLabel = $('#server-manage-host-label');
    if (!card || !hostLabel) return;
    const key = String(hostID || '').trim();
    if (!key) {
      card.classList.add('hidden');
      serverManageHostID = '';
      return;
    }
    serverManageHostID = key;
    const host = remoteHostsCache.find(item => String(item && item.id ? item.id : '') === key) || null;
    const displayName = host ? (host.name || host.id || key) : key;
    hostLabel.textContent = 'Selected host: ' + displayName + ' (' + key + ')';
    card.classList.remove('hidden');
    renderServerManageInstances([]);
    renderServerManageInstanceStatus({}, []);
    renderServerManageLogs('');
    renderServerManageSessions([]);
    renderServerManageMemory([]);
    setServerManageOperationMeta(null);
    setMsg('#server-manage-msg', '', 'info');
  }

  function validateServerManageTarget() {
    const hostID = getServerManageHostID();
    if (!hostID) {
      setMsg('#server-manage-msg', 'Select a server first from the list.', 'error');
      return '';
    }
    return hostID;
  }

  function setServerManageControlsDisabled(disabled) {
    const ids = [
      'server-manage-load-instances',
      'server-manage-instance-status',
      'server-manage-install-instance',
      'server-manage-repair-instance',
      'server-manage-load-logs',
      'server-manage-load-config',
      'server-manage-apply-config',
      'server-manage-load-sessions',
      'server-manage-archive-session',
      'server-manage-delete-session',
      'server-manage-load-memory',
      'server-manage-agent-id',
      'server-manage-session-id',
      'server-manage-log-tail',
      'server-manage-config',
    ];
    ids.forEach(id => {
      const el = $('#' + id);
      if (el) el.disabled = !!disabled;
    });
  }

  function renderServerManageProgress(operation, target) {
    const out = $('#server-manage-instance-status-out');
    if (!out) return;
    out.textContent = '';
    const card = document.createElement('div');
    card.className = 'manage-card';

    const header = document.createElement('div');
    header.className = 'manage-card-header';
    const title = document.createElement('div');
    title.className = 'manage-card-title';
    title.textContent = String(operation || 'Operation');
    const pill = document.createElement('span');
    pill.className = 'manage-pill manage-pill-warn';
    pill.textContent = 'running';
    header.appendChild(title);
    header.appendChild(pill);
    card.appendChild(header);

    const body = document.createElement('div');
    body.className = 'manage-kv';
    body.textContent = 'Target: ' + String(target || '-');
    card.appendChild(body);
    out.appendChild(card);
  }

  async function runServerManageOperation(_label, runner) {
    if (serverManageOperationRunning) {
      setMsg('#server-manage-msg', 'Another operation is already running.', 'info');
      return;
    }
    serverManageOperationRunning = true;
    setServerManageControlsDisabled(true);
    try {
      await runner();
    } finally {
      serverManageOperationRunning = false;
      setServerManageControlsDisabled(false);
    }
  }

  async function loadServerManageConfig(options) {
    const opts = options && typeof options === 'object' ? options : {};
    const silent = !!opts.silent;
    const skipLock = !!opts.skipLock;
    const hostID = validateServerManageTarget();
    if (!hostID) return;
    const task = async () => {
      renderServerManageProgress('Load Config', hostID);
      if (!silent) setMsg('#server-manage-msg', 'Loading config for host ' + hostID + '...', 'info');
      try {
        const path = '/api/v1/remote/hosts/' + encodeURIComponent(hostID) + '/config';
        const payload = await serverManageAPI('GET', path, null, 'load_config');
        const config = payload && payload.config && typeof payload.config === 'object' ? payload.config : {};
        const editor = $('#server-manage-config');
        if (editor) editor.value = JSON.stringify(config, null, 2);
        setMsg('#server-manage-msg', 'Config loaded for host ' + hostID + '.', 'success');
      } catch (e) {
        setMsg('#server-manage-msg', 'Load config failed: ' + e.message, 'error');
      }
    };
    if (skipLock) {
      await task();
      return;
    }
    await runServerManageOperation('load-config', task);
  }

  async function applyServerManageConfigPatch() {
    const hostID = validateServerManageTarget();
    if (!hostID) return;
    const editor = $('#server-manage-config');
    const raw = editor && typeof editor.value === 'string' ? editor.value.trim() : '';
    if (!raw) {
      setMsg('#server-manage-msg', 'Config patch cannot be empty.', 'error');
      return;
    }
    let patch = null;
    try {
      patch = JSON.parse(raw);
    } catch (e) {
      setMsg('#server-manage-msg', 'Config patch must be valid JSON: ' + e.message, 'error');
      return;
    }
    if (!patch || typeof patch !== 'object' || Array.isArray(patch)) {
      setMsg('#server-manage-msg', 'Config patch must be a JSON object.', 'error');
      return;
    }
    await runServerManageOperation('patch-config', async () => {
      renderServerManageProgress('Apply Config Patch', hostID);
      setMsg('#server-manage-msg', 'Applying config patch for host ' + hostID + '...', 'info');
      try {
        const path = '/api/v1/remote/hosts/' + encodeURIComponent(hostID) + '/config';
        const payload = await serverManageAPI('PATCH', path, patch, 'patch_config');
        const config = payload && payload.config && typeof payload.config === 'object' ? payload.config : patch;
        if (editor) editor.value = JSON.stringify(config, null, 2);
        setMsg('#server-manage-msg', 'Config patch applied for host ' + hostID + '.', 'success');
      } catch (e) {
        setMsg('#server-manage-msg', 'Apply config patch failed: ' + e.message, 'error');
      }
    });
  }

  async function loadServerManageSessions(options) {
    const opts = options && typeof options === 'object' ? options : {};
    const silent = !!opts.silent;
    const skipLock = !!opts.skipLock;
    const hostID = validateServerManageTarget();
    if (!hostID) return;
    const agentID = getServerManageAgentID();
    const task = async () => {
      renderServerManageProgress('Load Sessions', hostID + ':' + agentID);
      if (!silent) setMsg('#server-manage-msg', 'Loading sessions for ' + agentID + '...', 'info');
      try {
        const path = '/api/v1/remote/hosts/' + encodeURIComponent(hostID) + '/sessions?agentId=' + encodeURIComponent(agentID);
        const payload = await serverManageAPI('GET', path, null, 'load_sessions');
        const sessions = payload && Array.isArray(payload.sessions) ? payload.sessions : [];
        renderServerManageSessions(sessions);
        if (!silent) {
          setMsg('#server-manage-msg', 'Loaded ' + String(sessions.length) + ' sessions for ' + agentID + '.', 'success');
        }
      } catch (e) {
        setMsg('#server-manage-msg', 'Load sessions failed: ' + e.message, 'error');
      }
    };
    if (skipLock) {
      await task();
      return;
    }
    await runServerManageOperation('load-sessions', task);
  }

  async function applyServerManageSessionAction(action) {
    const hostID = validateServerManageTarget();
    if (!hostID) return;
    const agentID = getServerManageAgentID();
    const sessionInput = $('#server-manage-session-id');
    const sessionID = sessionInput && sessionInput.value ? sessionInput.value.trim() : '';
    if (!sessionID) {
      setMsg('#server-manage-msg', 'session id is required.', 'error');
      return;
    }
    const normalizedAction = String(action || '').trim().toLowerCase();
    if (normalizedAction !== 'archive' && normalizedAction !== 'delete') {
      return;
    }
    await runServerManageOperation('session-action', async () => {
      renderServerManageProgress((normalizedAction === 'archive' ? 'Archive Session' : 'Delete Session'), hostID + ':' + agentID + ':' + sessionID);
      setMsg('#server-manage-msg', 'Session ' + normalizedAction + ' in progress for ' + sessionID + '...', 'info');
      try {
        const path = '/api/v1/remote/hosts/' + encodeURIComponent(hostID) + '/sessions/' + encodeURIComponent(sessionID) + '/' + normalizedAction + '?agentId=' + encodeURIComponent(agentID);
        await serverManageAPI('POST', path, {}, 'session_' + normalizedAction);
        setMsg('#server-manage-msg', 'Session ' + sessionID + ' ' + normalizedAction + 'd.', 'success');
        await loadServerManageSessions({ skipLock: true, silent: true });
      } catch (e) {
        setMsg('#server-manage-msg', 'Session ' + normalizedAction + ' failed: ' + e.message, 'error');
      }
    });
  }

  async function loadServerManageMemory(options) {
    const opts = options && typeof options === 'object' ? options : {};
    const silent = !!opts.silent;
    const skipLock = !!opts.skipLock;
    const hostID = validateServerManageTarget();
    if (!hostID) return;
    const agentID = getServerManageAgentID();
    const task = async () => {
      renderServerManageProgress('Load Memory', hostID + ':' + agentID);
      if (!silent) setMsg('#server-manage-msg', 'Loading memory for ' + agentID + '...', 'info');
      try {
        const path = '/api/v1/remote/hosts/' + encodeURIComponent(hostID) + '/memory?agentId=' + encodeURIComponent(agentID);
        const payload = await serverManageAPI('GET', path, null, 'load_memory');
        const memory = payload && Array.isArray(payload.memory) ? payload.memory : [];
        renderServerManageMemory(memory);
        if (!silent) {
          setMsg('#server-manage-msg', 'Loaded ' + String(memory.length) + ' memory entries for ' + agentID + '.', 'success');
        }
      } catch (e) {
        setMsg('#server-manage-msg', 'Load memory failed: ' + e.message, 'error');
      }
    };
    if (skipLock) {
      await task();
      return;
    }
    await runServerManageOperation('load-memory', task);
  }

  async function loadServerManageInstances(options) {
    const opts = options && typeof options === 'object' ? options : {};
    const silent = !!opts.silent;
    const skipLock = !!opts.skipLock;
    const hostID = validateServerManageTarget();
    if (!hostID) return;
    const task = async () => {
      if (!silent) {
        renderServerManageProgress('Load Instances', hostID);
      }
      if (!silent) setMsg('#server-manage-msg', 'Loading instances for host ' + hostID + '...', 'info');
      try {
        const path = '/api/v1/remote/hosts/' + encodeURIComponent(hostID) + '/instances';
        const payload = await serverManageAPI('GET', path, null, 'load_instances');
        const instances = payload && Array.isArray(payload.instances) ? payload.instances : [];
        renderServerManageInstances(instances);
        if (instances.length) {
          const agentInput = $('#server-manage-agent-id');
          if (agentInput && !String(agentInput.value || '').trim()) {
            agentInput.value = pickRemoteInstanceAgentID(instances[0]);
          }
        }
        if (!silent) {
          setMsg('#server-manage-msg', 'Loaded ' + String(instances.length) + ' instances.', 'success');
        }
      } catch (e) {
        setMsg('#server-manage-msg', 'Load instances failed: ' + e.message, 'error');
      }
    };
    if (skipLock) {
      await task();
      return;
    }
    await runServerManageOperation('load-instances', task);
  }

  function validateServerManageInstanceTarget() {
    const hostID = validateServerManageTarget();
    if (!hostID) return { hostID: '', agentID: '' };
    const agentID = getServerManageAgentID();
    if (!agentID) {
      setMsg('#server-manage-msg', 'agent id is required.', 'error');
      return { hostID: '', agentID: '' };
    }
    return { hostID: hostID, agentID: agentID };
  }

  async function loadServerManageInstanceStatus(options) {
    const opts = options && typeof options === 'object' ? options : {};
    const skipLock = !!opts.skipLock;
    const target = validateServerManageInstanceTarget();
    if (!target.hostID || !target.agentID) return;
    const task = async () => {
      renderServerManageProgress('Load Instance Status', target.hostID + ':' + target.agentID);
      setMsg('#server-manage-msg', 'Loading instance status for ' + target.agentID + '...', 'info');
      try {
        const path = '/api/v1/remote/hosts/' + encodeURIComponent(target.hostID) + '/instances/' + encodeURIComponent(target.agentID) + '/status';
        const payload = await serverManageAPI('GET', path, null, 'instance_status');
        renderServerManageInstanceStatus(payload.instance || {}, payload.steps || []);
        setMsg('#server-manage-msg', 'Loaded instance status for ' + target.agentID + '.', 'success');
      } catch (e) {
        setMsg('#server-manage-msg', 'Load instance status failed: ' + e.message, 'error');
      }
    };
    if (skipLock) {
      await task();
      return;
    }
    await runServerManageOperation('instance-status', task);
  }

  async function installServerManageInstance() {
    const target = validateServerManageInstanceTarget();
    if (!target.hostID || !target.agentID) return;
    await runServerManageOperation('install', async () => {
      renderServerManageProgress('Install', target.hostID + ':' + target.agentID);
      setMsg('#server-manage-msg', 'Install in progress for ' + target.agentID + '...', 'info');
      try {
        const path = '/api/v1/remote/hosts/' + encodeURIComponent(target.hostID) + '/instances/' + encodeURIComponent(target.agentID) + '/install';
        const payload = await serverManageAPI('POST', path, {}, 'instance_install');
        renderServerManageInstanceStatus(payload.install || {}, payload.steps || []);
        setMsg('#server-manage-msg', 'Install completed for ' + target.agentID + '.', 'success');
        await loadServerManageInstances({ silent: true, skipLock: true });
      } catch (e) {
        setMsg('#server-manage-msg', 'Install failed: ' + e.message, 'error');
      }
    });
  }

  async function repairServerManageInstance() {
    const target = validateServerManageInstanceTarget();
    if (!target.hostID || !target.agentID) return;
    await runServerManageOperation('repair', async () => {
      renderServerManageProgress('Repair', target.hostID + ':' + target.agentID);
      setMsg('#server-manage-msg', 'Repair in progress for ' + target.agentID + '...', 'info');
      try {
        const path = '/api/v1/remote/hosts/' + encodeURIComponent(target.hostID) + '/instances/' + encodeURIComponent(target.agentID) + '/repair';
        const payload = await serverManageAPI('POST', path, {}, 'instance_repair');
        renderServerManageInstanceStatus(payload.repair || {}, payload.steps || []);
        setMsg('#server-manage-msg', 'Repair completed for ' + target.agentID + '.', 'success');
        await loadServerManageInstances({ silent: true, skipLock: true });
      } catch (e) {
        setMsg('#server-manage-msg', 'Repair failed: ' + e.message, 'error');
      }
    });
  }

  async function loadServerManageLogs(options) {
    const opts = options && typeof options === 'object' ? options : {};
    const skipLock = !!opts.skipLock;
    const target = validateServerManageInstanceTarget();
    if (!target.hostID || !target.agentID) return;
    const tail = getServerManageLogTail();
    const task = async () => {
      renderServerManageProgress('Load Logs', target.hostID + ':' + target.agentID);
      setMsg('#server-manage-msg', 'Loading logs for ' + target.agentID + ' (tail=' + String(tail) + ')...', 'info');
      try {
        const path = '/api/v1/remote/hosts/' + encodeURIComponent(target.hostID) + '/instances/' + encodeURIComponent(target.agentID) + '/logs?tail=' + encodeURIComponent(String(tail));
        const payload = await serverManageAPI('GET', path, null, 'instance_logs');
        renderServerManageLogs(payload.logs || '');
        setMsg('#server-manage-msg', 'Loaded logs for ' + target.agentID + ' (tail=' + String(tail) + ').', 'success');
      } catch (e) {
        setMsg('#server-manage-msg', 'Load logs failed: ' + e.message, 'error');
      }
    };
    if (skipLock) {
      await task();
      return;
    }
    await runServerManageOperation('load-logs', task);
  }

  function renderServersList(hosts) {
    const wrap = $('#servers-list');
    if (!wrap) return;
    const hostOperations = serverHostLastOperationByID;
    const byID = new Map(
      (Array.isArray(hosts) ? hosts : []).map(host => [String(host && host.id ? host.id : ''), host]),
    );

    async function handleCheck(hostID) {
      const key = String(hostID || '').trim();
      if (!key) return;
      const host = byID.get(key) || {};
      await runServerManageOperation('host-check', async () => {
        if (getServerManageHostID() === key) {
          renderServerManageProgress('Host Health Check', key);
        }
        setMsg('#servers-msg', 'Running health check: ' + (host.name || host.id || key) + '...', 'info');
        try {
          const path = '/api/v1/remote/hosts/' + encodeURIComponent(key) + '/check';
          await serverManageAPI('POST', path, {}, 'host_check');
          setMsg('#servers-msg', 'Health check completed: ' + (host.name || host.id || key), 'success');
          if (getServerManageHostID() === key) {
            setMsg('#server-manage-msg', 'Host health check completed.', 'success');
          }
          await initServers();
        } catch (e) {
          setMsg('#servers-msg', 'Health check failed: ' + e.message, 'error');
          if (getServerManageHostID() === key) {
            setMsg('#server-manage-msg', 'Host health check failed: ' + e.message, 'error');
          }
        }
      });
    }

    async function handleDelete(hostID) {
      const key = String(hostID || '').trim();
      if (!key) return;
      const host = byID.get(key) || {};
      if (!window.confirm('Delete remote host ' + (host.name || host.id || key) + '?')) return;
      try {
        await api('DELETE', '/api/v1/remote/hosts/' + encodeURIComponent(key));
        delete serverHostLastOperationByID[key];
        if (String(serverEditingHostID || '') === key) {
          resetServerEditor(true);
        }
        setMsg('#servers-msg', 'Deleted remote host: ' + (host.name || host.id || key), 'success');
        await initServers();
      } catch (e) {
        setMsg('#servers-msg', 'Delete failed: ' + e.message, 'error');
      }
    }

    function handleManage(hostID) {
      showServerManagePanel(hostID);
    }

    function handleEdit(hostID) {
      beginServerEdit(hostID);
      setMsg('#servers-msg', 'Editing remote host: ' + String(hostID || ''), 'info');
    }

    const island = window.CarrierRemoteControlIslands;
    if (
      island &&
      typeof island.renderServersList === 'function' &&
      island.renderServersList(wrap, hosts, {
        onCheck: handleCheck,
        onManage: handleManage,
        onEdit: handleEdit,
        onDelete: handleDelete,
      }, hostOperations)
    ) {
      return;
    }

    wrap.textContent = '';
    if (!hosts.length) {
      const empty = document.createElement('div');
      empty.className = 'card';
      empty.textContent = 'No remote servers configured.';
      wrap.appendChild(empty);
      return;
    }
    hosts.forEach(host => {
      const card = document.createElement('div');
      card.className = 'agent-card';
      const title = document.createElement('h4');
      title.textContent = host.name || host.id;
      const meta = document.createElement('div');
      meta.className = 'instance-meta';
      const endpoint = host.authMode === 'ssh_config' ? (host.sshConfigHost || host.host || '-') : (host.user || 'user') + '@' + (host.host || '-');
      const lines = [
        'id: ' + (host.id || '-'),
        'endpoint: ' + endpoint,
        'auth: ' + (host.authMode || '-'),
        'runtime: ' + (host.runtimeMode || '-'),
        'health: ' + (host.lastHealth || 'unknown'),
      ];
      lines.push(...formatServerHostOperationMetaLines(host.id));
      meta.textContent = lines.join('\n');
      meta.style.whiteSpace = 'pre-line';
      const actions = document.createElement('div');
      actions.className = 'btn-row';

      const checkBtn = document.createElement('button');
      checkBtn.className = 'btn-sm btn-secondary';
      checkBtn.textContent = 'Check';
      checkBtn.onclick = async () => {
        checkBtn.disabled = true;
        await handleCheck(host.id);
        checkBtn.disabled = false;
      };

      const deleteBtn = document.createElement('button');
      deleteBtn.className = 'btn-sm btn-danger';
      deleteBtn.textContent = 'Delete';
      deleteBtn.onclick = () => handleDelete(host.id);

      const manageBtn = document.createElement('button');
      manageBtn.className = 'btn-sm';
      manageBtn.textContent = 'Manage';
      manageBtn.onclick = () => handleManage(host.id);

      const editBtn = document.createElement('button');
      editBtn.className = 'btn-sm btn-secondary';
      editBtn.textContent = 'Edit';
      editBtn.onclick = () => handleEdit(host.id);

      actions.appendChild(checkBtn);
      actions.appendChild(manageBtn);
      actions.appendChild(editBtn);
      actions.appendChild(deleteBtn);
      card.appendChild(title);
      card.appendChild(meta);
      card.appendChild(actions);
      wrap.appendChild(card);
    });
  }

  async function initServers() {
    showView('servers');
    $('#nav').classList.remove('hidden');

    const authMode = $('#server-auth-mode');
    const refreshBtn = $('#servers-refresh');
    const saveBtn = $('#server-save');
    const cancelEditBtn = $('#server-cancel-edit');
    const manageCard = $('#server-manage-card');
    const loadInstancesBtn = $('#server-manage-load-instances');
    const instanceStatusBtn = $('#server-manage-instance-status');
    const installInstanceBtn = $('#server-manage-install-instance');
    const repairInstanceBtn = $('#server-manage-repair-instance');
    const loadLogsBtn = $('#server-manage-load-logs');
    const loadConfigBtn = $('#server-manage-load-config');
    const applyConfigBtn = $('#server-manage-apply-config');
    const loadSessionsBtn = $('#server-manage-load-sessions');
    const archiveSessionBtn = $('#server-manage-archive-session');
    const deleteSessionBtn = $('#server-manage-delete-session');
    const loadMemoryBtn = $('#server-manage-load-memory');
    const instancesOut = $('#server-manage-instances');
    const instanceStatusOut = $('#server-manage-instance-status-out');
    const logsOut = $('#server-manage-logs');
    const sessionsOut = $('#server-manage-sessions');
    const memoryOut = $('#server-manage-memory');

    function syncServerEditSelection(hosts) {
      const list = Array.isArray(hosts) ? hosts : [];
      const current = String(serverEditingHostID || '').trim();
      if (!current) return;
      const exists = list.some(host => String(host && host.id ? host.id : '') === current);
      if (!exists) {
        resetServerEditor(false);
      }
    }

    function syncManageSelection(hosts) {
      const list = Array.isArray(hosts) ? hosts : [];
      const current = getServerManageHostID();
      if (!current) {
        if (manageCard) manageCard.classList.add('hidden');
        return;
      }
      const exists = list.some(host => String(host && host.id ? host.id : '') === current);
      if (!exists) {
        serverManageHostID = '';
        if (manageCard) manageCard.classList.add('hidden');
        if (instancesOut) instancesOut.textContent = '';
        if (instanceStatusOut) instanceStatusOut.textContent = '';
        if (logsOut) logsOut.textContent = '';
        if (sessionsOut) sessionsOut.textContent = '';
        if (memoryOut) memoryOut.textContent = '';
        setServerManageOperationMeta(null);
        setMsg('#server-manage-msg', '', 'info');
        return;
      }
      showServerManagePanel(current);
    }

    authMode.onchange = syncServerAuthModeInputs;
    syncServerAuthModeInputs();
    updateServerEditorUI();
    setServerManageControlsDisabled(false);

    if (loadInstancesBtn) loadInstancesBtn.onclick = () => { loadServerManageInstances(); };
    if (instanceStatusBtn) instanceStatusBtn.onclick = () => { loadServerManageInstanceStatus(); };
    if (installInstanceBtn) installInstanceBtn.onclick = () => { installServerManageInstance(); };
    if (repairInstanceBtn) repairInstanceBtn.onclick = () => { repairServerManageInstance(); };
    if (loadLogsBtn) loadLogsBtn.onclick = () => { loadServerManageLogs(); };
    if (loadConfigBtn) loadConfigBtn.onclick = () => { loadServerManageConfig(); };
    if (applyConfigBtn) applyConfigBtn.onclick = () => { applyServerManageConfigPatch(); };
    if (loadSessionsBtn) loadSessionsBtn.onclick = () => { loadServerManageSessions(); };
    if (archiveSessionBtn) archiveSessionBtn.onclick = () => { applyServerManageSessionAction('archive'); };
    if (deleteSessionBtn) deleteSessionBtn.onclick = () => { applyServerManageSessionAction('delete'); };
    if (loadMemoryBtn) loadMemoryBtn.onclick = () => { loadServerManageMemory(); };

    refreshBtn.onclick = async () => {
      try {
        setMsg('#servers-msg', '', 'info');
        const hosts = await fetchRemoteHosts();
        renderServersList(hosts);
        syncManageSelection(hosts);
        syncServerEditSelection(hosts);
      } catch (e) {
        setMsg('#servers-msg', 'Load failed: ' + e.message, 'error');
        renderServersList([]);
        syncManageSelection([]);
        syncServerEditSelection([]);
      }
    };

    saveBtn.onclick = async () => {
      const mode = (authMode.value || '').trim().toLowerCase();
      const payload = {
        name: ($('#server-name').value || '').trim(),
        host: ($('#server-host').value || '').trim(),
        port: parseInt(($('#server-port').value || '22').trim(), 10) || 22,
        user: ($('#server-user').value || '').trim(),
        authMode: mode,
        keyPath: ($('#server-key-path').value || '').trim(),
        sshConfigHost: ($('#server-ssh-config-host').value || '').trim(),
        runtimeMode: ($('#server-runtime-mode').value || 'on_demand').trim(),
      };
      const editingID = String(serverEditingHostID || '').trim();
      try {
        saveBtn.disabled = true;
        if (editingID) {
          await api('PATCH', '/api/v1/remote/hosts/' + encodeURIComponent(editingID), payload);
          setMsg('#servers-msg', 'Remote host updated: ' + editingID, 'success');
        } else {
          await api('POST', '/api/v1/remote/hosts', payload);
          setMsg('#servers-msg', 'Remote host saved.', 'success');
        }
        resetServerEditor(true);
        refreshBtn.click();
      } catch (e) {
        setMsg('#servers-msg', 'Save failed: ' + e.message, 'error');
      } finally {
        saveBtn.disabled = false;
      }
    };

    if (cancelEditBtn) {
      cancelEditBtn.onclick = () => {
        resetServerEditor(true);
        setMsg('#servers-msg', 'Host edit cancelled.', 'info');
      };
    }

    refreshBtn.click();
  }

  function renderProfilesAndBindings(profiles, bindings) {
    const profilesWrap = $('#profiles-list');
    const bindingsWrap = $('#bindings-list');
    if (!profilesWrap || !bindingsWrap) return;
    const profileByID = new Map(
      (Array.isArray(profiles) ? profiles : []).map(profile => [String(profile && profile.id ? profile.id : ''), profile]),
    );
    const bindingByID = new Map(
      (Array.isArray(bindings) ? bindings : []).map(binding => [String(binding && binding.id ? binding.id : ''), binding]),
    );

    async function handleTestProfile(profileID) {
      const key = String(profileID || '').trim();
      if (!key) return;
      const profile = profileByID.get(key) || {};
      try {
        const hostID = resolveSelectedProfileTestHostID();
        const body = hostID ? { hostId: hostID } : {};
        await api('POST', '/api/v1/provider-profiles/' + encodeURIComponent(key) + '/test', body);
        setMsg('#profiles-msg', 'Profile test succeeded: ' + (profile.name || profile.id || key), 'success');
      } catch (e) {
        setMsg('#profiles-msg', 'Profile test failed: ' + e.message, 'error');
      }
    }

    function handleEditProfile(profileID) {
      const key = String(profileID || '').trim();
      if (!key) return;
      beginProfileEdit(key);
      setMsg('#profiles-msg', 'Editing profile: ' + key, 'info');
    }

    async function handleDeleteProfile(profileID) {
      const key = String(profileID || '').trim();
      if (!key) return;
      const profile = profileByID.get(key) || {};
      if (!window.confirm('Delete profile ' + (profile.name || profile.id || key) + '?')) return;
      try {
        await api('DELETE', '/api/v1/provider-profiles/' + encodeURIComponent(key));
        if (String(profileEditingProfileID || '') === key) {
          resetProfileEditor(true);
        }
        setMsg('#profiles-msg', 'Profile deleted: ' + (profile.name || profile.id || key), 'success');
        await initProfiles();
      } catch (e) {
        setMsg('#profiles-msg', 'Delete failed: ' + e.message, 'error');
      }
    }

    async function handleDeleteBinding(bindingID) {
      const key = String(bindingID || '').trim();
      if (!key) return;
      const binding = bindingByID.get(key) || {};
      const label = String(binding && binding.targetType ? binding.targetType : '-') + ': ' + String(binding && binding.targetId ? binding.targetId : key);
      if (!window.confirm('Delete binding ' + label + '?')) return;
      try {
        await api('DELETE', '/api/v1/provider-bindings/' + encodeURIComponent(key));
        setMsg('#profiles-msg', 'Binding deleted: ' + label, 'success');
        await initProfiles();
      } catch (e) {
        setMsg('#profiles-msg', 'Delete binding failed: ' + e.message, 'error');
      }
    }

    const island = window.CarrierRemoteControlIslands;
    if (
      island &&
      typeof island.renderProfilesAndBindings === 'function' &&
      island.renderProfilesAndBindings(profilesWrap, bindingsWrap, profiles, bindings, {
        onTestProfile: handleTestProfile,
        onEditProfile: handleEditProfile,
        onDeleteProfile: handleDeleteProfile,
        onDeleteBinding: handleDeleteBinding,
      })
    ) {
      return;
    }

    profilesWrap.textContent = '';
    bindingsWrap.textContent = '';

    if (!profiles.length) {
      const empty = document.createElement('div');
      empty.className = 'card';
      empty.textContent = 'No provider profiles configured.';
      profilesWrap.appendChild(empty);
    } else {
      profiles.forEach(profile => {
        const card = document.createElement('div');
        card.className = 'agent-card';
        const title = document.createElement('h4');
        title.textContent = profile.name || profile.id;
        const meta = document.createElement('div');
        meta.className = 'instance-meta';
        meta.textContent = 'id: ' + (profile.id || '-') + '\nprovider/model: ' + (profile.provider || '-') + '/' + (profile.model || '-') + '\nenabled: ' + String(profile.enabled);
        meta.style.whiteSpace = 'pre-line';
        const actions = document.createElement('div');
        actions.className = 'btn-row';

        const testBtn = document.createElement('button');
        testBtn.className = 'btn-sm btn-secondary';
        testBtn.textContent = 'Test';
        testBtn.onclick = () => handleTestProfile(profile.id);

        const editBtn = document.createElement('button');
        editBtn.className = 'btn-sm btn-secondary';
        editBtn.textContent = 'Edit';
        editBtn.onclick = () => handleEditProfile(profile.id);

        const deleteBtn = document.createElement('button');
        deleteBtn.className = 'btn-sm btn-danger';
        deleteBtn.textContent = 'Delete';
        deleteBtn.onclick = () => handleDeleteProfile(profile.id);

        actions.appendChild(testBtn);
        actions.appendChild(editBtn);
        actions.appendChild(deleteBtn);
        card.appendChild(title);
        card.appendChild(meta);
        card.appendChild(actions);
        profilesWrap.appendChild(card);
      });
    }

    if (!bindings.length) {
      const empty = document.createElement('div');
      empty.className = 'card';
      empty.textContent = 'No provider bindings configured.';
      bindingsWrap.appendChild(empty);
      return;
    }
    bindings.forEach(binding => {
      const card = document.createElement('div');
      card.className = 'agent-card';
      const title = document.createElement('h4');
      title.textContent = (binding.targetType || '-') + ': ' + (binding.targetId || '-');
      const meta = document.createElement('div');
      meta.className = 'instance-meta';
      meta.textContent = 'id: ' + (binding.id || '-') + '\nprofileId: ' + (binding.profileId || '-') + '\nsyncMode: ' + (binding.syncMode || 'always_push');
      meta.style.whiteSpace = 'pre-line';
      const actions = document.createElement('div');
      actions.className = 'btn-row';
      const deleteBtn = document.createElement('button');
      deleteBtn.className = 'btn-sm btn-danger';
      deleteBtn.textContent = 'Delete';
      deleteBtn.onclick = () => handleDeleteBinding(binding.id);
      card.appendChild(title);
      card.appendChild(meta);
      card.appendChild(actions);
      actions.appendChild(deleteBtn);
      bindingsWrap.appendChild(card);
    });
  }

  async function initProfiles() {
    showView('profiles');
    $('#nav').classList.remove('hidden');

    const refreshBtn = $('#profiles-refresh');
    const saveProfileBtn = $('#profile-save');
    const cancelEditBtn = $('#profile-cancel-edit');
    const saveBindingBtn = $('#binding-save');
    const profileSelect = $('#binding-profile-id');
    const bindingTargetType = $('#binding-target-type');
    const bindingTargetID = $('#binding-target-id');
    const profileTestHostSelect = $('#profile-test-host');

    function syncBindingControls() {
      const enabled = !!featureFlags.providerBindingEnabled;
      profileSelect.disabled = !enabled;
      bindingTargetType.disabled = !enabled;
      bindingTargetID.disabled = !enabled;
      saveBindingBtn.disabled = !enabled;
      if (!enabled) {
        setMsg('#profiles-msg', 'Provider binding is disabled by feature flag.', 'info');
      }
    }

    function syncProfileEditSelection(profiles) {
      const list = Array.isArray(profiles) ? profiles : [];
      const current = String(profileEditingProfileID || '').trim();
      if (!current) return;
      const exists = list.some(profile => String(profile && profile.id ? profile.id : '') === current);
      if (!exists) {
        resetProfileEditor(false);
      }
    }

    async function refreshAll() {
      try {
        if (featureFlags.providerBindingEnabled) {
          setMsg('#profiles-msg', '', 'info');
        }
        const [profilesPayload, bindingsPayload, hosts] = await Promise.all([
          api('GET', '/api/v1/provider-profiles'),
          api('GET', '/api/v1/provider-bindings'),
          fetchRemoteHosts(),
        ]);
        const profiles = profilesPayload && Array.isArray(profilesPayload.profiles) ? profilesPayload.profiles : [];
        const bindings = bindingsPayload && Array.isArray(bindingsPayload.bindings) ? bindingsPayload.bindings : [];
        providerProfilesCache = profiles;
        remoteHostsCache = hosts;
        pruneServerHostOperationCache(hosts);
        syncProfileTestHostOptions(hosts);
        renderProfilesAndBindings(profiles, bindings);
        syncProfileEditSelection(profiles);

        profileSelect.textContent = '';
        profiles.forEach(profile => {
          const opt = document.createElement('option');
          opt.value = profile.id;
          opt.textContent = profile.name || profile.id;
          profileSelect.appendChild(opt);
        });
      } catch (e) {
        setMsg('#profiles-msg', 'Load failed: ' + e.message, 'error');
        renderProfilesAndBindings([], []);
      } finally {
        syncBindingControls();
      }
    }

    refreshBtn.onclick = refreshAll;

    saveProfileBtn.onclick = async () => {
      const payload = {
        name: ($('#profile-name').value || '').trim(),
        provider: ($('#profile-provider').value || '').trim(),
        model: ($('#profile-model').value || '').trim(),
        baseUrl: ($('#profile-base-url').value || '').trim(),
        authRef: ($('#profile-auth-ref').value || '').trim(),
        enabled: ($('#profile-enabled').value || 'true') === 'true',
      };
      const editingID = String(profileEditingProfileID || '').trim();
      try {
        saveProfileBtn.disabled = true;
        if (editingID) {
          await api('PATCH', '/api/v1/provider-profiles/' + encodeURIComponent(editingID), payload);
          setMsg('#profiles-msg', 'Profile updated: ' + editingID, 'success');
        } else {
          await api('POST', '/api/v1/provider-profiles', payload);
          setMsg('#profiles-msg', 'Profile saved.', 'success');
        }
        resetProfileEditor(true);
        await refreshAll();
      } catch (e) {
        setMsg('#profiles-msg', 'Save profile failed: ' + e.message, 'error');
      } finally {
        saveProfileBtn.disabled = false;
      }
    };

    saveBindingBtn.onclick = async () => {
      if (!featureFlags.providerBindingEnabled) {
        setMsg('#profiles-msg', 'Provider binding is disabled by feature flag.', 'error');
        return;
      }
      const profileID = ($('#binding-profile-id').value || '').trim();
      const targetType = ($('#binding-target-type').value || '').trim();
      const targetID = ($('#binding-target-id').value || '').trim();
      if (!profileID || !targetType || !targetID) {
        setMsg('#profiles-msg', 'profile, target type and target id are required.', 'error');
        return;
      }
      try {
        saveBindingBtn.disabled = true;
        await api('POST', '/api/v1/provider-bindings', {
          id: profileID + ':' + targetType + ':' + targetID,
          profileId: profileID,
          targetType: targetType,
          targetId: targetID,
          syncMode: 'always_push',
        });
        setMsg('#profiles-msg', 'Binding saved and synced.', 'success');
        await refreshAll();
      } catch (e) {
        setMsg('#profiles-msg', 'Save binding failed: ' + e.message, 'error');
      } finally {
        saveBindingBtn.disabled = false;
      }
    };

    if (cancelEditBtn) {
      cancelEditBtn.onclick = () => {
        resetProfileEditor(true);
        setMsg('#profiles-msg', 'Profile edit cancelled.', 'info');
      };
    }
    if (profileTestHostSelect && !profileTestHostSelect.options.length) {
      const opt = document.createElement('option');
      opt.value = '';
      opt.textContent = 'auto (first host)';
      profileTestHostSelect.appendChild(opt);
    }
    updateProfileEditorUI();
    syncBindingControls();
    refreshAll();
  }

  function renderRemoteChatMessages() {
    const wrap = $('#remote-chat-messages');
    if (!wrap) return;
    const island = window.CarrierRemoteChatIsland;
    if (island && typeof island.renderMessages === 'function' && island.renderMessages(wrap, remoteChatMessages)) {
      return;
    }
    wrap.textContent = '';
    remoteChatMessages.forEach(message => {
      const msg = document.createElement('div');
      msg.className = 'chat-msg';
      const sender = document.createElement('span');
      sender.className = 'sender';
      sender.textContent = (message.role === 'user' ? 'You' : message.role === 'assistant' ? 'Agent' : 'Carrier') + ':';
      const body = document.createElement('span');
      body.className = 'body';
      body.textContent = ' ' + String(message.text || '');
      msg.appendChild(sender);
      msg.appendChild(body);
      wrap.appendChild(msg);
    });
    wrap.scrollTop = wrap.scrollHeight;
  }

  function appendRemoteChatMessage(role, text) {
    remoteChatMessageSeq += 1;
    const messageID = 'msg-' + String(remoteChatMessageSeq);
    remoteChatMessages.push({
      id: messageID,
      role: String(role || 'system'),
      text: String(text || ''),
    });
    renderRemoteChatMessages();
    return messageID;
  }

  function appendRemoteChatMessageDelta(messageID, delta) {
    const id = String(messageID || '');
    if (!id) return;
    const index = remoteChatMessages.findIndex(message => String(message.id || '') === id);
    if (index < 0) return;
    remoteChatMessages[index].text = String(remoteChatMessages[index].text || '') + String(delta || '');
    renderRemoteChatMessages();
  }

  function updateRemoteChatStatus(text, type) {
    const el = $('#remote-chat-status');
    if (!el) return;
    el.textContent = text || '';
    el.classList.remove('msg-error', 'msg-success', 'msg-info');
    if (type) el.classList.add('msg-' + type);
  }

  function parseSSEFrames(buffer, onEvent) {
    let remaining = buffer;
    for (;;) {
      const idx = remaining.indexOf('\n\n');
      if (idx < 0) break;
      const frame = remaining.slice(0, idx);
      remaining = remaining.slice(idx + 2);
      const lines = frame.split('\n');
      const dataLines = lines.filter(line => line.startsWith('data:')).map(line => line.slice(5).trim());
      if (!dataLines.length) continue;
      const dataText = dataLines.join('\n');
      try {
        const payload = JSON.parse(dataText);
        onEvent(payload);
      } catch (_) {
        // ignore invalid frame payload
      }
    }
    return remaining;
  }

  async function loadRemoteChatTargets() {
    const hostSelect = $('#remote-chat-host');
    const profileSelect = $('#remote-chat-profile');
    hostSelect.textContent = '';
    profileSelect.textContent = '';

    const [hosts, profiles] = await Promise.all([fetchRemoteHosts(), fetchProviderProfiles()]);
    hosts.forEach(host => {
      const opt = document.createElement('option');
      opt.value = host.id;
      opt.textContent = host.name || host.id;
      hostSelect.appendChild(opt);
    });
    profiles.forEach(profile => {
      const opt = document.createElement('option');
      opt.value = profile.id;
      opt.textContent = profile.name || profile.id;
      profileSelect.appendChild(opt);
    });
    if (!profileSelect.options.length) {
      const opt = document.createElement('option');
      opt.value = '';
      opt.textContent = 'none';
      profileSelect.appendChild(opt);
    } else {
      const none = document.createElement('option');
      none.value = '';
      none.textContent = 'none';
      profileSelect.insertBefore(none, profileSelect.firstChild);
      profileSelect.value = '';
    }
  }

  async function loadRemoteChatInstances(hostID) {
    const instanceSelect = $('#remote-chat-instance');
    instanceSelect.textContent = '';
    if (!hostID) return;
    const payload = await api('GET', '/api/v1/remote/hosts/' + encodeURIComponent(hostID) + '/instances');
    const instances = payload && Array.isArray(payload.instances) ? payload.instances : [];
    instances.forEach(instance => {
      const opt = document.createElement('option');
      const agentID = instance.agentId || instance.agentID || instance.id || 'main';
      opt.value = agentID;
      opt.textContent = agentID + ' (' + (instance.runtimeState || 'unknown') + ')';
      instanceSelect.appendChild(opt);
    });
  }

  async function loadLocalChatInstances() {
    const instanceSelect = $('#remote-chat-instance');
    instanceSelect.textContent = '';

    const baseOption = document.createElement('option');
    baseOption.value = '';
    baseOption.textContent = 'base-agent (fallback)';
    instanceSelect.appendChild(baseOption);

    const payload = await api('GET', '/api/v1/instances');
    const instances = payload && Array.isArray(payload.instances) ? payload.instances : [];
    instances.forEach(instance => {
      const runtimeAgentID = (instance.agent_id || instance.agentID || instance.type || '').trim();
      if (!runtimeAgentID) return;
      const instanceID = (instance.id || '').trim();
      const runtimeState = (instance.runtime_state || instance.runtimeState || 'unknown').trim();
      const opt = document.createElement('option');
      opt.value = runtimeAgentID;
      opt.textContent = (instanceID || runtimeAgentID) + ' (' + runtimeAgentID + ', ' + runtimeState + ')';
      instanceSelect.appendChild(opt);
    });
  }

  async function sendRemoteChat(inputText) {
    const targetType = ($('#remote-chat-target').value || 'remote').trim();
    const hostID = ($('#remote-chat-host').value || '').trim();
    const agentID = ($('#remote-chat-instance').value || '').trim();
    const profileID = ($('#remote-chat-profile').value || '').trim();
    const message = (inputText || '').trim();
    if (!message) {
      updateRemoteChatStatus('message is required.', 'error');
      return;
    }
    if (targetType === 'remote' && !featureFlags.remoteControlPlaneEnabled) {
      updateRemoteChatStatus('Remote control plane is disabled by feature flag.', 'error');
      return;
    }
    if (targetType === 'remote' && !featureFlags.remoteChatEnabled) {
      updateRemoteChatStatus('Remote chat is disabled by feature flag.', 'error');
      return;
    }
    if (targetType === 'remote' && (!hostID || !agentID)) {
      updateRemoteChatStatus('host, instance and message are required for remote target.', 'error');
      return;
    }

    remoteChatLastInput = message;
    appendRemoteChatMessage('user', message);
    remoteChatActiveAssistantNode = appendRemoteChatMessage('assistant', '');

    if (targetType === 'remote' && profileID) {
      if (!featureFlags.providerBindingEnabled) {
        updateRemoteChatStatus('Provider binding is disabled by feature flag.', 'error');
        return;
      }
      try {
        await api('POST', '/api/v1/provider-bindings', {
          id: profileID + ':instance:' + hostID + ':' + agentID,
          profileId: profileID,
          targetType: 'instance',
          targetId: hostID + ':' + agentID,
          syncMode: 'always_push',
        });
      } catch (e) {
        updateRemoteChatStatus('Profile apply failed: ' + e.message, 'error');
        return;
      }
    }

    if (remoteChatAbortController) {
      remoteChatAbortController.abort();
      remoteChatAbortController = null;
    }
    const controller = new AbortController();
    remoteChatAbortController = controller;
    updateRemoteChatStatus('Streaming response…', 'info');

    try {
      const headers = { 'Content-Type': 'application/json' };
      if (token) headers['Authorization'] = 'Bearer ' + token;
      const response = await fetch('/api/v1/chat/stream', {
        method: 'POST',
        headers,
        body: JSON.stringify({
          target: targetType,
          hostId: targetType === 'remote' ? hostID : '',
          agentId: targetType === 'remote' ? agentID : agentID,
          message: message,
          sessionId: remoteChatSessionID || '',
          provider: targetType === 'local' ? 'webui' : '',
          chatId: targetType === 'local' ? (remoteChatSessionID || '') : '',
        }),
        signal: controller.signal,
      });
      if (!response.ok) {
        const text = await response.text();
        throw new Error(text || 'chat failed (' + response.status + ')');
      }
      if (!response.body || !response.body.getReader) {
        throw new Error('streaming body is not supported in this browser');
      }
      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      let buffer = '';
      for (;;) {
        const step = await reader.read();
        if (step.done) break;
        buffer += decoder.decode(step.value, { stream: true });
        buffer = parseSSEFrames(buffer, payload => {
          const eventType = String(payload && payload.type ? payload.type : '').trim();
          if (eventType === 'text-delta') {
            const delta = String(payload.delta || '');
            if (remoteChatActiveAssistantNode) {
              appendRemoteChatMessageDelta(remoteChatActiveAssistantNode, delta);
            }
            return;
          }
          if (eventType === 'session') {
            remoteChatSessionID = String(payload.sessionId || '').trim();
            updateRemoteChatStatus('Session: ' + remoteChatSessionID, 'info');
            return;
          }
          if (eventType === 'finish') {
            updateRemoteChatStatus('Stream finished.', 'success');
          }
        });
      }
      buffer = parseSSEFrames(buffer, () => {});
      updateRemoteChatStatus('Stream finished.', 'success');
    } catch (e) {
      if (e.name === 'AbortError') {
        updateRemoteChatStatus('Stream cancelled.', 'info');
      } else {
        updateRemoteChatStatus('Stream failed: ' + e.message, 'error');
      }
    } finally {
      if (remoteChatAbortController === controller) {
        remoteChatAbortController = null;
      }
      remoteChatActiveAssistantNode = null;
    }
  }

  async function initRemoteChat() {
    showView('remote-chat');
    $('#nav').classList.remove('hidden');

    const targetSelect = $('#remote-chat-target');
    const hostSelect = $('#remote-chat-host');
    const input = $('#remote-chat-input');
    const sendBtn = $('#remote-chat-send');
    const refreshBtn = $('#remote-chat-refresh');
    const resetBtn = $('#remote-chat-reset-session');
    const cancelBtn = $('#remote-chat-cancel');
    const retryBtn = $('#remote-chat-retry');
    renderRemoteChatMessages();

    async function refreshTargets() {
      const targetType = ($('#remote-chat-target').value || 'remote').trim();
      if (targetType === 'local') {
        try {
          await loadLocalChatInstances();
          updateRemoteChatStatus('Local target selected. Choose a local instance or use base-agent fallback.', 'info');
        } catch (e) {
          updateRemoteChatStatus('Load local instances failed: ' + e.message, 'error');
        }
        return;
      }
      try {
        await loadRemoteChatTargets();
        await loadRemoteChatInstances((hostSelect.value || '').trim());
        updateRemoteChatStatus('Targets loaded.', 'info');
      } catch (e) {
        updateRemoteChatStatus('Load targets failed: ' + e.message, 'error');
      }
    }

    refreshBtn.onclick = refreshTargets;
    hostSelect.onchange = async () => {
      try {
        await loadRemoteChatInstances((hostSelect.value || '').trim());
      } catch (e) {
        updateRemoteChatStatus('Load instances failed: ' + e.message, 'error');
      }
    };
    sendBtn.onclick = () => {
      const text = (input.value || '').trim();
      if (!text) return;
      input.value = '';
      sendRemoteChat(text);
    };
    input.onkeydown = e => {
      if (e.key === 'Enter') {
        const text = (input.value || '').trim();
        if (!text) return;
        input.value = '';
        sendRemoteChat(text);
      }
    };
    resetBtn.onclick = () => {
      remoteChatSessionID = '';
      updateRemoteChatStatus('Session reset. Next message starts a new session.', 'info');
    };
    cancelBtn.onclick = () => {
      if (remoteChatAbortController) {
        remoteChatAbortController.abort();
      } else {
        updateRemoteChatStatus('No active stream.', 'info');
      }
    };
    retryBtn.onclick = () => {
      if (!remoteChatLastInput) {
        updateRemoteChatStatus('No previous message to retry.', 'info');
        return;
      }
      sendRemoteChat(remoteChatLastInput);
    };
    targetSelect.onchange = () => {
      const remoteMode = targetSelect.value === 'remote';
      hostSelect.disabled = !remoteMode;
      $('#remote-chat-instance').disabled = false;
      $('#remote-chat-profile').disabled = !remoteMode;
      remoteChatSessionID = '';
      updateRemoteChatStatus(remoteMode ? 'Remote target selected.' : 'Local target selected.', 'info');
      if (remoteMode) {
        refreshTargets();
        return;
      }
      refreshTargets();
    };

    targetSelect.onchange();
    refreshTargets();
  }

  function toFiniteNumber(value, fallback) {
    const num = Number(value);
    if (!Number.isFinite(num)) return fallback;
    return num;
  }

  function formatPercent(value) {
    return Math.round(toFiniteNumber(value, 0) * 100) + '%';
  }

  function formatMilliseconds(value) {
    return Math.round(toFiniteNumber(value, 0)) + 'ms';
  }

  function normalizeRemoteMetricsPayload(payload) {
    const metrics = payload && payload.metrics && typeof payload.metrics === 'object' ? payload.metrics : {};
    const totals = metrics.totals && typeof metrics.totals === 'object' ? metrics.totals : {};
    const repair = metrics.repair && typeof metrics.repair === 'object' ? metrics.repair : {};
    const chat = metrics.chatStream && typeof metrics.chatStream === 'object' ? metrics.chatStream : {};
    const operations = metrics.operations && typeof metrics.operations === 'object' ? metrics.operations : {};
    const rolloutSource = metrics.rollout && typeof metrics.rollout === 'object' ? metrics.rollout : {};
    const rolloutReasons = Array.isArray(rolloutSource.reasons)
      ? rolloutSource.reasons.map(item => String(item || '')).filter(Boolean)
      : [];
    const rollout = {
      state: String(rolloutSource.state || 'unknown'),
      canPromote: toFeatureBool(rolloutSource.canPromote, false),
      reasons: rolloutReasons,
    };
    return {
      timestamp: String(metrics.timestamp || ''),
      totals: totals,
      repair: repair,
      chatStream: chat,
      operations: operations,
      rollout: rollout,
    };
  }

  function classifyRemoteOperationGroup(operationName) {
    const name = String(operationName || '').trim().toLowerCase();
    if (name.startsWith('host_')) return 'host';
    if (name.startsWith('instances_')) return 'instances';
    if (name.startsWith('config_')) return 'config';
    if (name.startsWith('session_') || name.startsWith('sessions_')) return 'sessions';
    if (name.startsWith('memory_')) return 'memory';
    if (name.startsWith('provider_')) return 'provider';
    if (name.startsWith('remote_chat_') || name.startsWith('chat_')) return 'chat';
    return 'other';
  }

  function isRemoteOperationAnomaly(stats) {
    const row = stats || {};
    const failure = toFiniteNumber(row.failure, 0);
    const successRate = toFiniteNumber(row.successRate, 0);
    const avgLatencyMs = toFiniteNumber(row.avgLatencyMs, 0);
    return failure > 0 || successRate < 1 || avgLatencyMs >= 1000;
  }

  function renderRemoteObservabilityGroupOptions(normalized) {
    const select = $('#remote-observability-group');
    if (!select) return;
    const current = (select.value || 'all').trim().toLowerCase();
    const operations = normalized.operations || {};
    const groups = new Set(['all']);
    Object.keys(operations).forEach(name => {
      groups.add(classifyRemoteOperationGroup(name));
    });
    const ordered = Array.from(groups).sort((a, b) => {
      if (a === 'all') return -1;
      if (b === 'all') return 1;
      return a.localeCompare(b);
    });
    select.textContent = '';
    ordered.forEach(group => {
      const option = document.createElement('option');
      option.value = group;
      option.textContent = group;
      select.appendChild(option);
    });
    select.value = ordered.includes(current) ? current : 'all';
  }

  function getFilteredRemoteOperationEntries(normalized) {
    const operations = normalized.operations || {};
    const entries = Object.entries(operations);
    const groupSelect = $('#remote-observability-group');
    const anomalyToggle = $('#remote-observability-anomalies');
    const selectedGroup = (groupSelect && groupSelect.value ? groupSelect.value : 'all').trim().toLowerCase();
    const anomaliesOnly = !!(anomalyToggle && anomalyToggle.checked);
    const filtered = entries.filter(([name, stats]) => {
      const group = classifyRemoteOperationGroup(name);
      if (selectedGroup !== 'all' && group !== selectedGroup) return false;
      if (anomaliesOnly && !isRemoteOperationAnomaly(stats)) return false;
      return true;
    });
    return {
      entries: filtered,
      visibleCount: filtered.length,
      totalCount: entries.length,
      selectedGroup: selectedGroup,
      anomaliesOnly: anomaliesOnly,
    };
  }

  function renderRemoteObservabilitySummary(normalized) {
    const wrap = $('#remote-observability-summary');
    if (!wrap) return;

    const totals = normalized.totals || {};
    const repair = normalized.repair || {};
    const chat = normalized.chatStream || {};
    const rollout = normalized.rollout || {};
    const rolloutState = String(rollout.state || 'unknown').trim().toLowerCase() || 'unknown';
    const rolloutReasons = Array.isArray(rollout.reasons) ? rollout.reasons : [];
    const cards = [
      {
        title: 'Operations',
        lines: [
          'total: ' + toFiniteNumber(totals.total, 0),
          'success: ' + toFiniteNumber(totals.success, 0),
          'failure: ' + toFiniteNumber(totals.failure, 0),
          'success rate: ' + formatPercent(totals.successRate),
        ],
      },
      {
        title: 'Latency',
        lines: [
          'avg: ' + formatMilliseconds(totals.avgLatencyMs),
          'min: ' + formatMilliseconds(totals.minLatencyMs),
          'max: ' + formatMilliseconds(totals.maxLatencyMs),
        ],
      },
      {
        title: 'Repair',
        lines: [
          'triggered: ' + toFiniteNumber(repair.triggered, 0),
          'success: ' + toFiniteNumber(repair.success, 0),
          'failure: ' + toFiniteNumber(repair.failure, 0),
          'success rate: ' + formatPercent(repair.successRate),
        ],
      },
      {
        title: 'Chat Stream',
        lines: [
          'total: ' + toFiniteNumber(chat.total, 0),
          'failure: ' + toFiniteNumber(chat.failure, 0),
          'failure rate: ' + formatPercent(chat.failureRate),
        ],
      },
      {
        title: 'Rollout',
        lines: [
          'state: ' + rolloutState,
          'can promote: ' + (toFeatureBool(rollout.canPromote, false) ? 'yes' : 'no'),
          'reasons: ' + (rolloutReasons.length ? rolloutReasons.join('; ') : 'none'),
        ],
      },
    ];
    const island = window.CarrierRemoteObservabilityIsland;
    if (island && typeof island.renderSummary === 'function' && island.renderSummary(wrap, cards)) {
      return;
    }

    wrap.textContent = '';

    function appendCard(title, lines) {
      const card = document.createElement('div');
      card.className = 'agent-card';
      const h4 = document.createElement('h4');
      h4.textContent = title;
      card.appendChild(h4);
      lines.forEach(line => {
        const row = document.createElement('div');
        row.className = 'instance-meta';
        row.textContent = line;
        card.appendChild(row);
      });
      wrap.appendChild(card);
    }

    cards.forEach(card => appendCard(card.title, card.lines));
  }

  function renderRemoteObservabilityOperations(normalized) {
    const body = $('#remote-observability-ops-body');
    if (!body) return;
    renderRemoteObservabilityGroupOptions(normalized);
    const filtered = getFilteredRemoteOperationEntries(normalized);
    const entries = filtered.entries;
    const island = window.CarrierRemoteObservabilityIsland;
    if (!entries.length) {
      const emptyMessage = filtered.totalCount > 0
        ? 'No remote operation metrics match current filters.'
        : 'No remote operation metrics yet.';
      if (island && typeof island.renderOperations === 'function') {
        island.renderOperations(body, { rows: [], emptyMessage: emptyMessage });
      } else {
        body.textContent = '';
        const tr = document.createElement('tr');
        const td = document.createElement('td');
        td.colSpan = 6;
        td.className = 'text-dim';
        td.textContent = emptyMessage;
        tr.appendChild(td);
        body.appendChild(tr);
      }
      return filtered;
    }
    const sortedEntries = entries.slice().sort((a, b) => {
      const af = toFiniteNumber(a[1] && a[1].failure, 0);
      const bf = toFiniteNumber(b[1] && b[1].failure, 0);
      if (bf !== af) return bf - af;
      const as = toFiniteNumber(a[1] && a[1].successRate, 1);
      const bs = toFiniteNumber(b[1] && b[1].successRate, 1);
      if (as !== bs) return as - bs;
      const at = toFiniteNumber(a[1] && a[1].total, 0);
      const bt = toFiniteNumber(b[1] && b[1].total, 0);
      if (bt !== at) return bt - at;
      const al = toFiniteNumber(a[1] && a[1].avgLatencyMs, 0);
      const bl = toFiniteNumber(b[1] && b[1].avgLatencyMs, 0);
      if (bl !== al) return bl - al;
      return String(a[0]).localeCompare(String(b[0]));
    });
    const rows = sortedEntries.map(([name, stats]) => {
      const row = stats || {};
      return {
        name: String(name),
        total: String(toFiniteNumber(row.total, 0)),
        success: String(toFiniteNumber(row.success, 0)),
        failure: String(toFiniteNumber(row.failure, 0)),
        successRate: formatPercent(row.successRate),
        avgLatency: formatMilliseconds(row.avgLatencyMs),
      };
    });
    if (island && typeof island.renderOperations === 'function') {
      island.renderOperations(body, { rows: rows });
      return filtered;
    }
    body.textContent = '';
    rows.forEach(row => {
      const tr = document.createElement('tr');
      const cells = [
        row.name,
        row.total,
        row.success,
        row.failure,
        row.successRate,
        row.avgLatency,
      ];
      cells.forEach(value => {
        const td = document.createElement('td');
        td.textContent = value;
        tr.appendChild(td);
      });
      body.appendChild(tr);
    });
    return filtered;
  }

  async function initRemoteObservability() {
    showView('remote-observability');
    $('#nav').classList.remove('hidden');

    const status = $('#remote-observability-status');
    const refreshBtn = $('#remote-observability-refresh');
    const groupSelect = $('#remote-observability-group');
    const anomalyToggle = $('#remote-observability-anomalies');

    function renderFromCache() {
      if (!remoteObservabilityData) return;
      renderRemoteObservabilitySummary(remoteObservabilityData);
      const filtered = renderRemoteObservabilityOperations(remoteObservabilityData) || {
        visibleCount: 0,
        totalCount: 0,
        selectedGroup: 'all',
        anomaliesOnly: false,
      };
      const ts = remoteObservabilityData.timestamp || new Date().toISOString();
      const parts = [
        'Updated at ' + ts,
        'showing ' + filtered.visibleCount + '/' + filtered.totalCount + ' operations',
      ];
      if (filtered.selectedGroup !== 'all') {
        parts.push('group=' + filtered.selectedGroup);
      }
      if (filtered.anomaliesOnly) {
        parts.push('anomalies only');
      }
      const rollout = remoteObservabilityData.rollout || {};
      const rolloutState = String(rollout.state || '').trim().toLowerCase();
      if (rolloutState) {
        parts.push('rollout=' + rolloutState);
      }
      status.textContent = parts.join(' · ');
    }

    async function refreshMetrics() {
      status.textContent = 'Loading remote metrics...';
      try {
        const payload = await api('GET', '/api/v1/remote/metrics');
        remoteObservabilityData = normalizeRemoteMetricsPayload(payload);
        renderFromCache();
      } catch (e) {
        remoteObservabilityData = normalizeRemoteMetricsPayload({});
        renderFromCache();
        status.textContent = 'Load remote metrics failed: ' + e.message;
      }
    }

    refreshBtn.onclick = refreshMetrics;
    groupSelect.onchange = renderFromCache;
    anomalyToggle.onchange = renderFromCache;
    refreshMetrics();
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
    if (featureFlags.remoteControlPlaneEnabled) {
      try {
        const remote = normalizeRemoteMetricsPayload(await api('GET', '/api/v1/remote/metrics'));
        const totals = remote.totals;
        const repair = remote.repair;
        const chat = remote.chatStream;
        const rollout = remote.rollout || {};
        const rolloutState = String(rollout.state || 'unknown');
        const rolloutReasons = Array.isArray(rollout.reasons) ? rollout.reasons : [];
        const totalOps = toFiniteNumber(totals.total, 0);
        lines.push(
          'Remote metrics: ops=' + totalOps +
          ', op success rate=' + formatPercent(totals.successRate) +
          ', repair triggered=' + toFiniteNumber(repair.triggered, 0) +
          ', repair success=' + formatPercent(repair.successRate) +
          ', chat stream failure=' + formatPercent(chat.failureRate) + '.',
        );
        lines.push(
          'Remote rollout gate: state=' + rolloutState +
          ', can promote=' + (toFeatureBool(rollout.canPromote, false) ? 'yes' : 'no') +
          (rolloutReasons.length ? ', reasons=' + rolloutReasons.join('; ') : '') +
          '.',
        );
      } catch (_) {
        lines.push('Remote metrics unavailable.');
      }
    } else {
      lines.push('Remote control plane disabled by feature flag.');
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
            return refreshFeatureFlags().then(() => {
              navigate(location.hash || '#/dashboard');
            });
          } else {
            featureFlags = { ...DEFAULT_FEATURE_FLAGS };
            applyFeatureFlags();
            showLogin();
          }
        })
        .catch(() => {
          featureFlags = { ...DEFAULT_FEATURE_FLAGS };
          applyFeatureFlags();
          showLogin();
        });
    } else {
      // No token — check if auth is required
      fetch('/healthz')
        .then(r => {
          if (r.ok) {
            // No auth required
            hideLogin();
            return refreshFeatureFlags().then(() => {
              navigate(location.hash || '#/welcome');
            });
          } else {
            featureFlags = { ...DEFAULT_FEATURE_FLAGS };
            applyFeatureFlags();
            showLogin();
          }
        })
        .catch(() => {
          featureFlags = { ...DEFAULT_FEATURE_FLAGS };
          applyFeatureFlags();
          showLogin();
        });
    }
  }

  init();
})();
