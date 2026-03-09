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
  let delegateEventSource = null; // EventSource for delegate completion notifications
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
  let sshConfigHostAliasesCache = [];
  let providerProfilesCache = [];
  let executionTemplatesCache = [];
  let executionTriggersCache = [];
  let orchestratorPolicyRulesCache = [];
  let serverManageHostID = '';
  let serverManageOperationRunning = false;
  let serverManageLastOperation = null;
  let serverHostLastOperationByID = {};
  let serverEditingHostID = '';
  let profileEditingProfileID = '';
  let triggerEditingTriggerID = '';
  let serverManageInstallStreamAbortController = null;
  let serverManageLiveLogLines = [];
  let serverManageDiagnosisPending = false;
  let serverManageDiagnosisText = '';
  let serverManageChatSessionID = '';
  let serverManageChatAbortController = null;
  let serverManageChatLastInput = '';
  let serverManageChatActiveAssistantNode = null;
  let serverManageChatMessages = [];
  let serverManageChatMessageSeq = 0;
  let remoteChatSessionID = '';
  let remoteChatAbortController = null;
  let remoteChatLastInput = '';
  let remoteChatActiveAssistantNode = null;
  let remoteChatMessages = [];
  let remoteChatMessageSeq = 0;
  let remoteChatTargetsLoadSeq = 0;
  let remoteChatInstancesLoadSeq = 0;
  let remoteObservabilityData = null;
  let orchestratorObservabilityData = null;
  let dashboardExecutionPollTimer = null;
  let dashboardExecutionDetailsByID = {};
  let dashboardExpandedExecutionIDs = new Set();
  let executionRecordsCache = [];
  let selectedExecutionID = '';
  let workerInventoryCache = [];
  let workerQueueSummaryCache = null;
  let memoryListCache = null;
  let memorySearchResultsCache = [];
  let quickLaunchPlan = null;
  let quickLaunchProviderCatalog = [];
  let quickLaunchTemplates = [];
  let workersPollTimer = null;
  const DEFAULT_FEATURE_FLAGS = {
    remoteControlPlaneEnabled: false,
    remoteChatEnabled: false,
    providerBindingEnabled: false,
  };
  const DEFAULT_AUTHZ = {
    role: 'viewer',
    permissions: {
      viewExecutions: false,
      launchExecutions: false,
      approveExecutions: false,
      managePolicies: false,
      manageProviders: false,
      manageHosts: false,
    },
  };
  let featureFlags = { ...DEFAULT_FEATURE_FLAGS };
  let authzState = JSON.parse(JSON.stringify(DEFAULT_AUTHZ));

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
        const err = new Error(errMsg);
        err.status = r.status;
        err.payload = data;
        throw err;
      }
      return data;
    });
  }

  async function downloadAPI(path, filename) {
    const opts = { headers: {} as Record<string, string> };
    if (token) opts.headers['Authorization'] = 'Bearer ' + token;
    const response = await fetch(path, opts);
    if (response.status === 401) {
      clearToken();
      throw new Error('Unauthorized');
    }
    if (!response.ok) {
      const raw = await response.text();
      throw new Error(raw || 'Request failed (' + response.status + ')');
    }
    const blob = await response.blob();
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    if (filename) link.download = filename;
    document.body.appendChild(link);
    link.click();
    link.remove();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  }

  function clearToken() {
    disconnectDelegateEvents();
    stopDashboardExecutionPolling();
    token = '';
    localStorage.removeItem('carrier_token');
    showLogin();
  }

  function disconnectDelegateEvents() {
    if (delegateEventSource) {
      try { delegateEventSource.close(); } catch (_) {}
      delegateEventSource = null;
    }
  }

  function showDelegateNotification(message, status) {
    const text = String(message || '').trim();
    if (!text) return;
    let root = document.getElementById('delegate-toast-root');
    if (!root) {
      root = document.createElement('div');
      root.id = 'delegate-toast-root';
      root.className = 'delegate-toast-root';
      document.body.appendChild(root);
    }
    const item = document.createElement('div');
    item.textContent = text;
    item.className = 'delegate-toast-item';
    item.classList.add(String(status || '').toLowerCase() === 'completed' ? 'success' : 'error');
    root.appendChild(item);
    setTimeout(() => {
      item.remove();
      if (root && root.childElementCount === 0) {
        root.remove();
      }
    }, 7000);
  }

  function connectDelegateEvents() {
    disconnectDelegateEvents();
    let sseUrl = '/api/v1/webui/delegate/events';
    if (token) sseUrl += '?token=' + encodeURIComponent(token);
    try {
      const es = new EventSource(sseUrl);
      delegateEventSource = es;
      es.onmessage = e => {
        if (!e || !e.data) return;
        let payload = null;
        try {
          payload = JSON.parse(e.data);
        } catch (_) {
          return;
        }
        if (!payload || payload.type !== 'delegate-finish') return;
        const executionId = String(payload.executionId || '').trim();
        const status = String(payload.status || '').trim();
        const error = String(payload.error || '').trim();
        let msg = 'Execution updated: ' + executionId;
        if (status === 'completed') {
          msg = 'Execution completed: ' + executionId;
        } else if (status === 'cancelled') {
          msg = 'Execution cancelled: ' + executionId + (error ? ' (' + error + ')' : '');
        } else if (status) {
          msg = 'Execution ' + status + ': ' + executionId + (error ? ' (' + error + ')' : '');
        }
        showDelegateNotification(msg, status);
        const routeName = currentRouteName();
        if (routeName === 'dashboard' || routeName === 'executions') {
          refreshExecutions();
        }
        if (routeName === 'workers') {
          refreshWorkers();
        }
      };
      es.onerror = () => {
        if (delegateEventSource !== es) return;
        disconnectDelegateEvents();
        setTimeout(() => {
          connectDelegateEvents();
        }, 3000);
      };
    } catch (_) {
      // ignore
    }
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

  function normalizeAuthz(payload) {
    const source = payload && typeof payload === 'object' && payload.authz && typeof payload.authz === 'object'
      ? payload.authz
      : {};
    const permissions = source.permissions && typeof source.permissions === 'object' ? source.permissions : {};
    return {
      role: String(source.role || DEFAULT_AUTHZ.role).trim().toLowerCase() || DEFAULT_AUTHZ.role,
      permissions: {
        viewExecutions: toFeatureBool(permissions.viewExecutions, DEFAULT_AUTHZ.permissions.viewExecutions),
        launchExecutions: toFeatureBool(permissions.launchExecutions, DEFAULT_AUTHZ.permissions.launchExecutions),
        approveExecutions: toFeatureBool(permissions.approveExecutions, DEFAULT_AUTHZ.permissions.approveExecutions),
        managePolicies: toFeatureBool(permissions.managePolicies, DEFAULT_AUTHZ.permissions.managePolicies),
        manageProviders: toFeatureBool(permissions.manageProviders, DEFAULT_AUTHZ.permissions.manageProviders),
        manageHosts: toFeatureBool(permissions.manageHosts, DEFAULT_AUTHZ.permissions.manageHosts),
      },
    };
  }

  function canViewExecutionsUI() {
    return !!(authzState && authzState.permissions && authzState.permissions.viewExecutions);
  }

  function canLaunchExecutionsUI() {
    return !!(authzState && authzState.permissions && authzState.permissions.launchExecutions);
  }

  function canApproveExecutionsUI() {
    return !!(authzState && authzState.permissions && authzState.permissions.approveExecutions);
  }

  function canManagePoliciesUI() {
    return !!(authzState && authzState.permissions && authzState.permissions.managePolicies);
  }

  function canManageProvidersUI() {
    return !!(authzState && authzState.permissions && authzState.permissions.manageProviders);
  }

  function canManageHostsUI() {
    return !!(authzState && authzState.permissions && authzState.permissions.manageHosts);
  }

  function setNavRouteVisible(route, visible) {
    const link = $('.nav-link[data-route="' + route + '"]');
    if (!link) return;
    link.classList.toggle('hidden', !visible);
  }

  function parseRoute(hash) {
    let normalized = String(hash || '#/welcome').trim();
    if (!normalized || normalized === '#' || normalized === '#/') normalized = '#/welcome';
    let path = normalized.replace(/^#\/?/, '').replace(/^\/+/, '');
    let query = {};
    const queryIndex = path.indexOf('?');
    if (queryIndex >= 0) {
      const params = new URLSearchParams(path.slice(queryIndex + 1));
      path = path.slice(0, queryIndex);
      params.forEach((value, key) => {
        query[key] = value;
      });
    }
    path = path.replace(/\/+$/, '');
    if (!path) path = 'welcome';
    const segments = path.split('/').filter(Boolean);
    return {
      path,
      name: segments[0] || 'welcome',
      segments,
      query,
    };
  }

  function currentRouteName() {
    return parseRoute(location.hash).name;
  }

  function formatDateTime(raw) {
    const text = String(raw || '').trim();
    if (!text) return 'n/a';
    const parsed = new Date(text);
    if (Number.isNaN(parsed.getTime())) return text;
    return parsed.toLocaleString();
  }

  function formatAgeSeconds(value) {
    const seconds = Number(value || 0);
    if (!Number.isFinite(seconds) || seconds <= 0) return 'n/a';
    if (seconds < 60) return String(Math.round(seconds)) + 's';
    if (seconds < 3600) return String(Math.round(seconds / 60)) + 'm';
    return String((seconds / 3600).toFixed(seconds % 3600 === 0 ? 0 : 1)) + 'h';
  }

  function isRouteEnabled(route) {
    if (route === 'executions' || route === 'workers' || route === 'servers' || route === 'profiles' || route === 'remote-observability') {
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
    setNavRouteVisible('executions', remoteControlVisible);
    setNavRouteVisible('workers', remoteControlVisible);
    setNavRouteVisible('servers', remoteControlVisible);
    setNavRouteVisible('profiles', remoteControlVisible);
    setNavRouteVisible('remote-chat', remoteChatVisible);
    setNavRouteVisible('remote-observability', remoteControlVisible);
    const executionSection = $('#dashboard-executions-section');
    if (executionSection) executionSection.classList.toggle('hidden', !(remoteControlVisible && canViewExecutionsUI()));
    const quickLaunchSection = $('#dashboard-quick-launch-section');
    if (quickLaunchSection) quickLaunchSection.classList.toggle('hidden', !(remoteControlVisible && canLaunchExecutionsUI()));
  }

  async function refreshFeatureFlags() {
    const previous = { ...featureFlags };
    const previousAuthz = JSON.parse(JSON.stringify(authzState));
    try {
      const payload = await api('GET', '/api/v1/features');
      featureFlags = normalizeFeatureFlags(payload);
      authzState = normalizeAuthz(payload);
    } catch (_e) {
      // Rollout safeguard: keep prior known-good flags instead of failing open.
      featureFlags = previous;
      authzState = previousAuthz;
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
    'dashboard', 'executions', 'memory', 'workers', 'agent-detail', 'logs', 'chat', 'settings',
    'servers', 'profiles', 'remote-chat', 'remote-observability',
  ];

  function showView(name) {
    routes.forEach(r => {
      const el = $('#view-' + r);
      if (el) el.classList.toggle('hidden', r !== name);
    });
    if (name !== 'dashboard' && name !== 'executions') stopDashboardExecutionPolling();
    if (name !== 'workers') stopWorkersPolling();
    // Update nav active state
    $$('.nav-link').forEach(a => {
      a.classList.toggle('active', a.dataset.route === name);
    });
  }

  function navigate(hash) {
    const routeInfo = parseRoute(hash);
    const route = routeInfo.path;
    const routeName = routeInfo.name;

    if (routeName === 'add') {
      const agent = decodeURIComponent(routeInfo.segments[1] || '').trim().toLowerCase();
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
      if (!keepAddRoutes.has(routeName)) {
        resetAddMode();
      }
    }
    if (routeName !== 'dashboard') {
      closeAddAgentModal();
    }

    // Management views require auth
    const mgmtRoutes = ['dashboard', 'executions', 'memory', 'workers', 'logs', 'chat', 'settings', 'servers', 'profiles', 'remote-chat', 'remote-observability'];
    if (mgmtRoutes.includes(routeName)) {
      $('#nav').classList.remove('hidden');
    }

    if (!isRouteEnabled(routeName)) {
      location.hash = '#/dashboard';
      return;
    }

    switch (routeName) {
      case 'welcome': initWelcome(); break;
      case 'setup': initSetup(); break;
      case 'agents': initAgents(); break;
      case 'provider': initProvider(); break;
      case 'config': initConfig(); break;
      case 'install': initInstall(); break;
      case 'complete': initComplete(); break;
      case 'dashboard': initDashboard(); break;
      case 'executions': initExecutions(routeInfo.segments[1] || ''); break;
      case 'memory': initMemory(); break;
      case 'workers': initWorkers(); break;
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
          connectDelegateEvents();
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

  function flattenProviderCatalog(payload) {
    const categories = payload && payload.by_category && typeof payload.by_category === 'object'
      ? payload.by_category
      : {};
    const out = [];
    const seen = new Set();
    Object.keys(categories).forEach(key => {
      const providers = Array.isArray(categories[key]) ? categories[key] : [];
      providers.forEach(provider => {
        const id = String(provider && provider.id ? provider.id : '').trim().toLowerCase();
        if (!id || seen.has(id)) return;
        seen.add(id);
        out.push(provider);
      });
    });
    out.sort((left, right) => String(left && left.name ? left.name : left.id || '').localeCompare(String(right && right.name ? right.name : right.id || '')));
    return out;
  }

  function renderQuickLaunchProviderOptions(providers) {
    const select = $('#quick-launch-provider');
    if (!select) return;
    const previous = String(select.value || '').trim().toLowerCase();
    select.textContent = '';
    const empty = document.createElement('option');
    empty.value = '';
    empty.textContent = 'System default';
    select.appendChild(empty);
    providers.forEach(provider => {
      const opt = document.createElement('option');
      opt.value = provider.id;
      opt.textContent = provider.name || provider.id;
      select.appendChild(opt);
    });
    if (previous && providers.some(provider => String(provider.id || '').trim().toLowerCase() === previous)) {
      select.value = previous;
    }
  }

  function renderQuickLaunchTemplateOptions(templates) {
    const select = $('#quick-launch-template');
    if (!select) return;
    const previous = String(select.value || '').trim().toLowerCase();
    select.textContent = '';
    const empty = document.createElement('option');
    empty.value = '';
    empty.textContent = 'Select a template';
    select.appendChild(empty);
    (Array.isArray(templates) ? templates : []).forEach(template => {
      const id = String(template && template.id ? template.id : '').trim();
      if (!id) return;
      const opt = document.createElement('option');
      opt.value = id;
      opt.textContent = String(template && template.name ? template.name : id);
      select.appendChild(opt);
    });
    if (previous && Array.isArray(templates) && templates.some(template => String(template && template.id ? template.id : '').trim().toLowerCase() === previous)) {
      select.value = previous;
    }
  }

  function selectedQuickLaunchMode() {
    return String((($('#quick-launch-mode') || {}).value || 'goal')).trim().toLowerCase() || 'goal';
  }

  function selectedQuickLaunchTemplateID() {
    return String((($('#quick-launch-template') || {}).value || '')).trim();
  }

  function findQuickLaunchTemplate(templateID) {
    const key = String(templateID || '').trim().toLowerCase();
    return (Array.isArray(quickLaunchTemplates) ? quickLaunchTemplates : []).find(template => String(template && template.id ? template.id : '').trim().toLowerCase() === key) || null;
  }

  function renderQuickLaunchTemplateInputs(templateID) {
    const wrap = $('#quick-launch-template-inputs');
    if (!wrap) return;
    const previous = {};
    $$('[data-quick-launch-template-input]', wrap).forEach(input => {
      previous[String(input.getAttribute('data-quick-launch-template-input') || '').trim()] = String(input.value || '');
    });
    wrap.textContent = '';
    const template = findQuickLaunchTemplate(templateID);
    const schema = template && Array.isArray(template.inputSchema) ? template.inputSchema : [];
    if (!schema.length) return;
    schema.forEach(field => {
      const key = String(field && field.id ? field.id : '').trim();
      if (!key) return;
      const block = document.createElement('div');
      const label = document.createElement('label');
      label.htmlFor = 'quick-launch-template-input-' + key;
      label.textContent = String(field && field.label ? field.label : key);
      const input = document.createElement('input');
      input.id = 'quick-launch-template-input-' + key;
      input.setAttribute('data-quick-launch-template-input', key);
      input.type = 'text';
      input.placeholder = String(field && field.placeholder ? field.placeholder : '');
      input.value = previous[key] != null
        ? String(previous[key])
        : String(field && field.defaultValue ? field.defaultValue : '');
      if (field && field.required) input.required = true;
      block.appendChild(label);
      input.title = String(field && field.description ? field.description : '');
      block.appendChild(input);
      wrap.appendChild(block);
    });
  }

  function collectQuickLaunchTemplateInputs() {
    const values = {};
    $$('[data-quick-launch-template-input]').forEach(input => {
      const key = String(input.getAttribute('data-quick-launch-template-input') || '').trim();
      if (!key) return;
      values[key] = String(input.value || '').trim();
    });
    return values;
  }

  function syncQuickLaunchModeUI() {
    const mode = selectedQuickLaunchMode();
    const goalField = $('#quick-launch-goal-field');
    const templateField = $('#quick-launch-template-field');
    if (goalField) goalField.classList.toggle('hidden', mode !== 'goal');
    if (templateField) templateField.classList.toggle('hidden', mode !== 'template');
    if (mode === 'template') {
      renderQuickLaunchTemplateInputs(selectedQuickLaunchTemplateID());
    }
  }

  function renderQuickLaunchHostOptions(hosts) {
    const wrap = $('#quick-launch-hosts');
    if (!wrap) return;
    const selected = new Set();
    $$('#quick-launch-hosts input[type="checkbox"]:checked').forEach(input => {
      selected.add(String(input.value || '').trim());
    });
    wrap.textContent = '';

    const items = [{ id: 'local', name: 'local' }].concat(Array.isArray(hosts) ? hosts : []);
    const seen = new Set();
    items.forEach(item => {
      const hostID = String(item && item.id ? item.id : '').trim();
      if (!hostID || seen.has(hostID)) return;
      seen.add(hostID);
      const label = document.createElement('label');
      label.className = 'quick-launch-host-option';
      const input = document.createElement('input');
      input.type = 'checkbox';
      input.value = hostID;
      input.checked = selected.size ? selected.has(hostID) : hostID === 'local';
      const text = document.createElement('span');
      text.textContent = String(item && item.name ? item.name : hostID).trim() || hostID;
      label.appendChild(input);
      label.appendChild(text);
      wrap.appendChild(label);
    });
  }

  function selectedQuickLaunchHostIDs() {
    return $$('#quick-launch-hosts input[type="checkbox"]:checked').map(input => String(input.value || '').trim()).filter(Boolean);
  }

  function selectedQuickLaunchHostLabels() {
    return parseCommaSeparatedValues(String((($('#quick-launch-host-labels') || {}).value || '')).trim()).sort((a, b) => a.localeCompare(b));
  }

  function resetQuickLaunchPreview(clearForm) {
    quickLaunchPlan = null;
    const previewCard = $('#quick-launch-preview-card');
    if (previewCard) previewCard.classList.add('hidden');
    if (clearForm) {
      const mode = $('#quick-launch-mode');
      const goal = $('#quick-launch-goal');
      const template = $('#quick-launch-template');
      const provider = $('#quick-launch-provider');
      const maxConcurrency = $('#quick-launch-max-concurrency');
      const hostLabels = $('#quick-launch-host-labels');
      if (mode) mode.value = 'goal';
      if (goal) goal.value = '';
      if (template) template.value = '';
      if (provider) provider.value = '';
      if (maxConcurrency) maxConcurrency.value = '';
      if (hostLabels) hostLabels.value = '';
      renderQuickLaunchTemplateInputs('');
      syncQuickLaunchModeUI();
      renderQuickLaunchHostOptions(remoteHostsCache);
      setMsg('#quick-launch-msg', '', 'info');
    }
  }

  function renderQuickLaunchPlan(plan) {
    quickLaunchPlan = plan;
    const previewCard = $('#quick-launch-preview-card');
    const summary = $('#quick-launch-preview-summary');
    const tasks = $('#quick-launch-preview-tasks');
    const workers = $('#quick-launch-preview-workers');
    if (!previewCard || !summary || !tasks || !workers) return;

    previewCard.classList.remove('hidden');
    const templateID = String(plan && plan.templateId ? plan.templateId : '').trim();
    summary.textContent = 'Approval: ' + String(plan.approvalScope || 'infrastructure_only') +
      (templateID ? ' · Template: ' + templateID : '') +
      ' · Task units: ' + String(Array.isArray(plan.taskUnits) ? plan.taskUnits.length : 0) +
      ' · Max concurrency: ' + String(plan.maxConcurrency || 0);
    tasks.textContent = '';
    workers.textContent = '';

    const plannerTasks = Array.isArray(plan.plannerTasks) ? plan.plannerTasks : [];
    plannerTasks.forEach(task => {
      const row = document.createElement('div');
      row.className = 'quick-launch-line';
      row.textContent = String(task.id || 'task') + ' · ' + String(task.agentId || 'zeroclaw') + ' · ' + String(task.input || '').trim();
      tasks.appendChild(row);
    });

    const requiredWorkers = Array.isArray(plan.requiredWorkers) ? plan.requiredWorkers : [];
    requiredWorkers.forEach(worker => {
      const row = document.createElement('div');
      row.className = 'quick-launch-line';
      const hostLabels = Array.isArray(worker.hostLabels) ? worker.hostLabels.map(value => String(value || '').trim()).filter(Boolean) : [];
      const hostTarget = String(worker.hostId || '').trim() || (hostLabels.length ? ('labels[' + hostLabels.join(',') + ']') : 'local');
      row.textContent = hostTarget + '/' + String(worker.agentId || 'zeroclaw') + ' · count=' + String(worker.count || 1);
      workers.appendChild(row);
    });
  }

  async function loadQuickLaunchOptions() {
    if (!featureFlags.remoteControlPlaneEnabled) return;
    const [providerPayload, hosts, templatesPayload] = await Promise.all([
      api('GET', '/api/v1/providers'),
      fetchRemoteHosts().catch(() => []),
      api('GET', '/api/v1/templates').catch(() => ({ templates: [] })),
    ]);
    quickLaunchProviderCatalog = flattenProviderCatalog(providerPayload);
    quickLaunchTemplates = Array.isArray(templatesPayload && templatesPayload.templates) ? templatesPayload.templates : [];
    renderQuickLaunchProviderOptions(quickLaunchProviderCatalog);
    renderQuickLaunchTemplateOptions(quickLaunchTemplates);
    renderQuickLaunchTemplateInputs(selectedQuickLaunchTemplateID());
    syncQuickLaunchModeUI();
    renderQuickLaunchHostOptions(hosts);
  }

  async function previewQuickLaunchPlan() {
    const mode = selectedQuickLaunchMode();
    const goal = String(($('#quick-launch-goal') || {}).value || '').trim();
    const templateID = selectedQuickLaunchTemplateID();
    const templateInputs = collectQuickLaunchTemplateInputs();
    if (mode === 'goal' && !goal) {
      setMsg('#quick-launch-msg', 'Goal is required.', 'error');
      return;
    }
    if (mode === 'template' && !templateID) {
      setMsg('#quick-launch-msg', 'Template is required.', 'error');
      return;
    }
    const previewBtn = $('#quick-launch-preview');
    if (previewBtn) previewBtn.disabled = true;
    try {
      const provider = String(($('#quick-launch-provider') || {}).value || '').trim();
      const maxConcurrency = parseInt(String((($('#quick-launch-max-concurrency') || {}).value || '')).trim(), 10);
      const hostLabels = selectedQuickLaunchHostLabels();
      const response = await api('POST', '/api/v1/orchestrator/plans', {
        goal: mode === 'goal' ? goal : '',
        templateId: mode === 'template' ? templateID : '',
        inputs: mode === 'template' ? templateInputs : {},
        provider,
        hostIds: hostLabels.length ? [] : selectedQuickLaunchHostIDs(),
        hostLabels,
        maxConcurrency: Number.isFinite(maxConcurrency) ? maxConcurrency : 0,
      });
      renderQuickLaunchPlan(response && response.plan ? response.plan : {});
      setMsg('#quick-launch-msg', 'Preview ready. Confirm to create and authorize the execution.', 'info');
    } catch (e) {
      setMsg('#quick-launch-msg', 'Preview failed: ' + e.message, 'error');
    } finally {
      if (previewBtn) previewBtn.disabled = false;
    }
  }

  async function runQuickLaunchExecution() {
    if (!quickLaunchPlan) {
      setMsg('#quick-launch-msg', 'Preview a plan before running.', 'error');
      return;
    }
    const runBtn = $('#quick-launch-run');
    if (runBtn) runBtn.disabled = true;
    try {
      const createPayload = {
        goal: String(quickLaunchPlan.goal || '').trim(),
        templateId: String(quickLaunchPlan.templateId || '').trim(),
        requestedProvider: String(quickLaunchPlan.provider || '').trim(),
        approvalScope: String(quickLaunchPlan.approvalScope || 'infrastructure_only').trim(),
        requiredWorkers: Array.isArray(quickLaunchPlan.requiredWorkers) ? quickLaunchPlan.requiredWorkers : [],
        taskUnits: Array.isArray(quickLaunchPlan.taskUnits) ? quickLaunchPlan.taskUnits : [],
        maxConcurrency: Number(quickLaunchPlan.maxConcurrency || 0) || 0,
      };
      const created = await api('POST', '/api/v1/orchestrator/executions', createPayload);
      const execution = created && created.execution ? created.execution : {};
      const executionID = String(execution.id || '').trim();
      if (!executionID) throw new Error('create response missing execution id');
      await api('POST', '/api/v1/orchestrator/executions/' + encodeURIComponent(executionID) + '/authorize', {
        approved: true,
        actor: 'webui',
        maxConcurrency: Number(quickLaunchPlan.maxConcurrency || 0) || 0,
      });
      setMsg('#quick-launch-msg', 'Execution created: ' + executionID, 'info');
      location.hash = '#/executions/' + encodeURIComponent(executionID);
    } catch (e) {
      const payload = e && e.payload && typeof e.payload === 'object' ? e.payload : null;
      const blockedExecution = payload && payload.execution && typeof payload.execution === 'object'
        ? payload.execution
        : null;
      const blockedExecutionID = String(blockedExecution && blockedExecution.id ? blockedExecution.id : '').trim();
      if (e && Number(e.status || 0) === 409 && blockedExecutionID) {
        setMsg('#quick-launch-msg', 'Execution created but waiting for policy approval: ' + blockedExecutionID, 'error');
        location.hash = '#/executions/' + encodeURIComponent(blockedExecutionID);
        return;
      }
      setMsg('#quick-launch-msg', 'Run failed: ' + e.message, 'error');
    } finally {
      if (runBtn) runBtn.disabled = false;
    }
  }

  async function initQuickLaunch() {
    const section = $('#dashboard-quick-launch-section');
    if (!section) return;
    const quickLaunchEnabled = featureFlags.remoteControlPlaneEnabled && canLaunchExecutionsUI();
    section.classList.toggle('hidden', !quickLaunchEnabled);
    if (!quickLaunchEnabled) return;

    const previewBtn = $('#quick-launch-preview');
    const resetBtn = $('#quick-launch-reset');
    const editBtn = $('#quick-launch-edit');
    const runBtn = $('#quick-launch-run');
    const toggleBtn = $('#quick-launch-advanced-toggle');
    const advanced = $('#quick-launch-advanced');
    const modeSelect = $('#quick-launch-mode');
    const templateSelect = $('#quick-launch-template');

    if (previewBtn) previewBtn.onclick = previewQuickLaunchPlan;
    if (resetBtn) resetBtn.onclick = () => resetQuickLaunchPreview(true);
    if (editBtn) editBtn.onclick = () => {
      const previewCard = $('#quick-launch-preview-card');
      if (previewCard) previewCard.classList.add('hidden');
    };
    if (runBtn) runBtn.onclick = runQuickLaunchExecution;
    if (modeSelect) modeSelect.onchange = () => syncQuickLaunchModeUI();
    if (templateSelect) templateSelect.onchange = () => renderQuickLaunchTemplateInputs(templateSelect.value);
    if (toggleBtn) toggleBtn.onclick = () => {
      if (!advanced) return;
      const hidden = advanced.classList.toggle('hidden');
      toggleBtn.textContent = hidden ? 'Advanced' : 'Hide Advanced';
    };

    try {
      await loadQuickLaunchOptions();
    } catch (e) {
      setMsg('#quick-launch-msg', 'Failed to load orchestration options: ' + e.message, 'error');
    }
  }

  // --- Dashboard ---
  async function initDashboard() {
    resetAddMode();
    showView('dashboard');
    $('#nav').classList.remove('hidden');
    await Promise.all([
      refreshInstances(),
      initQuickLaunch(),
    ]);
    await refreshExecutions();
    startDashboardExecutionPolling();
    $('#refresh-instances').onclick = refreshInstances;
    const refreshExecutionsBtn = $('#refresh-executions');
    if (refreshExecutionsBtn) refreshExecutionsBtn.onclick = refreshExecutions;
    $('#dashboard-add-agent').onclick = openAddAgentModal;
    $('#add-agent-cancel').onclick = closeAddAgentModal;
  }

  async function initExecutions(executionID) {
    resetAddMode();
    showView('executions');
    $('#nav').classList.remove('hidden');
    selectedExecutionID = String(executionID || '').trim();
    applyExecutionRouteFilters(parseRoute(location.hash));

    const refreshBtn = $('#executions-refresh');
    const searchInput = $('#executions-search');
    const statusFilter = $('#executions-status-filter');
    const templateFilter = $('#executions-template-filter');
    const triggerFilter = $('#executions-trigger-filter');
    if (refreshBtn) refreshBtn.onclick = refreshExecutions;
    if (searchInput) searchInput.oninput = () => { void renderExecutionsView(executionRecordsCache, false); };
    if (statusFilter) statusFilter.onchange = () => { void renderExecutionsView(executionRecordsCache, false); };
    if (templateFilter) templateFilter.onchange = () => { void renderExecutionsView(executionRecordsCache, false); };
    if (triggerFilter) triggerFilter.onchange = () => { void renderExecutionsView(executionRecordsCache, false); };

    await refreshExecutions();
    startDashboardExecutionPolling();
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

  function isExecutionTerminalStatus(status) {
    const normalized = String(status || '').trim().toLowerCase();
    return normalized === 'completed' || normalized === 'partial_completed' || normalized === 'failed' || normalized === 'retryable_failed' || normalized === 'cancelled' || normalized === 'declined';
  }

  function executionStatusBadgeClass(status) {
    const normalized = String(status || '').trim().toLowerCase();
    if (normalized === 'completed') return 'badge badge-ok';
    if (normalized === 'partial_completed') return 'badge badge-warn';
    if (normalized === 'failed' || normalized === 'retryable_failed' || normalized === 'declined' || normalized === 'cancelled') return 'badge badge-error';
    return 'badge badge-unknown';
  }

  function executionHasFailedTasks(execution) {
    const results = Array.isArray(execution && execution.results) ? execution.results : [];
    return results.some(item => String(item && item.status ? item.status : '').trim().toLowerCase() === 'failed');
  }

  function artifactDownloadPath(executionID, artifactID) {
    return '/api/v1/orchestrator/executions/' + encodeURIComponent(String(executionID || '').trim()) + '/artifacts/' + encodeURIComponent(String(artifactID || '').trim());
  }

  function evidenceDownloadPath(executionID) {
    return '/api/v1/orchestrator/executions/' + encodeURIComponent(String(executionID || '').trim()) + '/evidence?format=zip';
  }

  function auditExportDownloadPath(executionID) {
    return '/api/v1/audit/export?executionId=' + encodeURIComponent(String(executionID || '').trim());
  }

  function buildExecutionsHash(params) {
    const searchParams = new URLSearchParams();
    Object.entries(params || {}).forEach(([key, value]) => {
      const text = String(value || '').trim();
      if (!text) return;
      searchParams.set(key, text);
    });
    const suffix = searchParams.toString();
    return '#/executions' + (suffix ? ('?' + suffix) : '');
  }

  function applyExecutionRouteFilters(routeInfo) {
    const query = routeInfo && routeInfo.query && typeof routeInfo.query === 'object' ? routeInfo.query : {};
    const searchInput = $('#executions-search');
    const statusFilter = $('#executions-status-filter');
    const templateFilter = $('#executions-template-filter');
    const triggerFilter = $('#executions-trigger-filter');
    if (searchInput && typeof query.search === 'string') searchInput.value = query.search;
    if (statusFilter && typeof query.status === 'string') statusFilter.value = query.status;
    if (templateFilter && typeof query.template === 'string') templateFilter.value = query.template;
    if (triggerFilter && typeof query.trigger === 'string') triggerFilter.value = query.trigger;
  }

  function workerStateBadgeClass(state) {
    const normalized = String(state || '').trim().toLowerCase();
    if (normalized === 'available' || normalized === 'managed') return 'badge badge-ok';
    if (normalized === 'busy' || normalized === 'provisioning' || normalized === 'reclaiming') return 'badge badge-warn';
    if (normalized === 'error') return 'badge badge-error';
    return 'badge badge-unknown';
  }

  function stopDashboardExecutionPolling() {
    if (dashboardExecutionPollTimer) {
      clearInterval(dashboardExecutionPollTimer);
      dashboardExecutionPollTimer = null;
    }
  }

  function startDashboardExecutionPolling() {
    stopDashboardExecutionPolling();
    if (!featureFlags.remoteControlPlaneEnabled) return;
    dashboardExecutionPollTimer = setInterval(() => {
      const routeName = currentRouteName();
      if (routeName !== 'dashboard' && routeName !== 'executions') {
        stopDashboardExecutionPolling();
        return;
      }
      void refreshExecutions();
    }, 5000);
  }

  function stopWorkersPolling() {
    if (workersPollTimer) {
      clearInterval(workersPollTimer);
      workersPollTimer = null;
    }
  }

  function startWorkersPolling() {
    stopWorkersPolling();
    if (!featureFlags.remoteControlPlaneEnabled) return;
    workersPollTimer = setInterval(() => {
      if (currentRouteName() !== 'workers') {
        stopWorkersPolling();
        return;
      }
      void refreshWorkers();
    }, 5000);
  }

  async function fetchExecutionDetails(executionID, force) {
    const id = String(executionID || '').trim();
    if (!id) throw new Error('execution id is required');
    if (!force && dashboardExecutionDetailsByID[id]) {
      return dashboardExecutionDetailsByID[id];
    }
    const payload = await api('GET', '/api/v1/orchestrator/executions/' + encodeURIComponent(id));
    dashboardExecutionDetailsByID[id] = payload;
    return payload;
  }

  async function loadExecutionDetails(executionID, target, force) {
    const id = String(executionID || '').trim();
    if (!id || !target) return;
    target.textContent = 'Loading details…';
    try {
      const payload = await fetchExecutionDetails(id, force);
      renderExecutionDetails(target, payload);
    } catch (e) {
      target.textContent = 'Load failed: ' + e.message;
    }
  }

  function renderExecutionDetails(target, payload) {
    target.textContent = '';
    const execution = payload && payload.execution && typeof payload.execution === 'object' ? payload.execution : {};
    const workers = payload && Array.isArray(payload.workers) ? payload.workers : [];
    const taskUnits = Array.isArray(execution.taskUnits) ? execution.taskUnits : [];
    const results = Array.isArray(execution.results) ? execution.results : [];
    const resultByTaskId = new Map();
    results.forEach(item => {
      const key = String(item && item.taskId ? item.taskId : '').trim();
      if (key) resultByTaskId.set(key, item);
    });

    const workerWrap = document.createElement('div');
    workerWrap.className = 'execution-detail-block';
    const workerTitle = document.createElement('div');
    workerTitle.className = 'execution-detail-title';
    workerTitle.textContent = 'Workers';
    workerWrap.appendChild(workerTitle);
    if (!workers.length) {
      const empty = document.createElement('div');
      empty.className = 'text-dim';
      empty.textContent = 'No worker leases recorded.';
      workerWrap.appendChild(empty);
    } else {
      workers.forEach(worker => {
        const line = document.createElement('div');
        line.className = 'execution-detail-line';
        const host = String(worker && worker.hostId ? worker.hostId : '').trim() || 'local';
        const agent = String(worker && worker.agentId ? worker.agentId : '').trim() || 'unknown';
        const state = String(worker && worker.state ? worker.state : '').trim() || 'unknown';
        line.textContent = host + '/' + agent + ' · state=' + state;
        workerWrap.appendChild(line);
      });
    }
    target.appendChild(workerWrap);

    const resultsWrap = document.createElement('div');
    resultsWrap.className = 'execution-detail-block';
    const resultTitle = document.createElement('div');
    resultTitle.className = 'execution-detail-title';
    resultTitle.textContent = 'Task Results';
    resultsWrap.appendChild(resultTitle);
    if (!taskUnits.length) {
      const empty = document.createElement('div');
      empty.className = 'text-dim';
      empty.textContent = 'No task units recorded.';
      resultsWrap.appendChild(empty);
    } else {
      taskUnits.forEach((task, index) => {
        const taskID = String(task && task.id ? task.id : 'task-' + String(index + 1)).trim();
        const taskInput = String(task && task.input ? task.input : '').trim();
        const result = resultByTaskId.get(taskID) || {};
        const item = document.createElement('div');
        item.className = 'execution-result-item';

        const header = document.createElement('div');
        header.className = 'execution-result-header';
        const title = document.createElement('strong');
        title.textContent = taskID;
        const status = document.createElement('span');
        status.className = executionStatusBadgeClass(result.status || execution.status);
        status.textContent = String(result.status || execution.status || 'pending').trim();
        header.appendChild(title);
        header.appendChild(status);
        item.appendChild(header);

        if (taskInput) {
          const body = document.createElement('div');
          body.className = 'execution-result-body';
          body.textContent = taskInput;
          item.appendChild(body);
        }

        const resultSummary = String(result.summary || '').trim();
        if (resultSummary) {
          const summary = document.createElement('div');
          summary.className = 'execution-result-body';
          summary.textContent = resultSummary;
          item.appendChild(summary);
        }

        const failureReason = String(result.failureReason || '').trim();
        const failureCategory = String(result.failureCategory || '').trim();
        if (failureReason || failureCategory) {
          const failure = document.createElement('div');
          failure.className = 'execution-result-meta';
          const parts = [];
          if (failureReason) parts.push('reason=' + failureReason);
          if (failureCategory) parts.push('category=' + failureCategory);
          failure.textContent = parts.join(' · ');
          item.appendChild(failure);
        }

        const output = String(result.output || result.error || '').trim();
        if (output) {
          const pre = document.createElement('pre');
          pre.className = 'code-block execution-result-output';
          pre.textContent = output;
          item.appendChild(pre);
        }

        const meta = document.createElement('div');
        meta.className = 'execution-result-meta';
        const host = String(result.hostId || '').trim();
        const agent = String(result.agentId || '').trim();
        const attempts = Number(result.attempts || 0);
        const latency = Number(result.latencyMs || 0);
        const parts = [];
        if (host || agent) parts.push((host || 'local') + '/' + (agent || 'unknown'));
        if (attempts) parts.push('attempts=' + String(attempts));
        if (latency) parts.push('latency=' + String(Math.round(latency)) + 'ms');
        meta.textContent = parts.join(' · ');
        if (meta.textContent) item.appendChild(meta);

        resultsWrap.appendChild(item);
      });
    }
    target.appendChild(resultsWrap);
  }

  function sortExecutionsByUpdatedAt(executions) {
    return (Array.isArray(executions) ? executions.slice() : []).sort((a, b) => {
      const left = new Date(String(a && a.updatedAt ? a.updatedAt : '')).getTime() || 0;
      const right = new Date(String(b && b.updatedAt ? b.updatedAt : '')).getTime() || 0;
      return right - left;
    });
  }

  function executionCounts(execution) {
    const taskUnits = Array.isArray(execution && execution.taskUnits) ? execution.taskUnits : [];
    const results = Array.isArray(execution && execution.results) ? execution.results : [];
    return {
      taskUnits,
      results,
      completed: results.filter(item => String(item && item.status ? item.status : '').trim() === 'completed').length,
      failed: results.filter(item => {
        const status = String(item && item.status ? item.status : '').trim().toLowerCase();
        return status === 'failed' || status === 'cancelled';
      }).length,
    };
  }

  function executionTemplateValue(execution) {
    return String(execution && execution.templateId ? execution.templateId : '').trim();
  }

  function executionTriggerValue(execution) {
    return String(execution && execution.triggerSource ? execution.triggerSource : '').trim().toLowerCase();
  }

  function executionTriggerLabel(execution) {
    const source = String(execution && execution.triggerSource ? execution.triggerSource : '').trim();
    const triggerID = String(execution && execution.triggerId ? execution.triggerId : '').trim();
    if (source && triggerID) return source + ':' + triggerID;
    return source || triggerID;
  }

  function executionSearchText(execution) {
    return [
      String(execution && execution.id ? execution.id : ''),
      String(execution && execution.goal ? execution.goal : ''),
      String(execution && execution.team ? execution.team : ''),
      String(execution && execution.project ? execution.project : ''),
      String(execution && execution.environment ? execution.environment : ''),
      executionTemplateValue(execution),
      String(execution && execution.triggerSource ? execution.triggerSource : ''),
      String(execution && execution.triggerId ? execution.triggerId : ''),
      executionTriggerLabel(execution),
      String(execution && execution.initiator ? execution.initiator : ''),
    ].join(' ').trim().toLowerCase();
  }

  function executionAttributionParts(execution) {
    const parts = [];
    const team = String(execution && execution.team ? execution.team : '').trim();
    const project = String(execution && execution.project ? execution.project : '').trim();
    const environment = String(execution && execution.environment ? execution.environment : '').trim();
    const templateID = executionTemplateValue(execution);
    const trigger = executionTriggerLabel(execution);
    if (team) parts.push('Team: ' + team);
    if (project) parts.push('Project: ' + project);
    if (environment) parts.push('Env: ' + environment);
    if (templateID) parts.push('Template: ' + templateID);
    if (trigger) parts.push('Trigger: ' + trigger);
    return parts;
  }

  function syncExecutionFilterOptions(executions) {
    const templateFilter = $('#executions-template-filter');
    const triggerFilter = $('#executions-trigger-filter');
    if (templateFilter) {
      const current = String(templateFilter.value || 'all').trim() || 'all';
      const templates = [...new Set((Array.isArray(executions) ? executions : [])
        .map(item => executionTemplateValue(item))
        .filter(Boolean))]
        .sort((left, right) => left.localeCompare(right));
      templateFilter.innerHTML = '<option value="all">All</option>' + templates.map(value => (
        '<option value="' + escapeHtml(value) + '">' + escapeHtml(value) + '</option>'
      )).join('');
      templateFilter.value = templates.includes(current) ? current : 'all';
    }
    if (triggerFilter) {
      const current = String(triggerFilter.value || 'all').trim().toLowerCase() || 'all';
      const triggers = [...new Set((Array.isArray(executions) ? executions : [])
        .map(item => executionTriggerValue(item))
        .filter(Boolean))]
        .sort((left, right) => left.localeCompare(right));
      triggerFilter.innerHTML = '<option value="all">All</option>' + triggers.map(value => (
        '<option value="' + escapeHtml(value) + '">' + escapeHtml(value) + '</option>'
      )).join('');
      triggerFilter.value = triggers.includes(current) ? current : 'all';
    }
  }

  function executionMatchesFilter(execution, filterValue, query, templateFilterValue, triggerFilterValue) {
    const status = String(execution && execution.status ? execution.status : '').trim().toLowerCase();
    const normalizedQuery = String(query || '').trim().toLowerCase();
    const normalizedTemplate = String(templateFilterValue || 'all').trim();
    const normalizedTrigger = String(triggerFilterValue || 'all').trim().toLowerCase();
    if (normalizedQuery && !executionSearchText(execution).includes(normalizedQuery)) {
      return false;
    }
    if (normalizedTemplate && normalizedTemplate !== 'all' && executionTemplateValue(execution) !== normalizedTemplate) {
      return false;
    }
    if (normalizedTrigger && normalizedTrigger !== 'all' && executionTriggerValue(execution) !== normalizedTrigger) {
      return false;
    }
    switch (String(filterValue || 'all').trim().toLowerCase()) {
      case 'active':
        return !isExecutionTerminalStatus(status);
      case 'completed':
        return status === 'completed' || status === 'partial_completed';
      case 'failed':
        return status === 'failed' || status === 'retryable_failed' || status === 'declined';
      case 'cancelled':
        return status === 'cancelled';
      default:
        return true;
    }
  }

  async function cancelExecution(executionID, actor) {
    const id = String(executionID || '').trim();
    if (!id) return;
    await api('POST', '/api/v1/orchestrator/executions/' + encodeURIComponent(id) + '/cancel', {
      actor: String(actor || 'webui').trim() || 'webui',
    });
    delete dashboardExecutionDetailsByID[id];
    await refreshExecutions();
  }

  async function authorizeExecution(executionID, actor, policyApprove) {
    const id = String(executionID || '').trim();
    if (!id) return;
    await api('POST', '/api/v1/orchestrator/executions/' + encodeURIComponent(id) + '/authorize', {
      approved: true,
      actor: String(actor || 'webui').trim() || 'webui',
      policyApproved: !!policyApprove,
    });
    delete dashboardExecutionDetailsByID[id];
    await refreshExecutions();
  }

  async function createDerivedExecution(executionID, action) {
    const id = String(executionID || '').trim();
    const normalizedAction = String(action || '').trim().toLowerCase();
    if (!id || !normalizedAction) return null;
    const payload = await api('POST', '/api/v1/orchestrator/executions/' + encodeURIComponent(id) + '/' + encodeURIComponent(normalizedAction));
    const execution = payload && payload.execution && typeof payload.execution === 'object' ? payload.execution : null;
    if (!execution || !execution.id) {
      throw new Error('derived execution was not returned');
    }
    dashboardExecutionDetailsByID[String(execution.id).trim()] = {
      result: 'ok',
      execution,
      workers: [],
    };
    await refreshExecutions();
    return execution;
  }

  function renderExecutionsDetailPanel(target, payload) {
    target.textContent = '';
    const execution = payload && payload.execution && typeof payload.execution === 'object' ? payload.execution : {};
    const statusText = String(execution && execution.status ? execution.status : 'unknown').trim();
    const summaryCard = document.createElement('div');
    summaryCard.className = 'executions-detail-summary';

    const header = document.createElement('div');
    header.className = 'section-head';
    const titleWrap = document.createElement('div');
    const title = document.createElement('h3');
    title.textContent = String(execution && execution.goal ? execution.goal : '').trim() || '(no goal)';
    const meta = document.createElement('div');
    meta.className = 'execution-detail-line';
    meta.textContent = 'ID: ' + String(execution && execution.id ? execution.id : '').trim() + ' · Updated: ' + formatDateTime(execution && execution.updatedAt);
    titleWrap.appendChild(title);
    titleWrap.appendChild(meta);
    const badge = document.createElement('span');
    badge.className = executionStatusBadgeClass(statusText);
    badge.textContent = statusText || 'unknown';
    header.appendChild(titleWrap);
    header.appendChild(badge);
    summaryCard.appendChild(header);

    const statusLine = document.createElement('div');
    statusLine.className = 'execution-detail-line';
    statusLine.textContent = 'status: ' + (statusText || 'unknown');
    summaryCard.appendChild(statusLine);

    const error = String(execution && execution.error ? execution.error : '').trim();
    if (error) {
      const errorLine = document.createElement('div');
      errorLine.className = 'execution-detail-line';
      errorLine.textContent = 'Error: ' + error;
      summaryCard.appendChild(errorLine);
    }
    target.appendChild(summaryCard);

    const triggerSource = String(execution && execution.triggerSource ? execution.triggerSource : '').trim();
    const triggerID = String(execution && execution.triggerId ? execution.triggerId : '').trim();
    const triggerEvent = String(execution && execution.triggerEvent ? execution.triggerEvent : '').trim();
    const initiator = String(execution && execution.initiator ? execution.initiator : '').trim();
    if (triggerSource || triggerID || triggerEvent || initiator) {
      const triggerCard = document.createElement('div');
      triggerCard.className = 'execution-detail-block';
      const triggerTitle = document.createElement('div');
      triggerTitle.className = 'execution-detail-title';
      triggerTitle.textContent = 'Trigger';
      triggerCard.appendChild(triggerTitle);
      if (triggerSource) {
        const row = document.createElement('div');
        row.className = 'execution-detail-line';
        row.textContent = 'source: ' + triggerSource;
        triggerCard.appendChild(row);
      }
      if (triggerID) {
        const row = document.createElement('div');
        row.className = 'execution-detail-line';
        row.textContent = 'id: ' + triggerID;
        triggerCard.appendChild(row);
      }
      if (triggerEvent) {
        const row = document.createElement('div');
        row.className = 'execution-detail-line';
        row.textContent = 'event: ' + triggerEvent;
        triggerCard.appendChild(row);
      }
      if (initiator) {
        const row = document.createElement('div');
        row.className = 'execution-detail-line';
        row.textContent = 'initiator: ' + initiator;
        triggerCard.appendChild(row);
      }
      target.appendChild(triggerCard);
    }

    const parentExecutionID = String(execution && execution.parentExecutionId ? execution.parentExecutionId : '').trim();
    const sourceExecutionID = String(execution && execution.sourceExecutionId ? execution.sourceExecutionId : '').trim();
    const launchReason = String(execution && execution.launchReason ? execution.launchReason : '').trim();
    if (parentExecutionID || sourceExecutionID || launchReason) {
      const lineageCard = document.createElement('div');
      lineageCard.className = 'execution-detail-block';
      const lineageTitle = document.createElement('div');
      lineageTitle.className = 'execution-detail-title';
      lineageTitle.textContent = 'Execution Lineage';
      lineageCard.appendChild(lineageTitle);

      if (parentExecutionID) {
        const row = document.createElement('div');
        row.className = 'execution-detail-line';
        row.textContent = 'parent: ' + parentExecutionID;
        lineageCard.appendChild(row);
      }
      if (sourceExecutionID) {
        const row = document.createElement('div');
        row.className = 'execution-detail-line';
        row.textContent = 'source: ' + sourceExecutionID;
        lineageCard.appendChild(row);
      }
      if (launchReason) {
        const row = document.createElement('div');
        row.className = 'execution-detail-line';
        row.textContent = 'launch reason: ' + launchReason;
        lineageCard.appendChild(row);
      }
      target.appendChild(lineageCard);
    }

    const outcome = execution && execution.outcome && typeof execution.outcome === 'object'
      ? execution.outcome
      : {};
    const outcomeSummary = String(outcome && outcome.summary ? outcome.summary : '').trim();
    const outcomeFailureReason = String(outcome && outcome.failureReason ? outcome.failureReason : '').trim();
    const outcomeFailureCategory = String(outcome && outcome.failureCategory ? outcome.failureCategory : '').trim();
    const artifacts = Array.isArray(outcome && outcome.artifacts) ? outcome.artifacts : [];
    if (outcomeSummary || outcomeFailureReason || outcomeFailureCategory || artifacts.length) {
      const outcomeCard = document.createElement('div');
      outcomeCard.className = 'execution-detail-block';
      const outcomeTitle = document.createElement('div');
      outcomeTitle.className = 'execution-detail-title';
      outcomeTitle.textContent = 'Outcome';
      outcomeCard.appendChild(outcomeTitle);

      if (outcomeSummary) {
        const row = document.createElement('div');
        row.className = 'execution-detail-line';
        row.textContent = 'Summary: ' + outcomeSummary;
        outcomeCard.appendChild(row);
      }
      if (outcomeFailureReason) {
        const row = document.createElement('div');
        row.className = 'execution-detail-line';
        row.textContent = 'Failure reason: ' + outcomeFailureReason;
        outcomeCard.appendChild(row);
      }
      if (outcomeFailureCategory) {
        const row = document.createElement('div');
        row.className = 'execution-detail-line';
        row.textContent = 'Failure category: ' + outcomeFailureCategory;
        outcomeCard.appendChild(row);
      }
      if (artifacts.length) {
        const artifactTitle = document.createElement('div');
        artifactTitle.className = 'execution-detail-line';
        artifactTitle.textContent = 'Artifacts';
        outcomeCard.appendChild(artifactTitle);
        artifacts.forEach(item => {
          const artifactID = String(item && item.id ? item.id : '').trim();
          if (!artifactID) return;
          const name = String(item && item.name ? item.name : artifactID).trim();
          const kind = String(item && item.kind ? item.kind : '').trim();
          const contentType = String(item && item.contentType ? item.contentType : '').trim();
          const sizeBytes = Number(item && item.sizeBytes ? item.sizeBytes : 0);
          const createdAt = String(item && item.createdAt ? item.createdAt : '').trim();
          const row = document.createElement('div');
          row.className = 'execution-detail-line';
          const metaParts = [];
          if (kind) metaParts.push(kind);
          if (contentType) metaParts.push(contentType);
          if (sizeBytes > 0) metaParts.push(String(sizeBytes) + ' bytes');
          if (createdAt) metaParts.push(formatDateTime(createdAt));
          row.textContent = name + (metaParts.length ? ' · ' + metaParts.join(' · ') : '');
          outcomeCard.appendChild(row);

          const downloadLink = document.createElement('a');
          downloadLink.className = 'btn-sm btn-secondary';
          downloadLink.href = artifactDownloadPath(execution.id, artifactID);
          downloadLink.textContent = 'Download ' + name;
          downloadLink.onclick = async (event) => {
            event.preventDefault();
            downloadLink.setAttribute('aria-busy', 'true');
            try {
              await downloadAPI(artifactDownloadPath(execution.id, artifactID), name);
            } finally {
              downloadLink.removeAttribute('aria-busy');
            }
          };
          outcomeCard.appendChild(downloadLink);
        });
      }
      target.appendChild(outcomeCard);
    }

    const policy = execution && execution.policy && typeof execution.policy === 'object'
      ? execution.policy
      : {};
    const policyDecision = String(policy && policy.decision ? policy.decision : '').trim();
    if (policyDecision) {
      const policyCard = document.createElement('div');
      policyCard.className = 'execution-detail-block';
      const policyTitle = document.createElement('div');
      policyTitle.className = 'execution-detail-title';
      policyTitle.textContent = 'Execution Policy';
      policyCard.appendChild(policyTitle);

      const policyLines = [];
      const policySummary = String(policy && policy.summary ? policy.summary : '').trim();
      const toolPolicy = policy && policy.toolPolicy && typeof policy.toolPolicy === 'object'
        ? policy.toolPolicy
        : {};
      const toolMode = String(toolPolicy && toolPolicy.mode ? toolPolicy.mode : '').trim();
      const allowedTools = Array.isArray(toolPolicy && toolPolicy.allowedTools) ? toolPolicy.allowedTools : [];
      const configuredMaxConcurrency = Number(policy && policy.configuredMaxConcurrency ? policy.configuredMaxConcurrency : 0);
      const effectiveMaxConcurrency = Number(policy && policy.effectiveMaxConcurrency ? policy.effectiveMaxConcurrency : 0);
      const maxTaskTimeoutMs = Number(policy && policy.maxTaskTimeoutMs ? policy.maxTaskTimeoutMs : 0);
      const maxRetryBudget = Number(policy && policy.maxRetryBudget ? policy.maxRetryBudget : 0);
      const requiresApproval = !!(policy && policy.requiresInfrastructureApproval);
      const policyReason = String(policy && policy.reason ? policy.reason : '').trim();
      const matchedRuleName = String(policy && policy.matchedRuleName ? policy.matchedRuleName : '').trim();
      const policyApprovedBy = String(policy && policy.approvedBy ? policy.approvedBy : '').trim();
      const policyApprovedAt = String(policy && policy.approvedAt ? policy.approvedAt : '').trim();
      const targets = Array.isArray(policy && policy.targets) ? policy.targets : [];

      policyLines.push('Decision: ' + policyDecision);
      policyLines.push('Infrastructure approval required: ' + (requiresApproval ? 'yes' : 'no'));
      if (matchedRuleName) policyLines.push('Matched rule: ' + matchedRuleName);
      if (policyReason) policyLines.push('Reason: ' + policyReason);
      if (toolMode) policyLines.push('tool mode: ' + toolMode);
      if (configuredMaxConcurrency > 0) policyLines.push('configured concurrency: ' + String(configuredMaxConcurrency));
      if (effectiveMaxConcurrency > 0) policyLines.push('effective concurrency: ' + String(effectiveMaxConcurrency));
      if (maxTaskTimeoutMs > 0) policyLines.push('max task timeout: ' + String(maxTaskTimeoutMs) + 'ms');
      policyLines.push('max retry budget: ' + String(maxRetryBudget));
      if (policySummary) policyLines.push('summary: ' + policySummary);
      if (policyApprovedBy) policyLines.push('Approved by: ' + policyApprovedBy);
      if (policyApprovedAt) policyLines.push('Approved at: ' + formatDateTime(policyApprovedAt));
      if (allowedTools.length) {
        policyLines.push('Allowed tools: ' + allowedTools.map(item => String(item || '').trim()).filter(Boolean).join(', '));
      }

      policyLines.forEach(line => {
        const row = document.createElement('div');
        row.className = 'execution-detail-line';
        row.textContent = line;
        policyCard.appendChild(row);
      });

      if (targets.length) {
        const targetsTitle = document.createElement('div');
        targetsTitle.className = 'execution-detail-line';
        targetsTitle.textContent = 'Worker scope';
        policyCard.appendChild(targetsTitle);
        targets.forEach(item => {
          const host = String(item && item.hostId ? item.hostId : '').trim();
          const hostLabels = Array.isArray(item && item.hostLabels) ? item.hostLabels.map(value => String(value || '').trim()).filter(Boolean) : [];
          const agent = String(item && item.agentId ? item.agentId : '').trim() || 'unknown';
          const count = Number(item && item.count ? item.count : 0) || 1;
          const row = document.createElement('div');
          row.className = 'execution-detail-line';
          row.textContent = (host || (hostLabels.length ? ('labels[' + hostLabels.join(',') + ']') : 'local')) + '/' + agent + ' · count=' + String(count);
          policyCard.appendChild(row);
        });
      }
      target.appendChild(policyCard);
    }

    const governanceCard = document.createElement('div');
    governanceCard.className = 'execution-detail-block';
    const governanceTitle = document.createElement('div');
    governanceTitle.className = 'execution-detail-title';
    governanceTitle.textContent = 'Approval & Governance';
    governanceCard.appendChild(governanceTitle);

    const authorization = execution && execution.authorization && typeof execution.authorization === 'object'
      ? execution.authorization
      : {};
    const approvedBy = String(authorization && authorization.approvedBy ? authorization.approvedBy : '').trim();
    const approvedAt = String(authorization && authorization.approvedAt ? authorization.approvedAt : '').trim();
    const infraApproved = !!(authorization && authorization.infrastructureApproved);
    const requestedProvider = String(execution && execution.requestedProvider ? execution.requestedProvider : '').trim();
    const providerResolutions = execution && execution.governance && Array.isArray(execution.governance.providerResolutions)
      ? execution.governance.providerResolutions
      : [];

    const governanceLines = [];
    governanceLines.push('Approved by: ' + (approvedBy || 'n/a'));
    governanceLines.push('Approved at: ' + (approvedAt ? formatDateTime(approvedAt) : 'n/a'));
    governanceLines.push('Infrastructure approved: ' + (infraApproved ? 'yes' : 'no'));
    if (requestedProvider) governanceLines.push('Requested provider: ' + requestedProvider);
    governanceLines.forEach(line => {
      const row = document.createElement('div');
      row.className = 'execution-detail-line';
      row.textContent = line;
      governanceCard.appendChild(row);
    });

    if (!providerResolutions.length) {
      const empty = document.createElement('div');
      empty.className = 'execution-detail-line';
      empty.textContent = 'Provider Governance: no binding resolution recorded.';
      governanceCard.appendChild(empty);
    } else {
      const resolutionTitle = document.createElement('div');
      resolutionTitle.className = 'execution-detail-line';
      resolutionTitle.textContent = 'Provider Governance';
      governanceCard.appendChild(resolutionTitle);
      providerResolutions.forEach(item => {
        const host = String(item && item.hostId ? item.hostId : '').trim() || 'local';
        const agent = String(item && item.agentId ? item.agentId : '').trim() || 'unknown';
        const source = String(item && item.source ? item.source : 'none').trim();
        const provider = String(item && item.provider ? item.provider : '').trim();
        const model = String(item && item.model ? item.model : '').trim();
        const profileName = String(item && item.profileName ? item.profileName : item && item.profileId ? item.profileId : '').trim();
        const status = String(item && item.status ? item.status : '').trim();
        const syncMode = String(item && item.syncMode ? item.syncMode : '').trim();
        const message = String(item && item.message ? item.message : '').trim();
        const estimatedTokens = toFiniteNumber(item && item.estimatedTotalTokens, 0);
        const estimatedCostUSD = toFiniteNumber(item && item.estimatedCostUsd, 0);
        const successCount = toFiniteNumber(item && item.successfulTasks, 0);
        const failureCount = toFiniteNumber(item && item.failedTasks, 0);
        const avgLatencyMs = toFiniteNumber(item && item.avgLatencyMs, 0);
        const driftState = String(item && item.driftState ? item.driftState : '').trim();
        const driftReason = String(item && item.driftReason ? item.driftReason : '').trim();
        const trace = Array.isArray(item && item.trace) ? item.trace : [];

        const row = document.createElement('div');
        row.className = 'execution-detail-line';
        row.textContent =
          host + '/' + agent +
          ' · source=' + source +
          (profileName ? ' · profile=' + profileName : '') +
          (provider || model ? ' · ' + [provider, model].filter(Boolean).join('/') : '') +
          (status ? ' · status=' + status : '') +
          (syncMode ? ' · sync=' + syncMode : '') +
          (estimatedTokens > 0 ? ' · tokens=' + String(estimatedTokens) : '') +
          (estimatedCostUSD > 0 ? ' · cost=' + formatUSD(estimatedCostUSD) : '') +
          ((successCount > 0 || failureCount > 0) ? ' · tasks=' + String(successCount) + '/' + String(failureCount) : '') +
          (driftState ? ' · drift=' + driftState : '') +
          (avgLatencyMs > 0 ? ' · latency=' + formatMilliseconds(avgLatencyMs) : '');
        governanceCard.appendChild(row);
        if (driftReason) {
          const drift = document.createElement('div');
          drift.className = 'execution-detail-line';
          drift.textContent = driftReason;
          governanceCard.appendChild(drift);
        }
        trace.forEach(traceItem => {
          const traceRow = document.createElement('div');
          traceRow.className = 'execution-detail-line';
          const traceSource = String(traceItem && traceItem.source ? traceItem.source : 'unknown').trim() || 'unknown';
          const traceStatus = String(traceItem && traceItem.status ? traceItem.status : 'unknown').trim() || 'unknown';
          const traceSelected = !!(traceItem && traceItem.selected);
          const traceProviderModel = [String(traceItem && traceItem.provider ? traceItem.provider : ''), String(traceItem && traceItem.model ? traceItem.model : '')]
            .filter(Boolean)
            .join('/');
          traceRow.textContent = traceSource + ' [' + traceStatus + (traceSelected ? ', selected' : '') + ']' + (traceProviderModel ? ' ' + traceProviderModel : '');
          governanceCard.appendChild(traceRow);
        });
        if (message) {
          const msg = document.createElement('div');
          msg.className = 'execution-detail-line';
          msg.textContent = message;
          governanceCard.appendChild(msg);
        }
      });
    }
    target.appendChild(governanceCard);

    const blocks = document.createElement('div');
    renderExecutionDetails(blocks, payload);
    target.appendChild(blocks);
  }

  async function renderExecutionsView(executions, forceDetail) {
    const list = $('#executions-list');
    const summary = $('#executions-summary');
    const detail = $('#executions-detail');
    const cancelBtn = $('#executions-cancel');
    const policyApproveBtn = $('#executions-policy-approve');
    const exportEvidenceBtn = $('#executions-export-evidence');
    const exportAuditBtn = $('#executions-export-audit');
    const retryBtn = $('#executions-retry');
    const rerunBtn = $('#executions-rerun');
    const cloneBtn = $('#executions-clone');
    if (!list || !summary || !detail || !cancelBtn || !policyApproveBtn || !exportEvidenceBtn || !exportAuditBtn || !retryBtn || !rerunBtn || !cloneBtn) return;
    if (!featureFlags.remoteControlPlaneEnabled || !canViewExecutionsUI()) {
      list.textContent = '';
      summary.textContent = featureFlags.remoteControlPlaneEnabled ? 'Execution access is restricted for current role.' : 'Remote control plane is disabled.';
      detail.textContent = 'Execution Center is unavailable.';
      exportEvidenceBtn.classList.add('hidden');
      exportAuditBtn.classList.add('hidden');
      retryBtn.classList.add('hidden');
      rerunBtn.classList.add('hidden');
      cloneBtn.classList.add('hidden');
      cancelBtn.classList.add('hidden');
      policyApproveBtn.classList.add('hidden');
      return;
    }

    syncExecutionFilterOptions(executions);
    const searchValue = String(($('#executions-search') || {}).value || '').trim();
    const statusFilter = String(($('#executions-status-filter') || {}).value || 'all').trim().toLowerCase();
    const templateFilter = String(($('#executions-template-filter') || {}).value || 'all').trim();
    const triggerFilter = String(($('#executions-trigger-filter') || {}).value || 'all').trim().toLowerCase();
    const filtered = executions.filter(execution => executionMatchesFilter(execution, statusFilter, searchValue, templateFilter, triggerFilter));
    const routeInfo = parseRoute(location.hash);
    const routeExecutionID = routeInfo.name === 'executions' ? String(routeInfo.segments[1] || '').trim() : '';
    if (routeExecutionID) selectedExecutionID = routeExecutionID;
    if (!selectedExecutionID && filtered.length) {
      selectedExecutionID = String(filtered[0] && filtered[0].id ? filtered[0].id : '').trim();
    }
    if (selectedExecutionID && !filtered.some(item => String(item && item.id ? item.id : '').trim() === selectedExecutionID)) {
      selectedExecutionID = filtered.length ? String(filtered[0] && filtered[0].id ? filtered[0].id : '').trim() : '';
    }

    summary.textContent = executions.length
      ? 'Total: ' + executions.length + ' · Visible: ' + filtered.length
      : 'No executions recorded yet.';
    list.textContent = '';
    if (!filtered.length) {
      const empty = document.createElement('div');
      empty.className = 'card';
      empty.textContent = 'No executions match the current filter.';
      list.appendChild(empty);
    } else {
      filtered.forEach(execution => {
        const executionID = String(execution && execution.id ? execution.id : '').trim();
        const statusText = String(execution && execution.status ? execution.status : 'unknown').trim();
        const counts = executionCounts(execution);
        const card = document.createElement('button');
        card.type = 'button';
        card.className = 'agent-card execution-card execution-list-card';
        if (executionID && executionID === selectedExecutionID) card.classList.add('active');

        const header = document.createElement('div');
        header.className = 'section-head';
        const title = document.createElement('h4');
        title.textContent = executionID || 'execution';
        const badge = document.createElement('span');
        badge.className = executionStatusBadgeClass(statusText);
        badge.textContent = statusText || 'unknown';
        header.appendChild(title);
        header.appendChild(badge);
        card.appendChild(header);

        const goal = document.createElement('div');
        goal.className = 'execution-goal';
        goal.textContent = String(execution && execution.goal ? execution.goal : '').trim() || '(no goal)';
        card.appendChild(goal);

        const meta = document.createElement('div');
        meta.className = 'instance-meta';
        meta.textContent = 'Tasks: ' + String(counts.taskUnits.length) + ' · Completed: ' + String(counts.completed) + ' · Failed: ' + String(counts.failed) + ' · Updated: ' + formatDateTime(execution && execution.updatedAt);
        card.appendChild(meta);

        const attributionParts = executionAttributionParts(execution);
        if (attributionParts.length) {
          const attribution = document.createElement('div');
          attribution.className = 'instance-meta';
          attribution.textContent = attributionParts.join(' · ');
          card.appendChild(attribution);
        }

        card.onclick = () => {
          selectedExecutionID = executionID;
          const targetHash = '#/executions/' + encodeURIComponent(executionID);
          if (location.hash !== targetHash) {
            location.hash = targetHash;
            return;
          }
          void renderExecutionsView(executionRecordsCache, true);
        };
        list.appendChild(card);
      });
    }

    if (!selectedExecutionID) {
      detail.textContent = 'Select an execution to inspect workers and task results.';
      exportEvidenceBtn.classList.add('hidden');
      exportAuditBtn.classList.add('hidden');
      retryBtn.classList.add('hidden');
      rerunBtn.classList.add('hidden');
      cloneBtn.classList.add('hidden');
      cancelBtn.classList.add('hidden');
      policyApproveBtn.classList.add('hidden');
      return;
    }

    const selectedExecution = executions.find(item => String(item && item.id ? item.id : '').trim() === selectedExecutionID) || {};
    const selectedTerminal = isExecutionTerminalStatus(selectedExecution && selectedExecution.status);
    const selectedHasFailedTasks = executionHasFailedTasks(selectedExecution);
    const selectedPolicy = selectedExecution && selectedExecution.policy && typeof selectedExecution.policy === 'object' ? selectedExecution.policy : {};
    const launchAllowed = canLaunchExecutionsUI();
    const approveAllowed = canApproveExecutionsUI();
    const selectedPolicyAskPending = !selectedTerminal &&
      String(selectedPolicy && selectedPolicy.decision ? selectedPolicy.decision : '').trim() === 'ask' &&
      !String(selectedPolicy && selectedPolicy.approvedAt ? selectedPolicy.approvedAt : '').trim();
    exportEvidenceBtn.classList.toggle('hidden', false);
    exportAuditBtn.classList.toggle('hidden', false);
    retryBtn.classList.toggle('hidden', !(launchAllowed && selectedTerminal && selectedHasFailedTasks));
    rerunBtn.classList.toggle('hidden', !(launchAllowed && selectedTerminal));
    cloneBtn.classList.toggle('hidden', !(launchAllowed && selectedTerminal));
    cancelBtn.classList.toggle('hidden', !(launchAllowed && !selectedTerminal));
    policyApproveBtn.classList.toggle('hidden', !(approveAllowed && selectedPolicyAskPending));

    detail.textContent = 'Loading details…';
    try {
      const payload = await fetchExecutionDetails(selectedExecutionID, !!forceDetail);
      renderExecutionsDetailPanel(detail, payload);
      const execution = payload && payload.execution && typeof payload.execution === 'object' ? payload.execution : {};
      const terminal = isExecutionTerminalStatus(execution && execution.status);
      const hasFailedTasks = executionHasFailedTasks(execution);
      const policy = execution && execution.policy && typeof execution.policy === 'object' ? execution.policy : {};
      const policyAskPending = !terminal &&
        String(policy && policy.decision ? policy.decision : '').trim() === 'ask' &&
        !String(policy && policy.approvedAt ? policy.approvedAt : '').trim();
      exportEvidenceBtn.classList.toggle('hidden', false);
      exportAuditBtn.classList.toggle('hidden', false);
      retryBtn.classList.toggle('hidden', !(launchAllowed && terminal && hasFailedTasks));
      rerunBtn.classList.toggle('hidden', !(launchAllowed && terminal));
      cloneBtn.classList.toggle('hidden', !(launchAllowed && terminal));
      cancelBtn.classList.toggle('hidden', !(launchAllowed && !terminal));
      exportEvidenceBtn.onclick = async () => {
        exportEvidenceBtn.disabled = true;
        try {
          await downloadAPI(
            evidenceDownloadPath(selectedExecutionID),
            String(selectedExecutionID || '').trim() + '-evidence.zip',
          );
        } catch (e) {
          summary.textContent = 'Evidence export failed: ' + e.message;
        } finally {
          exportEvidenceBtn.disabled = false;
        }
      };
      exportAuditBtn.onclick = async () => {
        exportAuditBtn.disabled = true;
        try {
          await downloadAPI(
            auditExportDownloadPath(selectedExecutionID),
            String(selectedExecutionID || '').trim() + '-audit.json',
          );
        } catch (e) {
          summary.textContent = 'Audit export failed: ' + e.message;
        } finally {
          exportAuditBtn.disabled = false;
        }
      };
      cancelBtn.onclick = async () => {
        if (!window.confirm('Cancel execution ' + selectedExecutionID + '?')) return;
        cancelBtn.disabled = true;
        try {
          await cancelExecution(selectedExecutionID, 'webui');
        } finally {
          cancelBtn.disabled = false;
        }
      };
      retryBtn.onclick = async () => {
        retryBtn.disabled = true;
        try {
          const derived = await createDerivedExecution(selectedExecutionID, 'retry');
          if (derived && derived.id) {
            selectedExecutionID = String(derived.id).trim();
            location.hash = '#/executions/' + encodeURIComponent(selectedExecutionID);
          }
        } catch (e) {
          summary.textContent = 'Retry failed: ' + e.message;
        } finally {
          retryBtn.disabled = false;
        }
      };
      rerunBtn.onclick = async () => {
        rerunBtn.disabled = true;
        try {
          const derived = await createDerivedExecution(selectedExecutionID, 'rerun');
          if (derived && derived.id) {
            selectedExecutionID = String(derived.id).trim();
            location.hash = '#/executions/' + encodeURIComponent(selectedExecutionID);
          }
        } catch (e) {
          summary.textContent = 'Rerun failed: ' + e.message;
        } finally {
          rerunBtn.disabled = false;
        }
      };
      cloneBtn.onclick = async () => {
        cloneBtn.disabled = true;
        try {
          const derived = await createDerivedExecution(selectedExecutionID, 'clone');
          if (derived && derived.id) {
            selectedExecutionID = String(derived.id).trim();
            location.hash = '#/executions/' + encodeURIComponent(selectedExecutionID);
          }
        } catch (e) {
          summary.textContent = 'Clone failed: ' + e.message;
        } finally {
          cloneBtn.disabled = false;
        }
      };
      policyApproveBtn.classList.toggle('hidden', !(approveAllowed && policyAskPending));
      policyApproveBtn.onclick = async () => {
        policyApproveBtn.disabled = true;
        try {
          await authorizeExecution(selectedExecutionID, 'webui', true);
        } finally {
          policyApproveBtn.disabled = false;
        }
      };
    } catch (e) {
      detail.textContent = 'Load failed: ' + e.message;
      exportEvidenceBtn.classList.add('hidden');
      exportAuditBtn.classList.add('hidden');
      retryBtn.classList.add('hidden');
      rerunBtn.classList.add('hidden');
      cloneBtn.classList.add('hidden');
      cancelBtn.classList.add('hidden');
      policyApproveBtn.classList.add('hidden');
    }
  }

  function renderDashboardExecutions(executions) {
    const section = $('#dashboard-executions-section');
    const list = $('#execution-list');
    const summary = $('#execution-summary');
    if (!section || !list || !summary) return;
    if (!featureFlags.remoteControlPlaneEnabled || !canViewExecutionsUI()) {
      section.classList.add('hidden');
      return;
    }
    section.classList.remove('hidden');
    const recent = executions.slice(0, 8);
    const active = recent.filter(item => !isExecutionTerminalStatus(item && item.status)).length;
    summary.textContent = recent.length
      ? 'Recent: ' + recent.length + ' · Active: ' + active
      : 'No executions recorded yet.';

    list.textContent = '';
    if (!recent.length) return;

    recent.forEach(execution => {
        const executionID = String(execution && execution.id ? execution.id : '').trim();
        const statusText = String(execution && execution.status ? execution.status : 'unknown').trim();
        const counts = executionCounts(execution);

        const card = document.createElement('div');
        card.className = 'agent-card execution-card';

        const header = document.createElement('div');
        header.className = 'section-head';
        const title = document.createElement('h4');
        title.textContent = executionID || 'execution';
        const badge = document.createElement('span');
        badge.className = executionStatusBadgeClass(statusText);
        badge.textContent = statusText || 'unknown';
        header.appendChild(title);
        header.appendChild(badge);
        card.appendChild(header);

        const goal = document.createElement('div');
        goal.className = 'execution-goal';
        goal.textContent = String(execution && execution.goal ? execution.goal : '').trim() || '(no goal)';
        card.appendChild(goal);

        const meta = document.createElement('div');
        meta.className = 'instance-meta';
        meta.textContent = 'Tasks: ' + String(counts.taskUnits.length) + ' · Completed: ' + String(counts.completed) + ' · Failed: ' + String(counts.failed) + ' · Updated: ' + formatDateTime(execution && execution.updatedAt);
        card.appendChild(meta);

        const attributionParts = executionAttributionParts(execution);
        if (attributionParts.length) {
          const attribution = document.createElement('div');
          attribution.className = 'instance-meta';
          attribution.textContent = attributionParts.join(' · ');
          card.appendChild(attribution);
        }

        const actions = document.createElement('div');
        actions.className = 'btn-row';

        const openBtn = document.createElement('button');
        openBtn.className = 'btn-sm';
        openBtn.textContent = 'Open';
        openBtn.onclick = () => {
          if (!executionID) return;
          location.hash = '#/executions/' + encodeURIComponent(executionID);
        };
        actions.appendChild(openBtn);

        const detailToggle = document.createElement('button');
        detailToggle.className = 'btn-sm btn-secondary';
        detailToggle.textContent = dashboardExpandedExecutionIDs.has(executionID) ? 'Hide Details' : 'View Details';
        actions.appendChild(detailToggle);

        if (canLaunchExecutionsUI() && !isExecutionTerminalStatus(statusText)) {
          const cancelBtn = document.createElement('button');
          cancelBtn.className = 'btn-sm btn-danger';
          cancelBtn.textContent = 'Cancel';
          cancelBtn.onclick = async () => {
            cancelBtn.disabled = true;
            try {
              await cancelExecution(executionID, 'webui');
              summary.textContent = 'Execution cancelled: ' + executionID;
            } catch (e) {
              summary.textContent = 'Cancel failed: ' + e.message;
            } finally {
              cancelBtn.disabled = false;
            }
          };
          actions.appendChild(cancelBtn);
        }
        card.appendChild(actions);

        const details = document.createElement('div');
        details.className = 'execution-details hidden';
        card.appendChild(details);

        detailToggle.onclick = async () => {
          if (!executionID) return;
          const open = details.classList.toggle('hidden');
          if (open) {
            dashboardExpandedExecutionIDs.delete(executionID);
            detailToggle.textContent = 'View Details';
            return;
          }
          dashboardExpandedExecutionIDs.add(executionID);
          detailToggle.textContent = 'Hide Details';
          await loadExecutionDetails(executionID, details, true);
        };

        if (dashboardExpandedExecutionIDs.has(executionID)) {
          details.classList.remove('hidden');
          detailToggle.textContent = 'Hide Details';
          void loadExecutionDetails(executionID, details, true);
        }

        list.appendChild(card);
      });
  }

  async function refreshExecutions() {
    const dashboardList = $('#execution-list');
    const dashboardSummary = $('#execution-summary');
    if (dashboardList && !dashboardList.childElementCount) dashboardList.textContent = 'Loading…';
    if (!featureFlags.remoteControlPlaneEnabled || !canViewExecutionsUI()) {
      renderDashboardExecutions([]);
      await renderExecutionsView([], false);
      return;
    }
    try {
      const payload = await api('GET', '/api/v1/orchestrator/executions');
      executionRecordsCache = sortExecutionsByUpdatedAt(payload && Array.isArray(payload.executions) ? payload.executions : []);
      renderDashboardExecutions(executionRecordsCache);
      if (currentRouteName() === 'executions') {
        await renderExecutionsView(executionRecordsCache, false);
      }
    } catch (e) {
      if (dashboardList) dashboardList.textContent = 'Error: ' + e.message;
      if (dashboardSummary) dashboardSummary.textContent = 'Execution history unavailable.';
      if (currentRouteName() === 'executions') {
        const list = $('#executions-list');
        const summary = $('#executions-summary');
        const detail = $('#executions-detail');
        if (list) list.textContent = 'Error: ' + e.message;
        if (summary) summary.textContent = 'Execution history unavailable.';
        if (detail) detail.textContent = 'Execution details unavailable.';
      }
    }
  }

  function workerMatchesFilter(worker, filterValue, query) {
    const normalizedQuery = String(query || '').trim().toLowerCase();
    const haystack = [
      worker && worker.id,
      worker && worker.hostId,
      worker && worker.hostName,
      worker && worker.agentId,
      worker && worker.source,
      worker && worker.executionId,
    ].map(value => String(value || '').trim().toLowerCase()).join(' ');
    if (normalizedQuery && !haystack.includes(normalizedQuery)) {
      return false;
    }
    const state = String(worker && worker.state ? worker.state : '').trim().toLowerCase();
    switch (String(filterValue || 'all').trim().toLowerCase()) {
      case 'active':
        return state === 'busy' || state === 'provisioning' || state === 'reclaiming' || state === 'ready';
      case 'stale':
        return !!(worker && worker.stale);
      case 'available':
      case 'managed':
      case 'stopped':
      case 'error':
      case 'reclaimed':
        return state === String(filterValue).trim().toLowerCase();
      default:
        return true;
    }
  }

  function buildWorkerSummaryPayload(workers, queueSummary) {
    const items = Array.isArray(workers) ? workers : [];
    const queue = queueSummary && typeof queueSummary === 'object' ? queueSummary : {};
    return {
      total: items.length,
      active: items.filter(item => ['busy', 'provisioning', 'reclaiming', 'ready'].includes(String(item && item.state || '').trim().toLowerCase())).length,
      busy: items.filter(item => String(item && item.state || '').trim().toLowerCase() === 'busy').length,
      error: items.filter(item => String(item && item.state || '').trim().toLowerCase() === 'error').length,
      local: items.filter(item => String(item && item.hostId || '').trim() === 'local').length,
      remote: items.filter(item => String(item && item.hostId || '').trim() !== 'local').length,
      stale: items.filter(item => !!(item && item.stale)).length,
      queueSummary: queue,
    };
  }

  function renderWorkersView(workers, summaryPayload, warnings) {
    const list = $('#workers-list');
    const summary = $('#workers-summary');
    if (!list || !summary) return;
    if (!featureFlags.remoteControlPlaneEnabled) {
      list.textContent = '';
      summary.textContent = 'Remote control plane is disabled.';
      setMsg('#workers-msg', '', 'info');
      return;
    }

    const searchValue = String(($('#workers-search') || {}).value || '').trim();
    const stateFilter = String(($('#workers-state-filter') || {}).value || 'all').trim().toLowerCase();
    const filtered = (Array.isArray(workers) ? workers : []).filter(worker => workerMatchesFilter(worker, stateFilter, searchValue));
    const summarySource = summaryPayload && typeof summaryPayload === 'object' ? summaryPayload : {};
    const total = Number(summarySource.total || 0) || (Array.isArray(workers) ? workers.length : 0);
    const busy = Number(summarySource.busy || 0) || 0;
    const active = Number(summarySource.active || 0) || 0;
    const local = Number(summarySource.local || 0) || 0;
    const remote = Number(summarySource.remote || 0) || 0;
    const stale = Number(summarySource.stale || 0) || 0;
    const queueSummary = summarySource.queueSummary && typeof summarySource.queueSummary === 'object' ? summarySource.queueSummary : {};
    const activeExecutions = Number(queueSummary.activeExecutions || 0) || 0;
    const queuedTasks = Number(queueSummary.queuedTasks || 0) || 0;
    summary.textContent = total
      ? 'Total: ' + total + ' · Visible: ' + filtered.length + ' · Active: ' + active + ' · Busy: ' + busy + ' · Stale: ' + stale + ' · Active Executions: ' + activeExecutions + ' · Queued Tasks: ' + queuedTasks + ' · Local: ' + local + ' · Remote: ' + remote
      : 'No workers discovered yet.';
    if (Array.isArray(warnings) && warnings.length) {
      setMsg('#workers-msg', warnings.join(' | '), 'error');
    } else {
      setMsg('#workers-msg', '', 'info');
    }

    list.textContent = '';
    if (!filtered.length) {
      const empty = document.createElement('div');
      empty.className = 'card';
      empty.textContent = 'No workers match the current filter.';
      list.appendChild(empty);
      return;
    }

    filtered.forEach(worker => {
      const card = document.createElement('div');
      card.className = 'agent-card worker-card';

      const header = document.createElement('div');
      header.className = 'section-head';
      const titleWrap = document.createElement('div');
      const title = document.createElement('h4');
      title.textContent = String(worker && worker.hostName ? worker.hostName : worker.hostId || 'unknown') + ' / ' + String(worker && worker.agentId ? worker.agentId : 'unknown');
      const sub = document.createElement('div');
      sub.className = 'instance-meta';
      sub.textContent = 'source: ' + String(worker && worker.source ? worker.source : 'unknown') + ' · id: ' + String(worker && worker.id ? worker.id : 'n/a');
      titleWrap.appendChild(title);
      titleWrap.appendChild(sub);

      const badgeWrap = document.createElement('div');
      badgeWrap.className = 'worker-badge-row';
      const stateBadge = document.createElement('span');
      stateBadge.className = workerStateBadgeClass(worker && worker.state);
      stateBadge.textContent = String(worker && worker.state ? worker.state : 'unknown');
      badgeWrap.appendChild(stateBadge);
      if (worker && worker.stale) {
        const staleBadge = document.createElement('span');
        staleBadge.className = 'badge badge-warn';
        staleBadge.textContent = 'stale';
        badgeWrap.appendChild(staleBadge);
      }
      header.appendChild(titleWrap);
      header.appendChild(badgeWrap);
      card.appendChild(header);

      const meta = document.createElement('div');
      meta.className = 'worker-meta-grid';
      const metaLines = [];
      if (worker && worker.executionId) metaLines.push('execution: ' + String(worker.executionId));
      if (worker && worker.runtimeState) metaLines.push('runtime: ' + String(worker.runtimeState));
      if (worker && worker.runtimeMode) metaLines.push('runtime mode: ' + String(worker.runtimeMode));
      if (worker && worker.health) metaLines.push('health: ' + String(worker.health));
      if (worker && worker.taskCount) metaLines.push('tasks: ' + String(worker.taskCount));
      if (worker && worker.queuePosition) metaLines.push('queue position: ' + String(worker.queuePosition));
      if (worker && worker.lastSyncStatus) metaLines.push('sync: ' + String(worker.lastSyncStatus));
      if (worker && worker.driftState) metaLines.push('drift: ' + String(worker.driftState));
      if (worker && worker.leaseState) metaLines.push('lease state: ' + String(worker.leaseState));
      if (worker && worker.staleReason) metaLines.push('stale reason: ' + String(worker.staleReason));
      if (worker && worker.leaseAgeSec) metaLines.push('lease age: ' + formatAgeSeconds(worker.leaseAgeSec));
      if (worker && worker.heartbeatAgeSec) metaLines.push('heartbeat age: ' + formatAgeSeconds(worker.heartbeatAgeSec));
      if (worker && worker.lastHeartbeatAt) metaLines.push('last heartbeat: ' + formatDateTime(worker.lastHeartbeatAt));
      if (worker && worker.heartbeatAt) metaLines.push('heartbeat: ' + formatDateTime(worker.heartbeatAt));
      if (worker && worker.updatedAt) metaLines.push('updated: ' + formatDateTime(worker.updatedAt));
      if (!metaLines.length) metaLines.push('host: ' + String(worker && worker.hostId ? worker.hostId : 'unknown'));
      metaLines.forEach(line => {
        const row = document.createElement('div');
        row.className = 'execution-detail-line';
        row.textContent = line;
        meta.appendChild(row);
      });
      card.appendChild(meta);

      if (worker && worker.lastError) {
        const error = document.createElement('pre');
        error.className = 'code-block execution-result-output';
        error.textContent = String(worker.lastError);
        card.appendChild(error);
      }

      list.appendChild(card);
    });
  }

  async function refreshWorkers() {
    const list = $('#workers-list');
    if (list && !list.childElementCount) list.textContent = 'Loading…';
    if (!featureFlags.remoteControlPlaneEnabled) {
      renderWorkersView([], buildWorkerSummaryPayload([], null), []);
      return;
    }
    try {
      const [payload, queuePayload] = await Promise.all([
        api('GET', '/api/v1/orchestrator/workers'),
        api('GET', '/api/v1/orchestrator/workers/queue'),
      ]);
      workerInventoryCache = Array.isArray(payload && payload.workers) ? payload.workers : [];
      workerQueueSummaryCache = queuePayload && queuePayload.summary ? queuePayload.summary : null;
      renderWorkersView(workerInventoryCache, {
        ...(payload && payload.summary ? payload.summary : {}),
        queueSummary: workerQueueSummaryCache,
      }, payload && payload.warnings);
    } catch (e) {
      if (list) list.textContent = 'Error: ' + e.message;
      const summary = $('#workers-summary');
      if (summary) summary.textContent = 'Worker inventory unavailable.';
      setMsg('#workers-msg', 'Load failed: ' + e.message, 'error');
    }
  }

  async function reclaimIdleWorkers() {
    const button = $('#workers-reclaim-idle');
    if (button) button.disabled = true;
    try {
      const payload = await api('POST', '/api/v1/orchestrator/workers/reclaim', {});
      const reclaim = payload && payload.reclaim ? payload.reclaim : {};
      await refreshWorkers();
      setMsg(
        '#workers-msg',
        'Idle reclaim finished: reclaimed=' + String(Number(reclaim.reclaimed || 0) || 0) +
        ', skipped=' + String(Number(reclaim.skipped || 0) || 0) +
        ', failed=' + String(Number(reclaim.failed || 0) || 0),
        'info',
      );
    } catch (e) {
      setMsg('#workers-msg', 'Reclaim failed: ' + e.message, 'error');
    } finally {
      if (button) button.disabled = false;
    }
  }

  async function reclaimStaleWorkers() {
    const button = $('#workers-reclaim-stale');
    if (button) button.disabled = true;
    try {
      const payload = await api('POST', '/api/v1/orchestrator/workers/reclaim-stale', {});
      const reclaim = payload && payload.reclaim ? payload.reclaim : {};
      await refreshWorkers();
      setMsg(
        '#workers-msg',
        'Stale reclaim finished: reclaimed=' + String(Number(reclaim.reclaimed || 0) || 0) +
        ', skipped=' + String(Number(reclaim.skipped || 0) || 0) +
        ', failed=' + String(Number(reclaim.failed || 0) || 0),
        'info',
      );
    } catch (e) {
      setMsg('#workers-msg', 'Stale reclaim failed: ' + e.message, 'error');
    } finally {
      if (button) button.disabled = false;
    }
  }

  async function initWorkers() {
    resetAddMode();
    showView('workers');
    $('#nav').classList.remove('hidden');

    const refreshBtn = $('#workers-refresh');
    const reclaimBtn = $('#workers-reclaim-idle');
    const reclaimStaleBtn = $('#workers-reclaim-stale');
    const searchInput = $('#workers-search');
    const stateFilter = $('#workers-state-filter');
    if (refreshBtn) refreshBtn.onclick = refreshWorkers;
    if (reclaimBtn) reclaimBtn.onclick = reclaimIdleWorkers;
    if (reclaimStaleBtn) reclaimStaleBtn.onclick = reclaimStaleWorkers;
    if (searchInput) searchInput.oninput = () => renderWorkersView(workerInventoryCache, buildWorkerSummaryPayload(workerInventoryCache, workerQueueSummaryCache), []);
    if (stateFilter) stateFilter.onchange = () => renderWorkersView(workerInventoryCache, buildWorkerSummaryPayload(workerInventoryCache, workerQueueSummaryCache), []);

    await refreshWorkers();
    startWorkersPolling();
  }

  function renderMemorySnapshot(payload) {
    memoryListCache = payload && typeof payload === 'object' ? payload : {};
    const list = $('#memory-entry-list');
    const summary = $('#memory-summary');
    if (!list || !summary) return;

    const entries = Array.isArray(memoryListCache.entries) ? memoryListCache.entries : [];
    const attachments = Array.isArray(memoryListCache.attachments) ? memoryListCache.attachments : [];
    const grants = Array.isArray(memoryListCache.grants) ? memoryListCache.grants : [];
    const audit = Array.isArray(memoryListCache.audit) ? memoryListCache.audit : [];
    const subject = String(memoryListCache.subject || ($('#memory-subject') && $('#memory-subject').value) || '').trim();
    summary.textContent =
      'subject=' + (subject || 'all') +
      ' · entries=' + entries.length +
      ' · attachments=' + attachments.length +
      ' · grants=' + grants.length +
      ' · audit=' + audit.length;

    list.textContent = '';
    const sections = [
      {
        title: 'Entries',
        empty: 'No memory packages found.',
        items: entries.map(entry => ({
          title: String(entry && entry.id ? entry.id : 'unknown'),
          meta: ['type: ' + String(entry && entry.type ? entry.type : 'unknown')],
        })),
      },
      {
        title: 'Attachments',
        empty: 'No attachments for this subject.',
        items: attachments.map(attachment => ({
          title: String(attachment && attachment.memory_id ? attachment.memory_id : 'unknown'),
          meta: ['agent: ' + String(attachment && attachment.agent_id ? attachment.agent_id : 'unknown')],
        })),
      },
      {
        title: 'Grants',
        empty: 'No grants for this subject.',
        items: grants.map(grant => ({
          title: String(grant && grant.scope ? grant.scope : 'unknown'),
          meta: ['subject: ' + String(grant && grant.subject ? grant.subject : 'unknown')],
        })),
      },
    ];

    sections.forEach(section => {
      const wrap = document.createElement('div');
      wrap.className = 'memory-section';
      const heading = document.createElement('h4');
      heading.textContent = section.title;
      wrap.appendChild(heading);
      if (!section.items.length) {
        const empty = document.createElement('div');
        empty.className = 'text-dim';
        empty.textContent = section.empty;
        wrap.appendChild(empty);
      } else {
        section.items.forEach(item => {
          const card = document.createElement('div');
          card.className = 'agent-card memory-card';
          const title = document.createElement('strong');
          title.textContent = item.title;
          card.appendChild(title);
          item.meta.forEach(line => {
            const row = document.createElement('div');
            row.className = 'execution-detail-line';
            row.textContent = line;
            card.appendChild(row);
          });
          wrap.appendChild(card);
        });
      }
      list.appendChild(wrap);
    });
  }

  function renderMemorySearchResultsView(results) {
    memorySearchResultsCache = Array.isArray(results) ? results : [];
    const list = $('#memory-search-results');
    if (!list) return;
    list.textContent = '';
    if (!memorySearchResultsCache.length) {
      const empty = document.createElement('div');
      empty.className = 'text-dim';
      empty.textContent = 'No search results yet.';
      list.appendChild(empty);
      return;
    }
    memorySearchResultsCache.forEach(result => {
      const card = document.createElement('div');
      card.className = 'agent-card memory-card';
      const title = document.createElement('strong');
      title.textContent = String(result && result.id ? result.id : 'unknown');
      card.appendChild(title);
      const meta = document.createElement('div');
      meta.className = 'execution-detail-line';
      meta.textContent = 'scope=' + String(result && result.scope ? result.scope : 'unknown') + ' · score=' + Number(result && result.score || 0).toFixed(2);
      card.appendChild(meta);
      const snippet = document.createElement('div');
      snippet.className = 'text-dim';
      snippet.textContent = String(result && result.snippet ? result.snippet : '');
      card.appendChild(snippet);
      list.appendChild(card);
    });
  }

  async function refreshMemoryView() {
    const list = $('#memory-entry-list');
    if (list && !list.childElementCount) list.textContent = 'Loading…';
    const subject = String(($('#memory-subject') || {}).value || '').trim();
    const path = subject ? '/api/v1/memory?subject=' + encodeURIComponent(subject) : '/api/v1/memory';
    try {
      const payload = await api('GET', path);
      renderMemorySnapshot(payload);
      setMsg('#memory-msg', '', 'info');
    } catch (e) {
      if (list) list.textContent = 'Error: ' + e.message;
      const summary = $('#memory-summary');
      if (summary) summary.textContent = 'Memory unavailable.';
      setMsg('#memory-msg', 'Load failed: ' + e.message, 'error');
    }
  }

  async function runMemorySearch() {
    const subject = String(($('#memory-subject') || {}).value || '').trim();
    const query = String(($('#memory-search-query') || {}).value || '').trim();
    const limit = parseInt(String(($('#memory-search-limit') || {}).value || '10').trim(), 10) || 10;
    const minScore = parseFloat(String(($('#memory-search-min-score') || {}).value || '0').trim()) || 0;
    if (!query) {
      setMsg('#memory-msg', 'Search query is required.', 'error');
      return;
    }
    try {
      const payload = await api('POST', '/api/v1/memory/search', {
        subject,
        query,
        maxResults: Math.max(1, limit),
        minScore: Math.max(0, minScore),
      });
      renderMemorySearchResultsView(payload && Array.isArray(payload.results) ? payload.results : []);
      setMsg('#memory-msg', 'Search completed.', 'success');
    } catch (e) {
      renderMemorySearchResultsView([]);
      setMsg('#memory-msg', 'Search failed: ' + e.message, 'error');
    }
  }

  async function runMemoryInstanceAction(action) {
    const instanceId = String(($('#memory-instance-id') || {}).value || '').trim();
    const scope = String(($('#memory-instance-scope') || {}).value || '').trim();
    const reason = String(($('#memory-distill-reason') || {}).value || '').trim();
    const dryRun = !!(($('#memory-distill-dry-run') || {}).checked);
    if (!instanceId) {
      setMsg('#memory-action-msg', 'Instance ID is required.', 'error');
      return;
    }
    if ((action === 'attach' || action === 'detach') && !scope) {
      setMsg('#memory-action-msg', 'Scope is required for attach/detach.', 'error');
      return;
    }
    const payload: any = { instanceId };
    let path = '';
    if (action === 'attach') {
      path = '/api/v1/memory/instance/attach';
      payload.scope = scope;
    } else if (action === 'detach') {
      path = '/api/v1/memory/instance/detach';
      payload.scope = scope;
    } else {
      path = '/api/v1/memory/instance/distill';
      if (scope) payload.scope = scope;
      if (reason) payload.reason = reason;
      if (dryRun) payload.dryRun = true;
    }
    try {
      const result = await api('POST', path, payload);
      if (action === 'distill') {
        const run = result && result.result && typeof result.result === 'object' ? result.result : {};
        setMsg(
          '#memory-action-msg',
          'distill ' + String(run.runId || 'unknown') + ' · ' + String(run.instanceId || instanceId) + ' · ' + String(run.status || 'unknown'),
          'success',
        );
      } else {
        setMsg('#memory-action-msg', String(result && result.status ? result.status : action), 'success');
      }
      await refreshMemoryView();
    } catch (e) {
      setMsg('#memory-action-msg', 'Memory action failed: ' + e.message, 'error');
    }
  }

  async function initMemory() {
    resetAddMode();
    showView('memory');
    $('#nav').classList.remove('hidden');

    const refreshBtn = $('#memory-refresh');
    const searchBtn = $('#memory-search-run');
    const attachBtn = $('#memory-attach');
    const detachBtn = $('#memory-detach');
    const distillBtn = $('#memory-distill');
    const subjectInput = $('#memory-subject');
    const queryInput = $('#memory-search-query');
    const readOnly = !canViewExecutionsUI();
    const canMutate = canLaunchExecutionsUI();

    if (refreshBtn) refreshBtn.onclick = refreshMemoryView;
    if (searchBtn) searchBtn.onclick = runMemorySearch;
    if (attachBtn) {
      attachBtn.disabled = !canMutate;
      attachBtn.onclick = () => runMemoryInstanceAction('attach');
    }
    if (detachBtn) {
      detachBtn.disabled = !canMutate;
      detachBtn.onclick = () => runMemoryInstanceAction('detach');
    }
    if (distillBtn) {
      distillBtn.disabled = !canMutate;
      distillBtn.onclick = () => runMemoryInstanceAction('distill');
    }
    if (subjectInput) subjectInput.onchange = refreshMemoryView;
    if (queryInput) queryInput.onkeydown = (event) => {
      if (event.key === 'Enter') runMemorySearch();
    };
    if (readOnly) {
      setMsg('#memory-msg', 'Current role has read-only memory access.', 'info');
    }

    renderMemorySearchResultsView(memorySearchResultsCache);
    await refreshMemoryView();
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
    loadLogAgentOptions().catch(() => {});
  }

  async function loadLogAgentOptions() {
    const agentSelect = $('#log-agent');
    if (!agentSelect) return;
    const previous = String(agentSelect.value || '').trim();
    agentSelect.textContent = '';
    const seen = new Set();

    function appendOption(value, label) {
      const id = String(value || '').trim();
      if (!id) return;
      if (seen.has(id)) return;
      seen.add(id);
      const opt = document.createElement('option');
      opt.value = id;
      opt.textContent = String(label || id);
      agentSelect.appendChild(opt);
    }

    try {
      const agents = normalizeAgentCatalog(await api('GET', '/api/v1/agents'));
      agents.forEach(agent => {
        const id = String(agent.id || '').trim();
        if (!id) return;
        const runtimeState = String(agent.runtimeState || agent.runtime_state || '').trim();
        const installState = String(agent.installState || agent.install_state || '').trim();
        const suffix = runtimeState || installState;
        appendOption(id, suffix ? (id + ' (' + suffix + ')') : id);
      });
    } catch (_) {}

    try {
      const instances = normalizeInstances(await api('GET', '/api/v1/instances'));
      instances.forEach(instance => {
        const runtimeAgentID = String(instance.agent_id || instance.agentID || instance.type || '').trim();
        if (!runtimeAgentID) return;
        const instanceID = String(instance.id || instance.ID || '').trim();
        const runtimeState = String(instance.runtime_state || instance.runtimeState || '').trim();
        const labelParts = [];
        if (instanceID && instanceID !== runtimeAgentID) labelParts.push(instanceID);
        if (runtimeState) labelParts.push(runtimeState);
        const suffix = labelParts.length ? (' [' + labelParts.join(', ') + ']') : '';
        appendOption(runtimeAgentID, runtimeAgentID + suffix);
      });
    } catch (_) {}

    if (previous && seen.has(previous)) {
      agentSelect.value = previous;
    } else if (agentSelect.options.length) {
      agentSelect.selectedIndex = 0;
    }

    if (!agentSelect.options.length) {
      const option = document.createElement('option');
      option.value = '';
      option.textContent = 'No agents available';
      agentSelect.appendChild(option);
      logStatusBase = 'No local agents available. Start an agent first.';
    } else {
      logStatusBase = 'Select an agent and click Connect.';
    }
    refreshLogStatus(getVisibleLogEntries().length);
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

  async function fetchSSHConfigHostAliases() {
    const payload = await api('GET', '/api/v1/remote/ssh-config-hosts');
    const aliases = payload && Array.isArray(payload.hosts) ? payload.hosts : [];
    const normalized = [];
    const seen = new Set();
    aliases.forEach(item => {
      const alias = String(item || '').trim();
      if (!alias) return;
      if (seen.has(alias)) return;
      seen.add(alias);
      normalized.push(alias);
    });
    normalized.sort((a, b) => a.localeCompare(b));
    sshConfigHostAliasesCache = normalized;
    return normalized;
  }

  function renderSSHConfigHostAliasOptions(aliases, loadErr) {
    const options = $('#server-ssh-config-host-options');
    const select = $('#server-ssh-config-host-select');
    const hint = $('#server-ssh-config-host-hint');
    const list = Array.isArray(aliases) ? aliases : [];
    if (options) {
      options.textContent = '';
      list.forEach(alias => {
        const value = String(alias || '').trim();
        if (!value) return;
        const option = document.createElement('option');
        option.value = value;
        options.appendChild(option);
      });
    }
    if (select) {
      select.textContent = '';
      const defaultOption = document.createElement('option');
      defaultOption.value = '';
      defaultOption.textContent = list.length ? 'Select detected SSH alias…' : 'No detected SSH aliases';
      select.appendChild(defaultOption);
      list.forEach(alias => {
        const value = String(alias || '').trim();
        if (!value) return;
        const option = document.createElement('option');
        option.value = value;
        option.textContent = value;
        select.appendChild(option);
      });
      select.value = '';
    }
    if (!hint) return;
    if (loadErr) {
      hint.textContent = 'Failed to load local SSH config aliases: ' + String(loadErr);
      syncServerAuthModeInputs();
      return;
    }
    if (!list.length) {
      hint.textContent = 'No aliases detected from local SSH config. You can still type one manually.';
      syncServerAuthModeInputs();
      return;
    }
    hint.textContent = 'Detected ' + String(list.length) + ' alias(es) from local SSH config. Select from dropdown or type manually.';
    syncServerAuthModeInputs();
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
    const sshConfigSelect = $('#server-ssh-config-host-select');
    const sshConfigHint = $('#server-ssh-config-host-hint');
    if (!authMode || !hostInput || !keyInput || !sshConfigInput) return;
    const mode = String(authMode.value || '').trim().toLowerCase();
    const privateKey = mode === 'private_key';
    keyInput.disabled = !privateKey;
    hostInput.disabled = false;
    sshConfigInput.disabled = privateKey;
    if (sshConfigSelect) {
      const hasAliases = sshConfigSelect.options.length > 1;
      sshConfigSelect.disabled = privateKey || !hasAliases;
      sshConfigSelect.classList.toggle('hidden', privateKey || !hasAliases);
    }
    if (sshConfigHint) {
      sshConfigHint.classList.toggle('hidden', privateKey);
    }
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
      '#server-labels': '',
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
      '#server-labels': Array.isArray(host.labels) ? host.labels.join(', ') : '',
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

  function updateTriggerEditorUI() {
    const saveBtn = $('#trigger-save');
    const cancelBtn = $('#trigger-cancel-edit');
    const stateEl = $('#trigger-editor-state');
    const editing = String(triggerEditingTriggerID || '').trim();
    if (saveBtn) {
      saveBtn.textContent = editing ? 'Update Trigger' : 'Save Trigger';
    }
    if (cancelBtn) {
      cancelBtn.classList.toggle('hidden', !editing);
    }
    if (stateEl) {
      stateEl.textContent = editing ? ('Editing trigger: ' + editing) : '';
    }
  }

  function clearTriggerFormValues() {
    const defaults = {
      '#trigger-name': '',
      '#trigger-type': 'webhook',
      '#trigger-provider': '',
      '#trigger-host-ids': '',
      '#trigger-host-labels': '',
      '#trigger-max-concurrency': '',
      '#trigger-inputs': '',
      '#trigger-webhook-secret': '',
      '#trigger-github-command': '',
      '#trigger-github-label': '',
      '#trigger-github-repository': '',
      '#trigger-cron': '',
      '#trigger-timezone': 'UTC',
    };
    Object.keys(defaults).forEach(selector => {
      const el = $(selector);
      if (!el) return;
      el.value = defaults[selector];
    });
    const templateSelect = $('#trigger-template-id');
    if (templateSelect && templateSelect.options && templateSelect.options.length > 0) {
      templateSelect.selectedIndex = 0;
    }
    const policyApprove = $('#trigger-policy-approve');
    if (policyApprove) policyApprove.checked = false;
  }

  function resetTriggerEditor(clearForm) {
    triggerEditingTriggerID = '';
    updateTriggerEditorUI();
    if (clearForm) {
      clearTriggerFormValues();
    }
  }

  function renderTriggerInputsText(inputs) {
    const source = inputs && typeof inputs === 'object' ? inputs : {};
    return Object.keys(source)
      .sort((a, b) => a.localeCompare(b))
      .map((key) => String(key) + '=' + String(source[key] || ''))
      .join('\n');
  }

  function parseTriggerInputsText(raw) {
    const out = {};
    String(raw || '')
      .split('\n')
      .map(line => String(line || '').trim())
      .filter(Boolean)
      .forEach(line => {
        const idx = line.indexOf('=');
        if (idx <= 0) return;
        const key = line.slice(0, idx).trim();
        const value = line.slice(idx + 1).trim();
        if (!key) return;
        out[key] = value;
      });
    return out;
  }

  function syncTriggerTemplateOptions(templates) {
    const select = $('#trigger-template-id');
    if (!select) return;
    const current = String(select.value || '').trim();
    select.textContent = '';
    (Array.isArray(templates) ? templates : []).forEach(template => {
      const opt = document.createElement('option');
      opt.value = String(template && template.id ? template.id : '').trim();
      opt.textContent = String(template && template.name ? template.name : opt.value).trim() || opt.value;
      select.appendChild(opt);
    });
    if (current && Array.from(select.options).some((opt) => opt.value === current)) {
      select.value = current;
    } else if (select.options.length > 0) {
      select.selectedIndex = 0;
    }
  }

  function beginTriggerEdit(triggerID) {
    const key = String(triggerID || '').trim();
    if (!key) return;
    const trigger = executionTriggersCache.find(item => String(item && item.id ? item.id : '') === key);
    if (!trigger) return;
    const config = trigger && trigger.config && typeof trigger.config === 'object' ? trigger.config : {};
    triggerEditingTriggerID = key;
    const map = {
      '#trigger-name': trigger.name || '',
      '#trigger-type': trigger.type || 'webhook',
      '#trigger-template-id': trigger.templateId || '',
      '#trigger-provider': config.provider || '',
      '#trigger-host-ids': Array.isArray(config.hostIds) ? config.hostIds.join(', ') : '',
      '#trigger-host-labels': Array.isArray(config.hostLabels) ? config.hostLabels.join(', ') : '',
      '#trigger-max-concurrency': config.maxConcurrency ? String(config.maxConcurrency) : '',
      '#trigger-inputs': renderTriggerInputsText(config.inputs || {}),
      '#trigger-webhook-secret': '',
      '#trigger-github-command': config.githubCommand || '',
      '#trigger-github-label': config.githubLabel || '',
      '#trigger-github-repository': config.githubRepository || '',
      '#trigger-cron': config.cron || '',
      '#trigger-timezone': config.timezone || 'UTC',
    };
    Object.keys(map).forEach(selector => {
      const el = $(selector);
      if (!el) return;
      el.value = map[selector];
    });
    const policyApprove = $('#trigger-policy-approve');
    if (policyApprove) policyApprove.checked = !!config.policyApprove;
    updateTriggerEditorUI();
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

  function getServerManageSyncMode() {
    const input = $('#server-manage-sync-mode');
    const value = input && input.value ? input.value.trim() : '';
    if (value === 'always_push' || value === 'manual') return value;
    return 'pull_validate_push';
  }

  function getServerManageRollbackCommit() {
    const input = $('#server-manage-rollback-commit');
    return input && input.value ? input.value.trim() : '';
  }

  function getServerManageCodeAgentBackend() {
    const input = $('#server-manage-codeagent-backend');
    const value = input && input.value ? input.value.trim().toLowerCase() : '';
    if (value === 'opencode') return 'opencode';
    return 'codex';
  }

  function getServerManageCodeAgentWorkspaceRoot() {
    const input = $('#server-manage-codeagent-workspace-root');
    const value = input && input.value ? input.value.trim() : '';
    return value || '/workspace';
  }

  function getServerManageCodeAgentCapability() {
    const input = $('#server-manage-codeagent-capability');
    const value = input && input.value ? input.value.trim().toLowerCase() : '';
    if (!value) return 'run_shell';
    return value;
  }

  function getServerManageCodeAgentWriteMode() {
    const input = $('#server-manage-codeagent-write-mode');
    const value = input && input.value ? input.value.trim().toLowerCase() : '';
    if (value === 'append') return 'append';
    return 'overwrite';
  }

  function getServerManageCodeAgentCommand() {
    const input = $('#server-manage-codeagent-command');
    return input && input.value ? input.value.trim() : '';
  }

  function getServerManageCodeAgentPath() {
    const input = $('#server-manage-codeagent-path');
    return input && input.value ? input.value.trim() : '';
  }

  function getServerManageCodeAgentContent() {
    const input = $('#server-manage-codeagent-content');
    return input && typeof input.value === 'string' ? input.value : '';
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

    function appendBoolKV(label, value) {
      if (typeof value !== 'boolean') return;
      appendKV(label, value ? 'true' : 'false');
    }

    function appendNumberKV(label, value) {
      if (!Number.isFinite(Number(value))) return;
      appendKV(label, String(value));
    }

    appendKV('id', current.id || current.ID || '');
    appendKV('runtime', runtimeState || 'unknown');
    appendKV('health', health || 'unknown');
    if (installed !== null) appendKV('install status', installed ? 'installed' : 'not installed');
    if (repaired !== null) appendKV('repair status', repaired ? 'repaired' : 'not repaired');
    if (gatewayHealthy !== null) appendKV('gateway', gatewayHealthy ? 'healthy' : 'unhealthy');
    appendKV('sync mode', current.syncMode || current.sync_mode || '');
    appendKV('drift state', current.driftState || current.drift_state || '');
    appendKV('sync status', current.lastSyncStatus || current.last_sync_status || current.status || '');
    appendKV('sync at', current.lastSyncAt || current.last_sync_at || '');
    appendKV('diagnose at', current.lastDiagnoseAt || current.last_diagnose_at || '');
    appendKV('diagnose result', current.lastDiagnoseResult || current.last_diagnose_result || current.result || '');
    appendKV('reconcile at', current.lastReconcileAt || current.last_reconcile_at || '');
    appendKV('rollback at', current.lastRollbackAt || current.last_rollback_at || '');
    appendKV('from commit', current.fromCommit || current.from_commit || '');
    appendKV('new commit', current.newCommit || current.new_commit || '');
    appendKV('local commit', current.lastLocalCommit || current.last_local_commit || '');
    appendKV('common commit', current.lastCommonCommit || current.last_common_commit || '');
    appendKV('remote hash', current.lastRemoteHash || current.last_remote_hash || '');
    appendKV('backend', current.backend || '');
    appendKV('policy decision', current.policy_decision || current.policyDecision || '');
    appendKV('policy reason', current.policy_reason || current.policyReason || '');
    appendNumberKV('exit code', current.exit_code != null ? current.exit_code : current.exitCode);
    appendNumberKV('duration ms', current.duration_ms != null ? current.duration_ms : current.durationMs);
    appendNumberKV('cost estimate usd', current.cost_estimate_usd != null ? current.cost_estimate_usd : current.costEstimateUSD);
    appendBoolKV('reconciled', current.reconciled);
    appendBoolKV('rolled back', current.rolledBack != null ? current.rolledBack : current.rolled_back);
    appendBoolKV('restored snapshot', current.restoredSnapshot != null ? current.restoredSnapshot : current.restored_snapshot);
    appendBoolKV('healthy', current.healthy);
    appendBoolKV('configured', current.configured);
    appendBoolKV('ok', current.ok);
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

  function updateServerManageStreamStatus(text, type) {
    const el = $('#server-manage-stream-status');
    if (!el) return;
    el.textContent = text || '';
    el.classList.remove('msg-error', 'msg-success', 'msg-info');
    if (type) el.classList.add('msg-' + type);
  }

  function renderServerManageDiagnosis(text, state) {
    const out = $('#server-manage-diagnosis');
    if (!out) return;
    out.textContent = '';
    const body = String(text || '').trim();
    if (!body) return;

    let pillState = 'manage-pill-warn';
    let title = 'BaseAgent Diagnosis';
    const normalized = String(state || '').trim().toLowerCase();
    if (normalized === 'ok') {
      pillState = 'manage-pill-ok';
      title = 'BaseAgent Diagnosis';
    } else if (normalized === 'error') {
      pillState = 'manage-pill-bad';
      title = 'BaseAgent Diagnosis Failed';
    } else if (normalized === 'running') {
      pillState = 'manage-pill-warn';
      title = 'BaseAgent Analyzing';
    }

    const card = document.createElement('div');
    card.className = 'manage-card';

    const header = document.createElement('div');
    header.className = 'manage-card-header';
    const h = document.createElement('div');
    h.className = 'manage-card-title';
    h.textContent = title;
    const pill = document.createElement('span');
    pill.className = 'manage-pill ' + pillState;
    pill.textContent = normalized || 'info';
    header.appendChild(h);
    header.appendChild(pill);
    card.appendChild(header);

    const pre = document.createElement('pre');
    pre.className = 'code-block';
    pre.textContent = body;
    card.appendChild(pre);
    out.appendChild(card);
  }

  function redactInstallLogLine(line) {
    return String(line || '')
      .replace(/\b[A-Za-z0-9._%+-]+:[A-Za-z0-9._%+-]+@/g, '***:***@')
      .replace(/\b(sk|rk|pk)-[A-Za-z0-9_-]{10,}\b/g, '$1-***')
      .replace(/\b(token|secret|password|apikey|api_key)\s*[:=]\s*[^\s]+/gi, '$1=***');
  }

  function appendServerManageLiveLogLine(line, stream) {
    const clean = redactInstallLogLine(String(line || '').trim());
    if (!clean) return;
    const prefix = stream === 'stderr' ? '[stderr] ' : '';
    serverManageLiveLogLines.push(prefix + clean);
    if (serverManageLiveLogLines.length > 800) {
      serverManageLiveLogLines = serverManageLiveLogLines.slice(serverManageLiveLogLines.length - 800);
    }
    renderServerManageLogs('--- install-stream.log ---\n' + serverManageLiveLogLines.join('\n'));
  }

  function isInstallAnomalyLine(line) {
    const text = String(line || '').trim().toLowerCase();
    if (!text) return false;
    const patterns = [
      'error',
      'failed',
      'panic',
      'exception',
      'permission denied',
      'timed out',
      'no such file',
      'cannot',
      'denied',
    ];
    return patterns.some(pattern => text.includes(pattern));
  }

  async function requestBaseAgentInstallDiagnosis(hostID, agentID, lines) {
    const tail = (Array.isArray(lines) ? lines : []).slice(-20).join('\n');
    const prompt = [
      'Diagnose remote OpenClaw install progress and likely failure cause.',
      'Host: ' + String(hostID || ''),
      'Agent: ' + String(agentID || ''),
      'Recent install logs:',
      tail || '(empty)',
      'Return concise diagnosis and next 3 actions.',
    ].join('\n');

    const headers = { 'Content-Type': 'application/json' };
    if (token) headers['Authorization'] = 'Bearer ' + token;

    const response = await fetch('/api/v1/chat/stream', {
      method: 'POST',
      headers,
      body: JSON.stringify({
        target: 'local',
        message: prompt,
        provider: 'webui',
        sessionId: '',
        chatId: '',
      }),
    });
    if (!response.ok) {
      const text = await response.text();
      throw new Error(text || 'diagnosis request failed (' + response.status + ')');
    }
    if (!response.body || !response.body.getReader) {
      throw new Error('diagnosis stream is not supported in this browser');
    }

    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';
    let output = '';
    for (;;) {
      const step = await reader.read();
      if (step.done) break;
      buffer += decoder.decode(step.value, { stream: true });
      buffer = parseSSEFrames(buffer, payload => {
        if (String(payload && payload.type ? payload.type : '') === 'text-delta') {
          output += String(payload.delta || '');
        }
      });
    }
    buffer = parseSSEFrames(buffer, payload => {
      if (String(payload && payload.type ? payload.type : '') === 'text-delta') {
        output += String(payload.delta || '');
      }
    });
    return String(output || '').trim();
  }

  function maybeRunServerManageDiagnosis(hostID, agentID) {
    if (serverManageDiagnosisPending || serverManageDiagnosisText) return;
    serverManageDiagnosisPending = true;
    renderServerManageDiagnosis('Analyzing install logs with local BaseAgent...', 'running');
    requestBaseAgentInstallDiagnosis(hostID, agentID, serverManageLiveLogLines)
      .then(text => {
        serverManageDiagnosisText = text || 'No diagnosis returned.';
        renderServerManageDiagnosis(serverManageDiagnosisText, 'ok');
      })
      .catch(err => {
        serverManageDiagnosisText = 'BaseAgent diagnosis unavailable: ' + (err && err.message ? err.message : String(err));
        renderServerManageDiagnosis(serverManageDiagnosisText, 'error');
      })
      .finally(() => {
        serverManageDiagnosisPending = false;
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

  function renderServerManageChatMessages() {
    const wrap = $('#server-manage-chat-messages');
    if (!wrap) return;
    const island = window.CarrierRemoteChatIsland;
    if (island && typeof island.renderMessages === 'function' && island.renderMessages(wrap, serverManageChatMessages)) {
      return;
    }
    wrap.textContent = '';
    serverManageChatMessages.forEach(message => {
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

  function appendServerManageChatMessage(role, text) {
    serverManageChatMessageSeq += 1;
    const messageID = 'server-manage-chat-msg-' + String(serverManageChatMessageSeq);
    serverManageChatMessages.push({
      id: messageID,
      role: String(role || 'system'),
      text: String(text || ''),
    });
    renderServerManageChatMessages();
    return messageID;
  }

  function appendServerManageChatMessageDelta(messageID, delta) {
    const id = String(messageID || '');
    if (!id) return;
    const index = serverManageChatMessages.findIndex(message => String(message.id || '') === id);
    if (index < 0) return;
    serverManageChatMessages[index].text = String(serverManageChatMessages[index].text || '') + String(delta || '');
    renderServerManageChatMessages();
  }

  function updateServerManageChatStatus(text, type) {
    const el = $('#server-manage-chat-status');
    if (!el) return;
    el.textContent = text || '';
    el.classList.remove('msg-error', 'msg-success', 'msg-info');
    if (type) el.classList.add('msg-' + type);
  }

  async function sendServerManageChat(inputText) {
    const target = validateServerManageInstanceTarget();
    if (!target.hostID || !target.agentID) return;
    const message = (inputText || '').trim();
    if (!message) {
      updateServerManageChatStatus('message is required.', 'error');
      return;
    }
    serverManageChatLastInput = message;
    appendServerManageChatMessage('user', message);
    serverManageChatActiveAssistantNode = appendServerManageChatMessage('assistant', '');

    if (serverManageChatAbortController) {
      serverManageChatAbortController.abort();
      serverManageChatAbortController = null;
    }
    const controller = new AbortController();
    serverManageChatAbortController = controller;
    updateServerManageChatStatus('Streaming response...', 'info');

    try {
      const headers = { 'Content-Type': 'application/json' };
      if (token) headers['Authorization'] = 'Bearer ' + token;
      const response = await fetch('/api/v1/chat/stream', {
        method: 'POST',
        headers,
        body: JSON.stringify({
          target: 'remote',
          hostId: target.hostID,
          agentId: target.agentID,
          message: message,
          sessionId: serverManageChatSessionID || '',
        }),
        signal: controller.signal,
      });
      if (!response.ok) {
        const text = await response.text();
        throw new Error(text || 'remote chat failed (' + response.status + ')');
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
            if (serverManageChatActiveAssistantNode) {
              appendServerManageChatMessageDelta(serverManageChatActiveAssistantNode, String(payload.delta || ''));
            }
            return;
          }
          if (eventType === 'session') {
            serverManageChatSessionID = String(payload.sessionId || '').trim();
            updateServerManageChatStatus('Session: ' + serverManageChatSessionID, 'info');
            return;
          }
          if (eventType === 'finish') {
            updateServerManageChatStatus('Stream finished.', 'success');
          }
        });
      }
      buffer = parseSSEFrames(buffer, () => {});
      updateServerManageChatStatus('Stream finished.', 'success');
    } catch (e) {
      if (e.name === 'AbortError') {
        updateServerManageChatStatus('Stream cancelled.', 'info');
      } else {
        updateServerManageChatStatus('Stream failed: ' + e.message, 'error');
      }
    } finally {
      if (serverManageChatAbortController === controller) {
        serverManageChatAbortController = null;
      }
      serverManageChatActiveAssistantNode = null;
    }
  }

  function resetServerManageChatState() {
    if (serverManageChatAbortController) {
      serverManageChatAbortController.abort();
      serverManageChatAbortController = null;
    }
    serverManageChatSessionID = '';
    serverManageChatLastInput = '';
    serverManageChatActiveAssistantNode = null;
    serverManageChatMessages = [];
    renderServerManageChatMessages();
    updateServerManageChatStatus('Ready to chat with selected SSG agent.', 'info');
  }

  function showServerManagePanel(hostID) {
    const card = $('#server-manage-card');
    const hostLabel = $('#server-manage-host-label');
    if (!card || !hostLabel) return;
    if (serverManageInstallStreamAbortController) {
      serverManageInstallStreamAbortController.abort();
      serverManageInstallStreamAbortController = null;
    }
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
    updateServerManageStreamStatus('', 'info');
    serverManageLiveLogLines = [];
    serverManageDiagnosisText = '';
    serverManageDiagnosisPending = false;
    renderServerManageDiagnosis('', '');
    renderServerManageSessions([]);
    renderServerManageMemory([]);
    resetServerManageChatState();
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
      'server-manage-sync-instance',
      'server-manage-sync-status',
      'server-manage-diagnose-instance',
      'server-manage-reconcile-instance',
      'server-manage-rollback-instance',
      'server-manage-codeagent-install',
      'server-manage-codeagent-health',
      'server-manage-codeagent-version',
      'server-manage-codeagent-run',
      'server-manage-load-config',
      'server-manage-apply-config',
      'server-manage-load-sessions',
      'server-manage-archive-session',
      'server-manage-delete-session',
      'server-manage-load-memory',
      'server-manage-sync-mode',
      'server-manage-rollback-commit',
      'server-manage-codeagent-backend',
      'server-manage-codeagent-workspace-root',
      'server-manage-codeagent-capability',
      'server-manage-codeagent-write-mode',
      'server-manage-codeagent-command',
      'server-manage-codeagent-path',
      'server-manage-codeagent-content',
      'server-manage-chat-input',
      'server-manage-chat-send',
      'server-manage-chat-reset-session',
      'server-manage-chat-cancel',
      'server-manage-chat-retry',
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

  async function streamServerManageInstall(target) {
    const path = '/api/v1/remote/hosts/' + encodeURIComponent(target.hostID) + '/instances/' + encodeURIComponent(target.agentID) + '/install/stream';
    const startedAt = performance.now();
    let requestID = '';
    let installPayload = null;
    let streamError = '';
    let finishReason = '';

    if (serverManageInstallStreamAbortController) {
      serverManageInstallStreamAbortController.abort();
      serverManageInstallStreamAbortController = null;
    }
    serverManageLiveLogLines = [];
    serverManageDiagnosisText = '';
    serverManageDiagnosisPending = false;
    renderServerManageLogs('');
    renderServerManageDiagnosis('', '');
    updateServerManageStreamStatus('Connecting stream...', 'info');

    const controller = new AbortController();
    serverManageInstallStreamAbortController = controller;

    try {
      const headers = { 'Content-Type': 'application/json' };
      if (token) headers['Authorization'] = 'Bearer ' + token;

      const response = await fetch(path, {
        method: 'POST',
        headers,
        body: JSON.stringify({}),
        signal: controller.signal,
      });
      if (!response.ok) {
        const text = await response.text();
        throw new Error(text || 'install stream failed (' + response.status + ')');
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
          if (eventType === 'start') {
            requestID = String(payload.requestId || '').trim();
            updateServerManageStreamStatus('Install stream started.', 'info');
            return;
          }
          if (eventType === 'log') {
            const line = String(payload.line || '');
            const stream = String(payload.stream || 'stdout');
            appendServerManageLiveLogLine(line, stream);
            if (isInstallAnomalyLine(line)) {
              updateServerManageStreamStatus('Potential issue detected in install output. Running BaseAgent diagnosis...', 'info');
              maybeRunServerManageDiagnosis(target.hostID, target.agentID);
            }
            return;
          }
          if (eventType === 'result') {
            installPayload = payload.install || null;
            const steps = installPayload && Array.isArray(installPayload.steps) ? installPayload.steps : [];
            renderServerManageInstanceStatus(installPayload || {}, steps);
            updateServerManageStreamStatus('Install stream returned result.', 'info');
            return;
          }
          if (eventType === 'error') {
            streamError = String(payload.message || payload.error || 'remote install failed');
            updateServerManageStreamStatus('Install stream error: ' + streamError, 'error');
            return;
          }
          if (eventType === 'finish') {
            finishReason = String(payload.finishReason || '').trim();
            return;
          }
        });
      }
      buffer = parseSSEFrames(buffer, payload => {
        const eventType = String(payload && payload.type ? payload.type : '').trim();
        if (eventType === 'error') {
          streamError = String(payload.message || payload.error || 'remote install failed');
        }
        if (eventType === 'finish') {
          finishReason = String(payload.finishReason || '').trim();
        }
      });

      const success = !streamError && (!finishReason || finishReason === 'stop');
      setServerManageOperationMeta({
        operation: 'instance_install_stream',
        success: success,
        requestId: requestID,
        durationMs: performance.now() - startedAt,
        path: path,
        error: success ? '' : streamError || ('finishReason=' + finishReason),
      });

      return {
        install: installPayload,
        requestId: requestID,
        error: streamError,
        finishReason: finishReason,
      };
    } catch (e) {
      const errMsg = e && e.message ? e.message : String(e);
      setServerManageOperationMeta({
        operation: 'instance_install_stream',
        success: false,
        durationMs: performance.now() - startedAt,
        path: path,
        error: errMsg,
      });
      throw e;
    } finally {
      if (serverManageInstallStreamAbortController === controller) {
        serverManageInstallStreamAbortController = null;
      }
    }
  }

  async function installServerManageInstance() {
    const target = validateServerManageInstanceTarget();
    if (!target.hostID || !target.agentID) return;
    await runServerManageOperation('install-stream', async () => {
      renderServerManageProgress('Install (Live)', target.hostID + ':' + target.agentID);
      setMsg('#server-manage-msg', 'Live install in progress for ' + target.agentID + '...', 'info');
      try {
        const result = await streamServerManageInstall(target);
        if (result && result.error) {
          setMsg('#server-manage-msg', 'Install failed: ' + result.error, 'error');
          return;
        }
        updateServerManageStreamStatus('Install stream finished.', 'success');
        setMsg('#server-manage-msg', 'Install completed for ' + target.agentID + '.', 'success');
        await loadServerManageInstances({ silent: true, skipLock: true });
      } catch (e) {
        updateServerManageStreamStatus('Install stream failed.', 'error');
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

  async function syncServerManageInstance() {
    const target = validateServerManageInstanceTarget();
    if (!target.hostID || !target.agentID) return;
    const mode = getServerManageSyncMode();
    await runServerManageOperation('sync-instance', async () => {
      renderServerManageProgress('Sync', target.hostID + ':' + target.agentID);
      setMsg('#server-manage-msg', 'Sync in progress for ' + target.agentID + ' (mode=' + mode + ')...', 'info');
      try {
        const path = '/api/v1/remote/hosts/' + encodeURIComponent(target.hostID) + '/instances/' + encodeURIComponent(target.agentID) + '/sync';
        const payload = await serverManageAPI('POST', path, { mode: mode }, 'instance_sync');
        renderServerManageInstanceStatus(payload.sync || {}, payload.steps || []);
        setMsg('#server-manage-msg', 'Sync completed for ' + target.agentID + '.', 'success');
      } catch (e) {
        setMsg('#server-manage-msg', 'Sync failed: ' + e.message, 'error');
      }
    });
  }

  async function loadServerManageSyncStatus() {
    const target = validateServerManageInstanceTarget();
    if (!target.hostID || !target.agentID) return;
    await runServerManageOperation('sync-status', async () => {
      renderServerManageProgress('Sync Status', target.hostID + ':' + target.agentID);
      setMsg('#server-manage-msg', 'Loading sync status for ' + target.agentID + '...', 'info');
      try {
        const path = '/api/v1/remote/hosts/' + encodeURIComponent(target.hostID) + '/instances/' + encodeURIComponent(target.agentID) + '/sync/status';
        const payload = await serverManageAPI('GET', path, null, 'instance_sync_status');
        renderServerManageInstanceStatus(payload.status || {}, []);
        setMsg('#server-manage-msg', 'Sync status loaded for ' + target.agentID + '.', 'success');
      } catch (e) {
        setMsg('#server-manage-msg', 'Load sync status failed: ' + e.message, 'error');
      }
    });
  }

  async function diagnoseServerManageInstance() {
    const target = validateServerManageInstanceTarget();
    if (!target.hostID || !target.agentID) return;
    await runServerManageOperation('diagnose-instance', async () => {
      renderServerManageProgress('Diagnose Drift', target.hostID + ':' + target.agentID);
      setMsg('#server-manage-msg', 'Diagnosing drift for ' + target.agentID + '...', 'info');
      try {
        const path = '/api/v1/remote/hosts/' + encodeURIComponent(target.hostID) + '/instances/' + encodeURIComponent(target.agentID) + '/diagnose';
        const payload = await serverManageAPI('POST', path, {}, 'instance_diagnose');
        renderServerManageInstanceStatus(payload.diagnose || {}, payload.steps || []);
        setMsg('#server-manage-msg', 'Diagnose completed for ' + target.agentID + '.', 'success');
      } catch (e) {
        setMsg('#server-manage-msg', 'Diagnose failed: ' + e.message, 'error');
      }
    });
  }

  async function reconcileServerManageInstance() {
    const target = validateServerManageInstanceTarget();
    if (!target.hostID || !target.agentID) return;
    await runServerManageOperation('reconcile-instance', async () => {
      renderServerManageProgress('Reconcile', target.hostID + ':' + target.agentID);
      setMsg('#server-manage-msg', 'Reconciling ' + target.agentID + '...', 'info');
      try {
        const path = '/api/v1/remote/hosts/' + encodeURIComponent(target.hostID) + '/instances/' + encodeURIComponent(target.agentID) + '/reconcile';
        const payload = await serverManageAPI('POST', path, {}, 'instance_reconcile');
        renderServerManageInstanceStatus(payload.reconcile || {}, payload.steps || []);
        setMsg('#server-manage-msg', 'Reconcile completed for ' + target.agentID + '.', 'success');
      } catch (e) {
        setMsg('#server-manage-msg', 'Reconcile failed: ' + e.message, 'error');
      }
    });
  }

  async function rollbackServerManageInstance() {
    const target = validateServerManageInstanceTarget();
    if (!target.hostID || !target.agentID) return;
    const commit = getServerManageRollbackCommit();
    await runServerManageOperation('rollback-instance', async () => {
      renderServerManageProgress('Rollback', target.hostID + ':' + target.agentID);
      setMsg('#server-manage-msg', 'Rollback in progress for ' + target.agentID + '...', 'info');
      try {
        const path = '/api/v1/remote/hosts/' + encodeURIComponent(target.hostID) + '/instances/' + encodeURIComponent(target.agentID) + '/rollback';
        const body = {};
        if (commit) body.commit = commit;
        const payload = await serverManageAPI('POST', path, body, 'instance_rollback');
        renderServerManageInstanceStatus(payload.rollback || {}, payload.steps || []);
        setMsg('#server-manage-msg', 'Rollback completed for ' + target.agentID + '.', 'success');
      } catch (e) {
        setMsg('#server-manage-msg', 'Rollback failed: ' + e.message, 'error');
      }
    });
  }

  async function installServerManageCodeAgent() {
    const target = validateServerManageInstanceTarget();
    if (!target.hostID || !target.agentID) return;
    const backend = getServerManageCodeAgentBackend();
    const workspaceRoot = getServerManageCodeAgentWorkspaceRoot();
    await runServerManageOperation('codeagent-install', async () => {
      renderServerManageProgress('CodeAgent Install', target.hostID + ':' + target.agentID);
      setMsg('#server-manage-msg', 'Installing codeagent (' + backend + ') for ' + target.agentID + '...', 'info');
      try {
        const path = '/api/v1/remote/hosts/' + encodeURIComponent(target.hostID) + '/instances/' + encodeURIComponent(target.agentID) + '/codeagent/install';
        const payload = await serverManageAPI('POST', path, {
          backend: backend,
          workspaceRoot: workspaceRoot,
        }, 'codeagent_install');
        renderServerManageInstanceStatus(payload.install || {}, []);
        setMsg('#server-manage-msg', 'CodeAgent install completed for ' + target.agentID + '.', 'success');
      } catch (e) {
        setMsg('#server-manage-msg', 'CodeAgent install failed: ' + e.message, 'error');
      }
    });
  }

  async function healthServerManageCodeAgent() {
    const target = validateServerManageInstanceTarget();
    if (!target.hostID || !target.agentID) return;
    const backend = getServerManageCodeAgentBackend();
    const workspaceRoot = getServerManageCodeAgentWorkspaceRoot();
    await runServerManageOperation('codeagent-health', async () => {
      renderServerManageProgress('CodeAgent Health', target.hostID + ':' + target.agentID);
      setMsg('#server-manage-msg', 'Checking codeagent health (' + backend + ') for ' + target.agentID + '...', 'info');
      try {
        const path = '/api/v1/remote/hosts/' + encodeURIComponent(target.hostID) + '/instances/' + encodeURIComponent(target.agentID) + '/codeagent/health?backend=' + encodeURIComponent(backend) + '&workspaceRoot=' + encodeURIComponent(workspaceRoot);
        const payload = await serverManageAPI('GET', path, null, 'codeagent_health');
        renderServerManageInstanceStatus(payload.health || {}, []);
        setMsg('#server-manage-msg', 'CodeAgent health check completed for ' + target.agentID + '.', 'success');
      } catch (e) {
        setMsg('#server-manage-msg', 'CodeAgent health check failed: ' + e.message, 'error');
      }
    });
  }

  async function versionServerManageCodeAgent() {
    const target = validateServerManageInstanceTarget();
    if (!target.hostID || !target.agentID) return;
    const backend = getServerManageCodeAgentBackend();
    await runServerManageOperation('codeagent-version', async () => {
      renderServerManageProgress('CodeAgent Version', target.hostID + ':' + target.agentID);
      setMsg('#server-manage-msg', 'Loading codeagent version (' + backend + ') for ' + target.agentID + '...', 'info');
      try {
        const path = '/api/v1/remote/hosts/' + encodeURIComponent(target.hostID) + '/instances/' + encodeURIComponent(target.agentID) + '/codeagent/version?backend=' + encodeURIComponent(backend);
        const payload = await serverManageAPI('GET', path, null, 'codeagent_version');
        renderServerManageInstanceStatus(payload.version || { backend: backend }, []);
        setMsg('#server-manage-msg', 'CodeAgent version loaded for ' + target.agentID + '.', 'success');
      } catch (e) {
        setMsg('#server-manage-msg', 'CodeAgent version failed: ' + e.message, 'error');
      }
    });
  }

  async function runServerManageCodeAgent() {
    const target = validateServerManageInstanceTarget();
    if (!target.hostID || !target.agentID) return;
    const backend = getServerManageCodeAgentBackend();
    const workspaceRoot = getServerManageCodeAgentWorkspaceRoot();
    const capability = getServerManageCodeAgentCapability();
    const command = getServerManageCodeAgentCommand();
    const pathInput = getServerManageCodeAgentPath();
    const content = getServerManageCodeAgentContent();
    const writeMode = getServerManageCodeAgentWriteMode();

    if ((capability === 'run_shell' || capability === 'run_shell_redirect') && !command) {
      setMsg('#server-manage-msg', 'CodeAgent command is required for run_shell capability.', 'error');
      return;
    }
    if ((capability === 'read_file' || capability === 'write_file') && !pathInput) {
      setMsg('#server-manage-msg', 'CodeAgent path is required for file capabilities.', 'error');
      return;
    }
    if ((capability === 'write_file' || capability === 'apply_patch') && !content) {
      setMsg('#server-manage-msg', 'CodeAgent content is required for write/apply_patch.', 'error');
      return;
    }

    await runServerManageOperation('codeagent-run', async () => {
      renderServerManageProgress('CodeAgent Run', target.hostID + ':' + target.agentID);
      setMsg('#server-manage-msg', 'Running codeagent capability ' + capability + ' on ' + target.agentID + '...', 'info');
      try {
        const endpoint = '/api/v1/remote/hosts/' + encodeURIComponent(target.hostID) + '/instances/' + encodeURIComponent(target.agentID) + '/codeagent/run';
        const payload = await serverManageAPI('POST', endpoint, {
          backend: backend,
          workspaceRoot: workspaceRoot,
          capability: capability,
          command: command,
          path: pathInput,
          content: content,
          writeMode: writeMode,
        }, 'codeagent_run');
        const runBlock = payload && payload.run && typeof payload.run === 'object' ? payload.run : {};
        const runResult = runBlock && runBlock.result && typeof runBlock.result === 'object' ? runBlock.result : {};
        renderServerManageInstanceStatus(runResult, []);

        const stdout = runResult && runResult.stdout ? String(runResult.stdout) : '';
        const stderr = runResult && runResult.stderr ? String(runResult.stderr) : '';
        const parts = [];
        if (stdout.trim()) parts.push('[stdout]\n' + stdout.trim());
        if (stderr.trim()) parts.push('[stderr]\n' + stderr.trim());
        renderServerManageLogs(parts.join('\n\n'));

        const policyDecision = String(runResult.policy_decision || '').trim();
        if (policyDecision === 'deny' || policyDecision === 'ask') {
          setMsg('#server-manage-msg', 'CodeAgent run blocked by policy (' + policyDecision + ').', 'error');
          return;
        }
        setMsg('#server-manage-msg', 'CodeAgent run completed for ' + target.agentID + '.', 'success');
      } catch (e) {
        setMsg('#server-manage-msg', 'CodeAgent run failed: ' + e.message, 'error');
      }
    });
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
          let checkPayload = await serverManageAPI('POST', path, {}, 'host_check');
          const pendingPull = checkPayload && Array.isArray(checkPayload.pendingPullInstances)
            ? checkPayload.pendingPullInstances
            : [];
          if (pendingPull.length > 0) {
            const pendingAgentIDs = pendingPull
              .map(instance => pickRemoteInstanceAgentID(instance))
              .map(id => String(id || '').trim())
              .filter(Boolean);
            const prompt = pendingAgentIDs.length > 0
              ? 'Discovered new remote instances (' + pendingAgentIDs.join(', ') + '). Pull configs to local machine now?'
              : 'Discovered new remote instances. Pull configs to local machine now?';
            const confirmed = window.confirm(prompt);
            if (confirmed) {
              checkPayload = await serverManageAPI('POST', path, { pullNewInstances: true }, 'host_check_confirm_pull');
              setMsg('#servers-msg', 'Health check completed and pulled new instance configs: ' + (host.name || host.id || key), 'success');
            } else {
              setMsg('#servers-msg', 'Health check completed (new instance config pull skipped): ' + (host.name || host.id || key), 'success');
            }
          } else {
            setMsg('#servers-msg', 'Health check completed: ' + (host.name || host.id || key), 'success');
          }
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
      }, hostOperations, {
        canManageHosts: canManageHostsUI(),
      })
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
        'labels: ' + (Array.isArray(host.labels) && host.labels.length ? host.labels.join(', ') : '-'),
        'health: ' + (host.lastHealth || 'unknown'),
      ];
      lines.push(...formatServerHostOperationMetaLines(host.id));
      meta.textContent = lines.join('\n');
      meta.style.whiteSpace = 'pre-line';
      card.appendChild(title);
      card.appendChild(meta);
      if (canManageHostsUI()) {
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
        card.appendChild(actions);
      }
      wrap.appendChild(card);
    });
  }

  async function initServers() {
    showView('servers');
    $('#nav').classList.remove('hidden');

    const authMode = $('#server-auth-mode');
    const sshConfigSelect = $('#server-ssh-config-host-select');
    const refreshBtn = $('#servers-refresh');
    const saveBtn = $('#server-save');
    const cancelEditBtn = $('#server-cancel-edit');
    const manageCard = $('#server-manage-card');
    const loadInstancesBtn = $('#server-manage-load-instances');
    const instanceStatusBtn = $('#server-manage-instance-status');
    const installInstanceBtn = $('#server-manage-install-instance');
    const repairInstanceBtn = $('#server-manage-repair-instance');
    const loadLogsBtn = $('#server-manage-load-logs');
    const syncInstanceBtn = $('#server-manage-sync-instance');
    const syncStatusBtn = $('#server-manage-sync-status');
    const diagnoseInstanceBtn = $('#server-manage-diagnose-instance');
    const reconcileInstanceBtn = $('#server-manage-reconcile-instance');
    const rollbackInstanceBtn = $('#server-manage-rollback-instance');
    const codeAgentInstallBtn = $('#server-manage-codeagent-install');
    const codeAgentHealthBtn = $('#server-manage-codeagent-health');
    const codeAgentVersionBtn = $('#server-manage-codeagent-version');
    const codeAgentRunBtn = $('#server-manage-codeagent-run');
    const loadConfigBtn = $('#server-manage-load-config');
    const applyConfigBtn = $('#server-manage-apply-config');
    const loadSessionsBtn = $('#server-manage-load-sessions');
    const archiveSessionBtn = $('#server-manage-archive-session');
    const deleteSessionBtn = $('#server-manage-delete-session');
    const loadMemoryBtn = $('#server-manage-load-memory');
    const chatInput = $('#server-manage-chat-input');
    const chatSendBtn = $('#server-manage-chat-send');
    const chatResetBtn = $('#server-manage-chat-reset-session');
    const chatCancelBtn = $('#server-manage-chat-cancel');
    const chatRetryBtn = $('#server-manage-chat-retry');
    const instancesOut = $('#server-manage-instances');
    const instanceStatusOut = $('#server-manage-instance-status-out');
    const logsOut = $('#server-manage-logs');
    const streamStatusOut = $('#server-manage-stream-status');
    const diagnosisOut = $('#server-manage-diagnosis');
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
      if (!canManageHostsUI()) {
        serverManageHostID = '';
        if (manageCard) manageCard.classList.add('hidden');
        return;
      }
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
        if (streamStatusOut) streamStatusOut.textContent = '';
        if (diagnosisOut) diagnosisOut.textContent = '';
        serverManageLiveLogLines = [];
        serverManageDiagnosisText = '';
        serverManageDiagnosisPending = false;
        if (sessionsOut) sessionsOut.textContent = '';
        if (memoryOut) memoryOut.textContent = '';
        setServerManageOperationMeta(null);
        resetServerManageChatState();
        setMsg('#server-manage-msg', '', 'info');
        return;
      }
      showServerManagePanel(current);
    }

    authMode.onchange = syncServerAuthModeInputs;
    if (sshConfigSelect) {
      sshConfigSelect.onchange = () => {
        const picked = String(sshConfigSelect.value || '').trim();
        if (!picked) return;
        const input = $('#server-ssh-config-host');
        if (!input) return;
        input.value = picked;
      };
    }
    syncServerAuthModeInputs();
    updateServerEditorUI();
    setServerManageControlsDisabled(!canManageHostsUI());
    renderServerManageChatMessages();
    updateServerManageChatStatus('Ready to chat with selected SSG agent.', 'info');

    if (loadInstancesBtn) loadInstancesBtn.onclick = () => { loadServerManageInstances(); };
    if (instanceStatusBtn) instanceStatusBtn.onclick = () => { loadServerManageInstanceStatus(); };
    if (installInstanceBtn) installInstanceBtn.onclick = () => { installServerManageInstance(); };
    if (repairInstanceBtn) repairInstanceBtn.onclick = () => { repairServerManageInstance(); };
    if (loadLogsBtn) loadLogsBtn.onclick = () => { loadServerManageLogs(); };
    if (syncInstanceBtn) syncInstanceBtn.onclick = () => { syncServerManageInstance(); };
    if (syncStatusBtn) syncStatusBtn.onclick = () => { loadServerManageSyncStatus(); };
    if (diagnoseInstanceBtn) diagnoseInstanceBtn.onclick = () => { diagnoseServerManageInstance(); };
    if (reconcileInstanceBtn) reconcileInstanceBtn.onclick = () => { reconcileServerManageInstance(); };
    if (rollbackInstanceBtn) rollbackInstanceBtn.onclick = () => { rollbackServerManageInstance(); };
    if (codeAgentInstallBtn) codeAgentInstallBtn.onclick = () => { installServerManageCodeAgent(); };
    if (codeAgentHealthBtn) codeAgentHealthBtn.onclick = () => { healthServerManageCodeAgent(); };
    if (codeAgentVersionBtn) codeAgentVersionBtn.onclick = () => { versionServerManageCodeAgent(); };
    if (codeAgentRunBtn) codeAgentRunBtn.onclick = () => { runServerManageCodeAgent(); };
    if (loadConfigBtn) loadConfigBtn.onclick = () => { loadServerManageConfig(); };
    if (applyConfigBtn) applyConfigBtn.onclick = () => { applyServerManageConfigPatch(); };
    if (loadSessionsBtn) loadSessionsBtn.onclick = () => { loadServerManageSessions(); };
    if (archiveSessionBtn) archiveSessionBtn.onclick = () => { applyServerManageSessionAction('archive'); };
    if (deleteSessionBtn) deleteSessionBtn.onclick = () => { applyServerManageSessionAction('delete'); };
    if (loadMemoryBtn) loadMemoryBtn.onclick = () => { loadServerManageMemory(); };
    if (chatSendBtn) {
      chatSendBtn.onclick = () => {
        const text = chatInput && chatInput.value ? chatInput.value.trim() : '';
        if (!text) return;
        if (chatInput) chatInput.value = '';
        sendServerManageChat(text);
      };
    }
    if (chatInput) {
      chatInput.onkeydown = e => {
        if (e.key !== 'Enter') return;
        const text = chatInput.value ? chatInput.value.trim() : '';
        if (!text) return;
        chatInput.value = '';
        sendServerManageChat(text);
      };
    }
    if (chatResetBtn) {
      chatResetBtn.onclick = () => {
        serverManageChatSessionID = '';
        updateServerManageChatStatus('Session reset. Next message starts a new session.', 'info');
      };
    }
    if (chatCancelBtn) {
      chatCancelBtn.onclick = () => {
        if (serverManageChatAbortController) {
          serverManageChatAbortController.abort();
        } else {
          updateServerManageChatStatus('No active stream.', 'info');
        }
      };
    }
    if (chatRetryBtn) {
      chatRetryBtn.onclick = () => {
        if (!serverManageChatLastInput) {
          updateServerManageChatStatus('No previous message to retry.', 'info');
          return;
        }
        sendServerManageChat(serverManageChatLastInput);
      };
    }

    refreshBtn.onclick = async () => {
      try {
        const aliases = await fetchSSHConfigHostAliases();
        renderSSHConfigHostAliasOptions(aliases, '');
      } catch (e) {
        renderSSHConfigHostAliasOptions(sshConfigHostAliasesCache, e && e.message ? e.message : String(e));
      }

      try {
        if (canManageHostsUI()) {
          setMsg('#servers-msg', '', 'info');
        }
        const hosts = await fetchRemoteHosts();
        renderServersList(hosts);
        syncManageSelection(hosts);
        syncServerEditSelection(hosts);
        if (!canManageHostsUI()) {
          setMsg('#servers-msg', 'Current role cannot modify remote hosts.', 'info');
        }
      } catch (e) {
        setMsg('#servers-msg', 'Load failed: ' + e.message, 'error');
        renderServersList([]);
        syncManageSelection([]);
        syncServerEditSelection([]);
      }
    };

    if (saveBtn) saveBtn.disabled = !canManageHostsUI();
    if (cancelEditBtn) cancelEditBtn.disabled = !canManageHostsUI();
    if (!canManageHostsUI()) {
      setMsg('#servers-msg', 'Current role cannot modify remote hosts.', 'info');
    }

    saveBtn.onclick = async () => {
      if (!canManageHostsUI()) {
        setMsg('#servers-msg', 'Current role cannot modify remote hosts.', 'error');
        return;
      }
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
        labels: parseCommaSeparatedValues(($('#server-labels').value || '').trim()).sort((a, b) => a.localeCompare(b)),
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

  function renderProfilesAndBindings(profiles, bindings, policies) {
    const profilesWrap = $('#profiles-list');
    const bindingsWrap = $('#bindings-list');
    const policiesWrap = $('#execution-policies-list');
    if (!profilesWrap || !bindingsWrap || !policiesWrap) return;
    const profileByID = new Map(
      (Array.isArray(profiles) ? profiles : []).map(profile => [String(profile && profile.id ? profile.id : ''), profile]),
    );
    const bindingByID = new Map(
      (Array.isArray(bindings) ? bindings : []).map(binding => [String(binding && binding.id ? binding.id : ''), binding]),
    );
    const policyByID = new Map(
      (Array.isArray(policies) ? policies : []).map(policy => [String(policy && policy.id ? policy.id : ''), policy]),
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

    async function handleDeletePolicy(policyID) {
      const key = String(policyID || '').trim();
      if (!key) return;
      const policy = policyByID.get(key) || {};
      const label = String(policy && policy.name ? policy.name : key);
      if (!window.confirm('Delete execution policy ' + label + '?')) return;
      try {
        await api('DELETE', '/api/v1/orchestrator/policies/' + encodeURIComponent(key));
        setMsg('#profiles-msg', 'Execution policy deleted: ' + label, 'success');
        await initProfiles();
      } catch (e) {
        setMsg('#profiles-msg', 'Delete execution policy failed: ' + e.message, 'error');
      }
    }

    const island = window.CarrierRemoteControlIslands;
    const renderedByIsland = !!(
      island &&
      typeof island.renderProfilesAndBindings === 'function' &&
      island.renderProfilesAndBindings(profilesWrap, bindingsWrap, profiles, bindings, {
        onTestProfile: handleTestProfile,
        onEditProfile: handleEditProfile,
        onDeleteProfile: handleDeleteProfile,
        onDeleteBinding: handleDeleteBinding,
      }, {
        canManageProviders: canManageProvidersUI(),
      })
    );

    policiesWrap.textContent = '';
    if (!renderedByIsland) {
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
          card.appendChild(title);
          card.appendChild(meta);
          if (canManageProvidersUI()) {
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
            card.appendChild(actions);
          }
          profilesWrap.appendChild(card);
        });
      }

      if (!bindings.length) {
        const empty = document.createElement('div');
        empty.className = 'card';
        empty.textContent = 'No provider bindings configured.';
        bindingsWrap.appendChild(empty);
      } else {
        bindings.forEach(binding => {
          const card = document.createElement('div');
          card.className = 'agent-card';
          const title = document.createElement('h4');
          title.textContent = (binding.targetType || '-') + ': ' + (binding.targetId || '-');
          const meta = document.createElement('div');
          meta.className = 'instance-meta';
          meta.textContent = 'id: ' + (binding.id || '-') + '\nprofileId: ' + (binding.profileId || '-') + '\nsyncMode: ' + (binding.syncMode || 'always_push');
          meta.style.whiteSpace = 'pre-line';
          card.appendChild(title);
          card.appendChild(meta);
          if (canManageProvidersUI()) {
            const actions = document.createElement('div');
            actions.className = 'btn-row';
            const deleteBtn = document.createElement('button');
            deleteBtn.className = 'btn-sm btn-danger';
            deleteBtn.textContent = 'Delete';
            deleteBtn.onclick = () => handleDeleteBinding(binding.id);
            card.appendChild(actions);
            actions.appendChild(deleteBtn);
          }
          bindingsWrap.appendChild(card);
        });
      }
    }

    if (!policies.length) {
      const empty = document.createElement('div');
      empty.className = 'card';
      empty.textContent = 'No execution policies configured.';
      policiesWrap.appendChild(empty);
    } else {
      policies.forEach(policy => {
        const card = document.createElement('div');
        card.className = 'agent-card';
        const title = document.createElement('h4');
        title.textContent = policy.name || policy.id;
        const meta = document.createElement('div');
        meta.className = 'instance-meta';
        meta.textContent =
          'action: ' + (policy.action || 'allow') +
          '\nenabled: ' + String(policy.enabled !== false) +
          '\npriority: ' + String(policy.priority || 0) +
          (policy.reason ? '\nreason: ' + policy.reason : '') +
          (policy.teams && policy.teams.length ? '\nteams: ' + policy.teams.join(', ') : '') +
          (policy.projects && policy.projects.length ? '\nprojects: ' + policy.projects.join(', ') : '') +
          (policy.environments && policy.environments.length ? '\nenvironments: ' + policy.environments.join(', ') : '') +
          (policy.templateIds && policy.templateIds.length ? '\ntemplates: ' + policy.templateIds.join(', ') : '') +
          (policy.requestedProviders && policy.requestedProviders.length ? '\nproviders: ' + policy.requestedProviders.join(', ') : '') +
          (policy.hostIds && policy.hostIds.length ? '\nhosts: ' + policy.hostIds.join(', ') : '') +
          (policy.hostLabels && policy.hostLabels.length ? '\nhost labels: ' + policy.hostLabels.join(', ') : '') +
          (policy.agentIds && policy.agentIds.length ? '\nagents: ' + policy.agentIds.join(', ') : '') +
          (policy.allowedTools && policy.allowedTools.length ? '\nallowed tools: ' + policy.allowedTools.join(', ') : '') +
          (typeof policy.maxTaskTimeoutMs === 'number' ? '\nmax timeout: ' + String(policy.maxTaskTimeoutMs) + 'ms' : '') +
          (typeof policy.maxRetryBudget === 'number' ? '\nmax retry: ' + String(policy.maxRetryBudget) : '');
        meta.style.whiteSpace = 'pre-line';
        card.appendChild(title);
        card.appendChild(meta);
        if (canManagePoliciesUI()) {
          const actions = document.createElement('div');
          actions.className = 'btn-row';
          const deleteBtn = document.createElement('button');
          deleteBtn.className = 'btn-sm btn-danger';
          deleteBtn.textContent = 'Delete';
          deleteBtn.onclick = () => handleDeletePolicy(policy.id);
          actions.appendChild(deleteBtn);
          card.appendChild(actions);
        }
        policiesWrap.appendChild(card);
      });
    }
  }

  function renderExecutionTriggers(triggers, onEditTrigger, onToggleTrigger, onDeleteTrigger) {
    const wrap = $('#execution-triggers-list');
    if (!wrap) return;
    wrap.textContent = '';
    const list = Array.isArray(triggers) ? triggers : [];
    if (!list.length) {
      const empty = document.createElement('div');
      empty.className = 'card';
      empty.textContent = 'No execution triggers configured.';
      wrap.appendChild(empty);
      return;
    }
    list.forEach(trigger => {
      const config = trigger && trigger.config && typeof trigger.config === 'object' ? trigger.config : {};
      const card = document.createElement('div');
      card.className = 'agent-card';
      const title = document.createElement('h4');
      title.textContent = trigger.name || trigger.id;
      const meta = document.createElement('div');
      meta.className = 'instance-meta';
      meta.textContent =
        'id: ' + (trigger.id || '-') +
        '\ntype: ' + (trigger.type || '-') +
        '\ntemplate: ' + (trigger.templateId || '-') +
        '\nenabled: ' + String(trigger.enabled !== false) +
        (trigger.createdBy ? '\ncreated by: ' + trigger.createdBy : '') +
        (config.provider ? '\nprovider: ' + config.provider : '') +
        (config.hostIds && config.hostIds.length ? '\nhost ids: ' + config.hostIds.join(', ') : '') +
        (config.hostLabels && config.hostLabels.length ? '\nhost labels: ' + config.hostLabels.join(', ') : '') +
        (typeof config.maxConcurrency === 'number' && config.maxConcurrency > 0 ? '\nmax concurrency: ' + String(config.maxConcurrency) : '') +
        (config.policyApprove ? '\npolicy approve: true' : '') +
        (config.webhookSecretConfigured ? '\nwebhook secret: configured' : '') +
        (config.githubCommand ? '\ngithub command: ' + config.githubCommand : '') +
        (config.githubLabel ? '\ngithub label: ' + config.githubLabel : '') +
        (config.githubRepository ? '\ngithub repository: ' + config.githubRepository : '') +
        (config.cron ? '\ncron: ' + config.cron : '') +
        (config.timezone ? '\ntimezone: ' + config.timezone : '') +
        (trigger.nextRunAt ? '\nnext run: ' + trigger.nextRunAt : '') +
        (typeof trigger.triggeredCount === 'number' && trigger.triggeredCount > 0 ? '\ntriggered count: ' + String(trigger.triggeredCount) : '') +
        (trigger.lastExecutionId ? '\nlast execution: ' + trigger.lastExecutionId : '') +
        (trigger.lastError ? '\nlast error: ' + trigger.lastError : '') +
        (config.inputs && Object.keys(config.inputs).length ? '\ninputs:\n' + renderTriggerInputsText(config.inputs) : '');
      meta.style.whiteSpace = 'pre-line';
      card.appendChild(title);
      card.appendChild(meta);
      if (canManagePoliciesUI()) {
        const actions = document.createElement('div');
        actions.className = 'btn-row';

        const editBtn = document.createElement('button');
        editBtn.className = 'btn-sm btn-secondary';
        editBtn.textContent = 'Edit';
        editBtn.onclick = () => onEditTrigger(trigger.id);

        const toggleBtn = document.createElement('button');
        toggleBtn.className = 'btn-sm btn-secondary';
        toggleBtn.textContent = trigger.enabled === false ? 'Enable' : 'Disable';
        toggleBtn.onclick = () => onToggleTrigger(trigger.id, trigger.enabled === false);

        const deleteBtn = document.createElement('button');
        deleteBtn.className = 'btn-sm btn-danger';
        deleteBtn.textContent = 'Delete';
        deleteBtn.onclick = () => onDeleteTrigger(trigger.id);

        actions.appendChild(editBtn);
        actions.appendChild(toggleBtn);
        actions.appendChild(deleteBtn);
        card.appendChild(actions);
      }
      wrap.appendChild(card);
    });
  }

  function parseCommaSeparatedValues(raw) {
    return String(raw || '')
      .split(',')
      .map(item => String(item || '').trim())
      .filter(Boolean);
  }

  function renderGovernancePreviewResolution(payload) {
    const out = $('#governance-preview-out');
    if (!out) return;
    const resolution = payload && payload.resolution && typeof payload.resolution === 'object'
      ? payload.resolution
      : {};
    const trace = Array.isArray(resolution.trace) ? resolution.trace : [];
    const lines = [];
    lines.push('source=' + String(resolution.source || 'none'));
    lines.push('status=' + String(resolution.status || 'unbound'));
    if (resolution.driftState) {
      lines.push('drift=' + String(resolution.driftState));
    }
    if (resolution.profileName || resolution.profileId) {
      lines.push('profile=' + String(resolution.profileName || resolution.profileId));
    }
    if (resolution.provider || resolution.model) {
      lines.push([String(resolution.provider || ''), String(resolution.model || '')].filter(Boolean).join('/'));
    }
    if (resolution.syncMode) {
      lines.push('sync=' + String(resolution.syncMode));
    }
    if (resolution.message) {
      lines.push(String(resolution.message));
    }
    if (resolution.driftReason) {
      lines.push(String(resolution.driftReason));
    }
    trace.forEach(item => {
      const source = String(item && item.source ? item.source : 'unknown').trim() || 'unknown';
      const status = String(item && item.status ? item.status : 'unknown').trim() || 'unknown';
      const selected = !!(item && item.selected);
      const providerModel = [String(item && item.provider ? item.provider : ''), String(item && item.model ? item.model : '')]
        .filter(Boolean)
        .join('/');
      const label = source + ' [' + status + (selected ? ', selected' : '') + ']';
      lines.push(label + (providerModel ? ' ' + providerModel : ''));
    });
    out.textContent = lines.join('\n');
  }

  async function initProfiles() {
    showView('profiles');
    $('#nav').classList.remove('hidden');

    const refreshBtn = $('#profiles-refresh');
    const saveProfileBtn = $('#profile-save');
    const cancelEditBtn = $('#profile-cancel-edit');
    const saveBindingBtn = $('#binding-save');
    const saveExecutionPolicyBtn = $('#execution-policy-save');
    const saveTriggerBtn = $('#trigger-save');
    const cancelTriggerEditBtn = $('#trigger-cancel-edit');
    const profileSelect = $('#binding-profile-id');
    const bindingTargetType = $('#binding-target-type');
    const bindingTargetID = $('#binding-target-id');
    const profileTestHostSelect = $('#profile-test-host');
    const governancePreviewHost = $('#governance-preview-host');
    const governancePreviewAgent = $('#governance-preview-agent');
    const governancePreviewResolve = $('#governance-preview-resolve');

    function syncBindingControls() {
      const bindingEnabled = !!featureFlags.providerBindingEnabled;
      const canManageProviders = canManageProvidersUI();
      profileSelect.disabled = !(bindingEnabled && canManageProviders);
      bindingTargetType.disabled = !(bindingEnabled && canManageProviders);
      bindingTargetID.disabled = !(bindingEnabled && canManageProviders);
      saveBindingBtn.disabled = !(bindingEnabled && canManageProviders);
      if (governancePreviewHost) governancePreviewHost.disabled = !bindingEnabled;
      if (governancePreviewAgent) governancePreviewAgent.disabled = !bindingEnabled;
      if (governancePreviewResolve) governancePreviewResolve.disabled = !bindingEnabled;
      if (!bindingEnabled) {
        setMsg('#profiles-msg', 'Provider binding is disabled by feature flag.', 'info');
      } else if (!canManageProviders || !canManagePoliciesUI()) {
        setMsg('#profiles-msg', 'Current role has read-only access to provider and policy settings.', 'info');
      }
    }

    function syncTriggerControls() {
      const canManageTriggers = canManagePoliciesUI();
      if (saveTriggerBtn) saveTriggerBtn.disabled = !canManageTriggers;
      [
        '#trigger-name',
        '#trigger-type',
        '#trigger-template-id',
        '#trigger-provider',
        '#trigger-host-ids',
        '#trigger-host-labels',
        '#trigger-max-concurrency',
        '#trigger-inputs',
        '#trigger-webhook-secret',
        '#trigger-github-command',
        '#trigger-github-label',
        '#trigger-github-repository',
        '#trigger-cron',
        '#trigger-timezone',
        '#trigger-policy-approve',
      ].forEach(selector => {
        const el = $(selector);
        if (!el) return;
        el.disabled = !canManageTriggers;
      });
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
        const [profilesPayload, bindingsPayload, hosts, policiesPayload, triggersPayload, templatesPayload] = await Promise.all([
          api('GET', '/api/v1/provider-profiles'),
          api('GET', '/api/v1/provider-bindings'),
          fetchRemoteHosts(),
          api('GET', '/api/v1/orchestrator/policies'),
          canViewExecutionsUI() ? api('GET', '/api/v1/triggers') : Promise.resolve({ result: 'ok', triggers: [] }),
          canViewExecutionsUI() ? api('GET', '/api/v1/templates') : Promise.resolve({ result: 'ok', templates: [] }),
        ]);
        const profiles = profilesPayload && Array.isArray(profilesPayload.profiles) ? profilesPayload.profiles : [];
        const bindings = bindingsPayload && Array.isArray(bindingsPayload.bindings) ? bindingsPayload.bindings : [];
        const policies = policiesPayload && Array.isArray(policiesPayload.policies) ? policiesPayload.policies : [];
        const triggers = triggersPayload && Array.isArray(triggersPayload.triggers) ? triggersPayload.triggers : [];
        const templates = templatesPayload && Array.isArray(templatesPayload.templates) ? templatesPayload.templates : [];
        providerProfilesCache = profiles;
        executionTemplatesCache = templates;
        executionTriggersCache = triggers;
        orchestratorPolicyRulesCache = policies;
        remoteHostsCache = hosts;
        pruneServerHostOperationCache(hosts);
        syncProfileTestHostOptions(hosts);
        syncTriggerTemplateOptions(templates);
        if (governancePreviewHost) {
          governancePreviewHost.textContent = '';
          hosts.forEach(host => {
            const opt = document.createElement('option');
            opt.value = host.id;
            opt.textContent = host.name || host.id;
            governancePreviewHost.appendChild(opt);
          });
        }
        renderProfilesAndBindings(profiles, bindings, policies);
        renderExecutionTriggers(
          triggers,
          (triggerID) => {
            beginTriggerEdit(triggerID);
            setMsg('#profiles-msg', 'Editing trigger: ' + triggerID, 'info');
          },
          async (triggerID, nextEnabled) => {
            const key = String(triggerID || '').trim();
            if (!key) return;
            const trigger = executionTriggersCache.find(item => String(item && item.id ? item.id : '') === key) || {};
            const label = String(trigger && trigger.name ? trigger.name : key);
            try {
              await api('PATCH', '/api/v1/triggers/' + encodeURIComponent(key), { enabled: !!nextEnabled });
              await refreshAll();
              setMsg('#profiles-msg', 'Execution trigger updated: ' + label, 'success');
            } catch (e) {
              setMsg('#profiles-msg', 'Update execution trigger failed: ' + e.message, 'error');
            }
          },
          async (triggerID) => {
            const key = String(triggerID || '').trim();
            if (!key) return;
            const trigger = executionTriggersCache.find(item => String(item && item.id ? item.id : '') === key) || {};
            const label = String(trigger && trigger.name ? trigger.name : key);
            if (!window.confirm('Delete execution trigger ' + label + '?')) return;
            try {
              await api('DELETE', '/api/v1/triggers/' + encodeURIComponent(key));
              if (String(triggerEditingTriggerID || '') === key) {
                resetTriggerEditor(true);
              }
              await refreshAll();
              setMsg('#profiles-msg', 'Execution trigger deleted: ' + label, 'success');
            } catch (e) {
              setMsg('#profiles-msg', 'Delete execution trigger failed: ' + e.message, 'error');
            }
          },
        );
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
        renderProfilesAndBindings([], [], []);
        renderExecutionTriggers([], () => {}, () => {}, () => {});
      } finally {
        syncBindingControls();
        syncTriggerControls();
      }
    }

    refreshBtn.onclick = refreshAll;

    saveProfileBtn.onclick = async () => {
      if (!canManageProvidersUI()) {
        setMsg('#profiles-msg', 'Current role cannot modify provider profiles.', 'error');
        return;
      }
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
      if (!canManageProvidersUI()) {
        setMsg('#profiles-msg', 'Current role cannot modify provider bindings.', 'error');
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

    if (saveExecutionPolicyBtn) {
      saveExecutionPolicyBtn.onclick = async () => {
        const payload: any = {
          name: ($('#execution-policy-name').value || '').trim(),
          action: ($('#execution-policy-action').value || 'ask').trim(),
          priority: parseInt(($('#execution-policy-priority').value || '0').trim(), 10) || 0,
          reason: ($('#execution-policy-reason').value || '').trim(),
          teams: parseCommaSeparatedValues(($('#execution-policy-teams').value || '').trim()).sort((a, b) => a.localeCompare(b)),
          projects: parseCommaSeparatedValues(($('#execution-policy-projects').value || '').trim()).sort((a, b) => a.localeCompare(b)),
          environments: parseCommaSeparatedValues(($('#execution-policy-environments').value || '').trim()).sort((a, b) => a.localeCompare(b)),
          templateIds: parseCommaSeparatedValues(($('#execution-policy-template-ids').value || '').trim()).sort((a, b) => a.localeCompare(b)),
          requestedProviders: parseCommaSeparatedValues(($('#execution-policy-providers').value || '').trim()).sort((a, b) => a.localeCompare(b)),
          hostIds: parseCommaSeparatedValues(($('#execution-policy-host-ids').value || '').trim()).sort((a, b) => a.localeCompare(b)),
          hostLabels: parseCommaSeparatedValues(($('#execution-policy-host-labels').value || '').trim()).sort((a, b) => a.localeCompare(b)),
          agentIds: parseCommaSeparatedValues(($('#execution-policy-agent-ids').value || '').trim()).sort((a, b) => a.localeCompare(b)),
          allowedTools: parseCommaSeparatedValues(($('#execution-policy-allowed-tools').value || '').trim()).sort((a, b) => a.localeCompare(b)),
          enabled: ($('#execution-policy-enabled').value || 'true') === 'true',
        };
        const timeoutRaw = String(($('#execution-policy-max-timeout-ms').value || '')).trim();
        const retryRaw = String(($('#execution-policy-max-retry-budget').value || '')).trim();
        if (timeoutRaw !== '') {
          payload.maxTaskTimeoutMs = parseInt(timeoutRaw, 10) || 0;
        }
        if (retryRaw !== '') {
          payload.maxRetryBudget = parseInt(retryRaw, 10);
        }
        if (!payload.name) {
          setMsg('#profiles-msg', 'execution policy name is required.', 'error');
          return;
        }
        if (!canManagePoliciesUI()) {
          setMsg('#profiles-msg', 'Current role cannot modify execution policies.', 'error');
          return;
        }
        try {
          saveExecutionPolicyBtn.disabled = true;
          await api('POST', '/api/v1/orchestrator/policies', payload);
          $('#execution-policy-name').value = '';
          $('#execution-policy-action').value = 'ask';
          $('#execution-policy-priority').value = '0';
          $('#execution-policy-reason').value = '';
          $('#execution-policy-teams').value = '';
          $('#execution-policy-projects').value = '';
          $('#execution-policy-environments').value = '';
          $('#execution-policy-template-ids').value = '';
          $('#execution-policy-providers').value = '';
          $('#execution-policy-host-ids').value = '';
          $('#execution-policy-host-labels').value = '';
          $('#execution-policy-agent-ids').value = '';
          $('#execution-policy-allowed-tools').value = '';
          $('#execution-policy-max-timeout-ms').value = '';
          $('#execution-policy-max-retry-budget').value = '';
          $('#execution-policy-enabled').value = 'true';
          await refreshAll();
          setMsg('#profiles-msg', 'Execution policy saved.', 'success');
        } catch (e) {
          setMsg('#profiles-msg', 'Save execution policy failed: ' + e.message, 'error');
        } finally {
          saveExecutionPolicyBtn.disabled = false;
        }
      };
    }

    if (saveTriggerBtn) {
      saveTriggerBtn.onclick = async () => {
        if (!canManagePoliciesUI()) {
          setMsg('#profiles-msg', 'Current role cannot modify execution triggers.', 'error');
          return;
        }
        const payload: any = {
          name: ($('#trigger-name').value || '').trim(),
          type: ($('#trigger-type').value || 'webhook').trim(),
          templateId: ($('#trigger-template-id').value || '').trim(),
          createdBy: 'carrier-webui',
          config: {
            inputs: parseTriggerInputsText(($('#trigger-inputs').value || '').trim()),
            provider: ($('#trigger-provider').value || '').trim(),
            hostIds: parseCommaSeparatedValues(($('#trigger-host-ids').value || '').trim()).sort((a, b) => a.localeCompare(b)),
            hostLabels: parseCommaSeparatedValues(($('#trigger-host-labels').value || '').trim()).sort((a, b) => a.localeCompare(b)),
            policyApprove: !!$('#trigger-policy-approve').checked,
            webhookSecret: ($('#trigger-webhook-secret').value || '').trim(),
            githubCommand: ($('#trigger-github-command').value || '').trim(),
            githubLabel: ($('#trigger-github-label').value || '').trim(),
            githubRepository: ($('#trigger-github-repository').value || '').trim(),
            cron: ($('#trigger-cron').value || '').trim(),
            timezone: ($('#trigger-timezone').value || 'UTC').trim(),
          },
        };
        const concurrencyRaw = String(($('#trigger-max-concurrency').value || '')).trim();
        if (concurrencyRaw !== '') {
          payload.config.maxConcurrency = parseInt(concurrencyRaw, 10) || 0;
        }
        if (!payload.name) {
          setMsg('#profiles-msg', 'execution trigger name is required.', 'error');
          return;
        }
        if (!payload.templateId) {
          setMsg('#profiles-msg', 'execution trigger template is required.', 'error');
          return;
        }
        try {
          saveTriggerBtn.disabled = true;
          const editingID = String(triggerEditingTriggerID || '').trim();
          if (editingID) {
            await api('PATCH', '/api/v1/triggers/' + encodeURIComponent(editingID), payload);
            await refreshAll();
            setMsg('#profiles-msg', 'Execution trigger updated: ' + editingID, 'success');
          } else {
            await api('POST', '/api/v1/triggers', payload);
            await refreshAll();
            setMsg('#profiles-msg', 'Execution trigger saved.', 'success');
          }
          resetTriggerEditor(true);
        } catch (e) {
          setMsg('#profiles-msg', 'Save execution trigger failed: ' + e.message, 'error');
        } finally {
          saveTriggerBtn.disabled = false;
        }
      };
    }

    if (governancePreviewResolve) {
      governancePreviewResolve.onclick = async () => {
        if (!featureFlags.providerBindingEnabled) {
          setMsg('#profiles-msg', 'Provider binding is disabled by feature flag.', 'error');
          return;
        }
        const hostID = String((governancePreviewHost && governancePreviewHost.value) || '').trim();
        const agentID = String((governancePreviewAgent && governancePreviewAgent.value) || '').trim();
        if (!hostID) {
          setMsg('#profiles-msg', 'preview host is required.', 'error');
          return;
        }
        try {
          governancePreviewResolve.disabled = true;
          const query = '/api/v1/provider-governance/resolve?hostId=' + encodeURIComponent(hostID) +
            (agentID ? '&agentId=' + encodeURIComponent(agentID) : '');
          const payload = await api('GET', query);
          renderGovernancePreviewResolution(payload);
          setMsg('#profiles-msg', 'Governance resolution loaded.', 'success');
        } catch (e) {
          renderGovernancePreviewResolution(null);
          setMsg('#profiles-msg', 'Governance resolution failed: ' + e.message, 'error');
        } finally {
          governancePreviewResolve.disabled = false;
        }
      };
    }

    if (cancelEditBtn) {
      cancelEditBtn.onclick = () => {
        resetProfileEditor(true);
        setMsg('#profiles-msg', 'Profile edit cancelled.', 'info');
      };
    }
    if (cancelTriggerEditBtn) {
      cancelTriggerEditBtn.onclick = () => {
        resetTriggerEditor(true);
        setMsg('#profiles-msg', 'Trigger edit cancelled.', 'info');
      };
    }
    if (saveProfileBtn) saveProfileBtn.disabled = !canManageProvidersUI();
    if (saveBindingBtn) saveBindingBtn.disabled = !featureFlags.providerBindingEnabled || !canManageProvidersUI();
    if (saveExecutionPolicyBtn) saveExecutionPolicyBtn.disabled = !canManagePoliciesUI();
    if (saveTriggerBtn) saveTriggerBtn.disabled = !canManagePoliciesUI();
    if (profileTestHostSelect && !profileTestHostSelect.options.length) {
      const opt = document.createElement('option');
      opt.value = '';
      opt.textContent = 'auto (first host)';
      profileTestHostSelect.appendChild(opt);
    }
    renderGovernancePreviewResolution(null);
    updateProfileEditorUI();
    updateTriggerEditorUI();
    syncBindingControls();
    syncTriggerControls();
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
    const targetLoadSeq = ++remoteChatTargetsLoadSeq;
    const previousHostID = String(hostSelect.value || '').trim();
    const previousProfileID = String(profileSelect.value || '').trim();
    hostSelect.textContent = '';
    profileSelect.textContent = '';

    const [hosts, profiles] = await Promise.all([fetchRemoteHosts(), fetchProviderProfiles()]);
    if (targetLoadSeq !== remoteChatTargetsLoadSeq) return;
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
      profileSelect.value = previousProfileID || '';
    }

    if (previousHostID) {
      for (let i = 0; i < hostSelect.options.length; i++) {
        if (String(hostSelect.options[i].value || '') === previousHostID) {
          hostSelect.value = previousHostID;
          break;
        }
      }
    }
    if (profileSelect.value !== previousProfileID) {
      for (let i = 0; i < profileSelect.options.length; i++) {
        if (String(profileSelect.options[i].value || '') === previousProfileID) {
          profileSelect.value = previousProfileID;
          break;
        }
      }
    }
  }

  async function loadRemoteChatInstances(hostID) {
    const instanceSelect = $('#remote-chat-instance');
    const instanceLoadSeq = ++remoteChatInstancesLoadSeq;
    instanceSelect.textContent = '';
    if (!hostID) return;
    const payload = await api('GET', '/api/v1/remote/hosts/' + encodeURIComponent(hostID) + '/instances');
    if (instanceLoadSeq !== remoteChatInstancesLoadSeq) return;
    const instances = payload && Array.isArray(payload.instances) ? payload.instances : [];
    const seen = new Set();
    instances.forEach(instance => {
      const opt = document.createElement('option');
      const agentID = instance.agentId || instance.agentID || instance.id || 'main';
      if (seen.has(agentID)) return;
      seen.add(agentID);
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
      refreshTargets();
    };

    targetSelect.onchange();
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

  function formatUSD(value) {
    return '$' + toFiniteNumber(value, 0).toFixed(4);
  }

  function formatMetricsBreakdown(value) {
    const input = value && typeof value === 'object' ? value : {};
    const entries = Object.entries(input)
      .map(([key, count]) => [String(key || '').trim(), toFiniteNumber(count, 0)])
      .filter(([key, count]) => key && count > 0);
    if (!entries.length) return 'none';
    entries.sort((a, b) => {
      if (b[1] !== a[1]) return b[1] - a[1];
      return a[0].localeCompare(b[0]);
    });
    return entries.map(([key, count]) => key + '=' + count).join(', ');
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

  function normalizeOrchestratorMetricsPayload(payload) {
    const metrics = payload && payload.metrics && typeof payload.metrics === 'object' ? payload.metrics : {};
    const executions = metrics.executions && typeof metrics.executions === 'object' ? metrics.executions : {};
    const workers = metrics.workers && typeof metrics.workers === 'object' ? metrics.workers : {};
    const providers = metrics.providers && typeof metrics.providers === 'object' ? metrics.providers : {};
    const policies = metrics.policies && typeof metrics.policies === 'object' ? metrics.policies : {};
    const queue = metrics.queue && typeof metrics.queue === 'object' ? metrics.queue : {};
    return {
      timestamp: String(metrics.timestamp || ''),
      executions: executions,
      workers: workers,
      providers: {
        requestedFailures: providers.requestedFailures && typeof providers.requestedFailures === 'object' ? providers.requestedFailures : {},
        resolvedFailures: providers.resolvedFailures && typeof providers.resolvedFailures === 'object' ? providers.resolvedFailures : {},
        driftStates: providers.driftStates && typeof providers.driftStates === 'object' ? providers.driftStates : {},
        attribution: providers.attribution && typeof providers.attribution === 'object' ? providers.attribution : {},
        totalEstimatedCostUsd: toFiniteNumber(providers.totalEstimatedCostUsd, 0),
        aggregates: Array.isArray(providers.aggregates) ? providers.aggregates : [],
        models: Array.isArray(providers.models) ? providers.models : [],
      },
      policies: policies,
      queue: queue,
    };
  }

  function topProviderUsage(providerMetrics) {
    const aggregates = providerMetrics && Array.isArray(providerMetrics.aggregates) ? providerMetrics.aggregates : [];
    if (!aggregates.length) return '';
    const ordered = aggregates
      .map(item => ({
        provider: String(item && item.provider ? item.provider : '').trim(),
        estimatedCostUsd: toFiniteNumber(item && item.estimatedCostUsd, 0),
        total: toFiniteNumber(item && item.successes, 0) + toFiniteNumber(item && item.failures, 0),
      }))
      .filter(item => item.provider);
    if (!ordered.length) return '';
    ordered.sort((a, b) => {
      if (b.estimatedCostUsd !== a.estimatedCostUsd) return b.estimatedCostUsd - a.estimatedCostUsd;
      if (b.total !== a.total) return b.total - a.total;
      return a.provider.localeCompare(b.provider);
    });
    return ordered[0].provider;
  }

  function topModelUsage(providerMetrics) {
    const models = providerMetrics && Array.isArray(providerMetrics.models) ? providerMetrics.models : [];
    if (!models.length) return '';
    const ordered = models
      .map(item => ({
        provider: String(item && item.provider ? item.provider : '').trim(),
        model: String(item && item.model ? item.model : '').trim(),
        estimatedCostUsd: toFiniteNumber(item && item.estimatedCostUsd, 0),
        total: toFiniteNumber(item && item.successes, 0) + toFiniteNumber(item && item.failures, 0),
      }))
      .filter(item => item.model);
    if (!ordered.length) return '';
    ordered.sort((a, b) => {
      if (b.estimatedCostUsd !== a.estimatedCostUsd) return b.estimatedCostUsd - a.estimatedCostUsd;
      if (b.total !== a.total) return b.total - a.total;
      const left = [a.provider, a.model].filter(Boolean).join('/');
      const right = [b.provider, b.model].filter(Boolean).join('/');
      return left.localeCompare(right);
    });
    return ordered[0].model;
  }

  function topUsageAttribution(providerMetrics, key) {
    const attribution = providerMetrics && providerMetrics.attribution && typeof providerMetrics.attribution === 'object'
      ? providerMetrics.attribution
      : {};
    const items = Array.isArray(attribution[key]) ? attribution[key] : [];
    if (!items.length) return '';
    const ordered = items
      .map(item => ({
        label: String(item && item.label ? item.label : '').trim(),
        estimatedCostUsd: toFiniteNumber(item && item.estimatedCostUsd, 0),
        executions: toFiniteNumber(item && item.executions, 0),
      }))
      .filter(item => item.label);
    if (!ordered.length) return '';
    ordered.sort((a, b) => {
      if (b.estimatedCostUsd !== a.estimatedCostUsd) return b.estimatedCostUsd - a.estimatedCostUsd;
      if (b.executions !== a.executions) return b.executions - a.executions;
      return a.label.localeCompare(b.label);
    });
    return ordered[0].label;
  }

  function buildRemoteObservabilityExecutionLinkLine(label, value, href) {
    const text = label + ': ' + (value || 'none');
    if (!value || !href) return text;
    return {
      text: text,
      href: href,
    };
  }

  function buildExecutionTriggerRouteParams(label) {
    const value = String(label || '').trim();
    if (!value) return {};
    const separator = value.indexOf(':');
    if (separator <= 0) {
      return { trigger: value.toLowerCase(), search: value };
    }
    const source = value.slice(0, separator).trim().toLowerCase();
    return {
      trigger: source,
      search: value,
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

  function renderRemoteObservabilitySummary(normalized, orchestrator) {
    const wrap = $('#remote-observability-summary');
    if (!wrap) return;

    const totals = normalized.totals || {};
    const repair = normalized.repair || {};
    const chat = normalized.chatStream || {};
    const rollout = normalized.rollout || {};
    const executionMetrics = orchestrator && orchestrator.executions ? orchestrator.executions : {};
    const workerMetrics = orchestrator && orchestrator.workers ? orchestrator.workers : {};
    const providerMetrics = orchestrator && orchestrator.providers ? orchestrator.providers : {};
    const policyMetrics = orchestrator && orchestrator.policies ? orchestrator.policies : {};
    const queueMetrics = orchestrator && orchestrator.queue ? orchestrator.queue : {};
    const rolloutState = String(rollout.state || 'unknown').trim().toLowerCase() || 'unknown';
    const rolloutReasons = Array.isArray(rollout.reasons) ? rollout.reasons : [];
    const topTeam = topUsageAttribution(providerMetrics, 'teams');
    const topProject = topUsageAttribution(providerMetrics, 'projects');
    const topTemplate = topUsageAttribution(providerMetrics, 'templates');
    const topTrigger = topUsageAttribution(providerMetrics, 'triggers');
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
      {
        title: 'Executions',
        lines: [
          'total: ' + toFiniteNumber(executionMetrics.total, 0),
          'running: ' + toFiniteNumber(executionMetrics.running, 0),
          'completed: ' + toFiniteNumber(executionMetrics.completed, 0),
          'failed: ' + toFiniteNumber(executionMetrics.failed, 0),
          'retry count: ' + toFiniteNumber(executionMetrics.retryCount, 0),
          'avg latency: ' + formatMilliseconds(executionMetrics.avgLatencyMs),
        ],
      },
      {
        title: 'Workers',
        lines: [
          'total: ' + toFiniteNumber(workerMetrics.total, 0),
          'busy: ' + toFiniteNumber(workerMetrics.busy, 0),
          'ready: ' + toFiniteNumber(workerMetrics.ready, 0),
          'error: ' + toFiniteNumber(workerMetrics.error, 0),
          'stale: ' + toFiniteNumber(workerMetrics.stale, 0),
          'queued tasks: ' + toFiniteNumber(queueMetrics.queuedTasks, 0),
        ],
      },
      {
        title: 'Provider Failures',
        lines: [
          'requested: ' + formatMetricsBreakdown(providerMetrics.requestedFailures),
          'resolved: ' + formatMetricsBreakdown(providerMetrics.resolvedFailures),
        ],
      },
      {
        title: 'Provider Usage',
        lines: [
          'estimated cost: ' + formatUSD(providerMetrics.totalEstimatedCostUsd),
          'top provider: ' + (topProviderUsage(providerMetrics) || 'none'),
          'top model: ' + (topModelUsage(providerMetrics) || 'none'),
          'drift: ' + formatMetricsBreakdown(providerMetrics.driftStates),
        ],
      },
      {
        title: 'Cost Attribution',
        lines: [
          buildRemoteObservabilityExecutionLinkLine('top team', topTeam, topTeam ? buildExecutionsHash({ search: topTeam }) : ''),
          buildRemoteObservabilityExecutionLinkLine('top project', topProject, topProject ? buildExecutionsHash({ search: topProject }) : ''),
          buildRemoteObservabilityExecutionLinkLine('top template', topTemplate, topTemplate ? buildExecutionsHash({ template: topTemplate }) : ''),
          buildRemoteObservabilityExecutionLinkLine('top trigger', topTrigger, topTrigger ? buildExecutionsHash(buildExecutionTriggerRouteParams(topTrigger)) : ''),
        ],
      },
      {
        title: 'Policy Blocks',
        lines: [
          'allow: ' + toFiniteNumber(policyMetrics.allow, 0),
          'ask: ' + toFiniteNumber(policyMetrics.ask, 0),
          'deny: ' + toFiniteNumber(policyMetrics.deny, 0),
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
        const row = document.createElement(line && typeof line === 'object' && line.href ? 'a' : 'div');
        row.className = 'instance-meta';
        if (line && typeof line === 'object') {
          row.textContent = String(line.text || '');
          if (line.href) {
            row.classList.add('summary-link');
            row.href = String(line.href);
          }
        } else {
          row.textContent = line;
        }
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
      renderRemoteObservabilitySummary(remoteObservabilityData, orchestratorObservabilityData);
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
      const executions = orchestratorObservabilityData && orchestratorObservabilityData.executions ? orchestratorObservabilityData.executions : {};
      const workers = orchestratorObservabilityData && orchestratorObservabilityData.workers ? orchestratorObservabilityData.workers : {};
      if (toFiniteNumber(executions.total, 0) > 0) {
        parts.push('executions=' + toFiniteNumber(executions.total, 0));
      }
      if (toFiniteNumber(workers.stale, 0) > 0) {
        parts.push('stale_workers=' + toFiniteNumber(workers.stale, 0));
      }
      status.textContent = parts.join(' · ');
    }

    async function refreshMetrics() {
      status.textContent = 'Loading remote metrics...';
      const results = await Promise.allSettled([
        api('GET', '/api/v1/remote/metrics'),
        api('GET', '/api/v1/orchestrator/metrics'),
      ]);
      const errors = [];
      if (results[0].status === 'fulfilled') {
        remoteObservabilityData = normalizeRemoteMetricsPayload(results[0].value);
      } else {
        remoteObservabilityData = normalizeRemoteMetricsPayload({});
        errors.push('remote=' + results[0].reason.message);
      }
      if (results[1].status === 'fulfilled') {
        orchestratorObservabilityData = normalizeOrchestratorMetricsPayload(results[1].value);
      } else {
        orchestratorObservabilityData = normalizeOrchestratorMetricsPayload({});
        errors.push('orchestrator=' + results[1].reason.message);
      }
      renderFromCache();
      if (errors.length) {
        status.textContent += ' · warnings=' + errors.join('; ');
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
              connectDelegateEvents();
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
              connectDelegateEvents();
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
