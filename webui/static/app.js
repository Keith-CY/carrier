// Carrier WebUI — vanilla JS, no external dependencies
(function () {
  'use strict';

  const $ = (s, p) => (p || document).querySelector(s);
  const $$ = (s, p) => [...(p || document).querySelectorAll(s)];

  // --- State ---
  let token = localStorage.getItem('carrier_token') || '';
  let selectedAgent = '';
  let logSource = null; // EventSource for SSE logs

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
    if (token) opts.headers['X-Gateway-Token'] = token;
    if (body) opts.body = JSON.stringify(body);
    return fetch(path, opts).then(r => {
      if (r.status === 401) {
        clearToken();
        throw new Error('Unauthorized');
      }
      return r.json();
    });
  }

  function command(input, args) {
    return api('POST', '/command', { input: input, args: args || [] });
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

  // --- Health ---
  function checkHealth() {
    const opts = { headers: {} };
    if (token) opts.headers['X-Gateway-Token'] = token;
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
    'welcome', 'setup', 'agents', 'config', 'install', 'complete',
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

    // Management views require auth
    const mgmtRoutes = ['dashboard', 'logs', 'chat', 'settings'];
    if (mgmtRoutes.includes(route)) {
      $('#nav').classList.remove('hidden');
    }

    switch (route) {
      case 'welcome': initWelcome(); break;
      case 'setup': initSetup(); break;
      case 'agents': initAgents(); break;
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
          headers: { 'X-Gateway-Token': t },
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
    showView('welcome');
    const status = $('#welcome-status');
    const btn = $('#welcome-continue');
    status.textContent = '';
    btn.classList.add('hidden');

    checkHealth();
    fetch('/healthz', { headers: token ? { 'X-Gateway-Token': token } : {} })
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
    renderSteps('#steps-indicator', 0, 5);

    $('#setup-btn').onclick = async () => {
      const provider = $('#provider').value;
      const provToken = $('#provider-token').value.trim();
      const secret = $('#webhook-secret').value.trim();
      if (!provider) { setMsg('#setup-msg', 'Please select a provider.', 'error'); return; }
      if (!provToken) { setMsg('#setup-msg', 'Token is required.', 'error'); return; }
      try {
        const res = await api('POST', '/api/v1/setup', {
          provider, token: provToken, webhook_secret: secret,
        });
        if (res.result === 'ok' || res.status === 'ok') {
          location.hash = '#/agents';
        } else {
          setMsg('#setup-msg', res.message || 'Setup failed.', 'error');
        }
      } catch (e) {
        setMsg('#setup-msg', 'Error: ' + e.message, 'error');
      }
    };
  }

  // --- Agents selection ---
  async function initAgents() {
    showView('agents');
    renderSteps('#steps-indicator-2', 1, 5);

    const list = $('#agent-pick');
    list.textContent = '';
    setMsg('#agents-msg', 'Loading agents…', 'info');

    try {
      const res = await command('onboard', []);
      const text = res.text || res.message || '';
      const agents = parseAgentList(text);
      setMsg('#agents-msg', '', 'info');

      agents.forEach(a => {
        const li = document.createElement('li');
        li.textContent = a;
        li.onclick = () => {
          $$('li', list).forEach(x => x.classList.remove('selected'));
          li.classList.add('selected');
          selectedAgent = a;
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
      try {
        await command('onboard', [selectedAgent]);
        location.hash = '#/config';
      } catch (e) {
        setMsg('#agents-msg', 'Error: ' + e.message, 'error');
      }
    };
  }

  function parseAgentList(text) {
    const lines = text.split('\n').filter(l => l.trim());
    const result = [];
    for (const line of lines) {
      const m = line.match(/^\s*[-•*]\s*(\S+)/);
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

  // --- Config ---
  function initConfig() {
    showView('config');
    renderSteps('#steps-indicator-3', 2, 5);

    $('#config-agent-name').textContent = 'Configuring: ' + selectedAgent;
    const fields = $('#env-fields');
    fields.textContent = '';
    addEnvRow();

    $('#add-env').onclick = addEnvRow;
    $('#config-back').onclick = () => { location.hash = '#/agents'; };
    $('#config-next').onclick = async () => {
      try {
        // Send env vars
        const rows = $$('.env-row');
        for (const row of rows) {
          const inputs = $$('input', row);
          const k = inputs[0].value.trim();
          const v = inputs[1].value.trim();
          if (k) await command('onboard', ['env', k + '=' + v]);
        }
        await command('onboard', ['done']);
        location.hash = '#/install';
      } catch (e) {
        setMsg('#config-msg', 'Error: ' + e.message, 'error');
      }
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
    renderSteps('#steps-indicator-4', 3, 5);

    $('#install-summary').textContent = 'Agent: ' + selectedAgent;

    $('#install-back').onclick = () => { location.hash = '#/config'; };
    $('#install-confirm').onclick = async () => {
      setMsg('#install-msg', 'Installing…', 'info');
      try {
        await command('onboard', ['yes']);
        location.hash = '#/complete';
      } catch (e) {
        setMsg('#install-msg', 'Error: ' + e.message, 'error');
      }
    };
  }

  // --- Complete ---
  function initComplete() {
    showView('complete');
    $('#complete-dashboard').onclick = () => { location.hash = '#/dashboard'; };
  }

  // --- Dashboard ---
  async function initDashboard() {
    showView('dashboard');
    $('#nav').classList.remove('hidden');
    await refreshAgents();
    $('#refresh-agents').onclick = refreshAgents;
  }

  async function refreshAgents() {
    const el = $('#agent-list');
    el.textContent = 'Loading…';
    try {
      const res = await command('onboard', ['status']);
      const text = res.text || res.message || '';
      const agents = parseAgentStatus(text);

      el.textContent = '';
      if (agents.length === 0) {
        el.textContent = 'No agents found.';
        return;
      }

      agents.forEach(a => {
        const card = document.createElement('div');
        card.className = 'agent-card';
        card.onclick = () => { location.hash = '#/agents/' + encodeURIComponent(a.name); };

        const h = document.createElement('h4');
        h.textContent = a.name;

        const status = document.createElement('div');
        status.className = 'agent-status';
        status.textContent = statusIcon(a.status) + ' ' + a.status;

        const btnRow = document.createElement('div');
        btnRow.className = 'btn-row';

        const startBtn = document.createElement('button');
        startBtn.className = 'btn-sm';
        startBtn.textContent = '▶ Start';
        startBtn.onclick = (e) => { e.stopPropagation(); agentAction(a.name, 'start'); };

        const stopBtn = document.createElement('button');
        stopBtn.className = 'btn-sm btn-secondary';
        stopBtn.textContent = '⏹ Stop';
        stopBtn.onclick = (e) => { e.stopPropagation(); agentAction(a.name, 'stop'); };

        btnRow.appendChild(startBtn);
        btnRow.appendChild(stopBtn);
        card.appendChild(h);
        card.appendChild(status);
        card.appendChild(btnRow);
        el.appendChild(card);
      });
    } catch (e) {
      el.textContent = 'Error: ' + e.message;
    }
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
      await command('onboard', [action, name]);
      await refreshAgents();
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
      const res = await command('onboard', ['status']);
      const text = res.text || res.message || '';
      el.textContent = '';

      const card = document.createElement('div');
      card.className = 'card';

      const h = document.createElement('h3');
      h.textContent = 'Agent: ' + id;
      card.appendChild(h);

      const pre = document.createElement('pre');
      pre.className = 'log-box';
      pre.textContent = text;
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
    const agentSelect = $('#log-agent');
    agentSelect.textContent = '';

    // Populate agent list
    command('onboard', ['status']).then(res => {
      const text = res.text || res.message || '';
      const agents = parseAgentStatus(text);
      agents.forEach(a => {
        const opt = document.createElement('option');
        opt.value = a.name;
        opt.textContent = a.name;
        agentSelect.appendChild(opt);
      });
    }).catch(() => {});

    $('#log-clear').onclick = () => {
      $('#log-output').textContent = '';
    };

    $('#log-connect').onclick = () => {
      connectLogs(agentSelect.value);
    };
  }

  function connectLogs(agentId) {
    if (logSource) {
      logSource.close();
      logSource = null;
    }

    const output = $('#log-output');
    output.textContent = 'Connecting to logs for ' + agentId + '…\n';

    // Try SSE first
    const sseUrl = '/api/v1/logs/stream?agent=' + encodeURIComponent(agentId);
    try {
      const es = new EventSource(sseUrl);
      logSource = es;

      es.onmessage = (e) => {
        output.textContent += e.data + '\n';
        output.scrollTop = output.scrollHeight;
      };

      es.onerror = () => {
        es.close();
        logSource = null;
        output.textContent += '\n[SSE disconnected, falling back to polling]\n';
        pollLogs(agentId);
      };
    } catch (e) {
      pollLogs(agentId);
    }
  }

  function pollLogs(agentId) {
    const output = $('#log-output');
    let running = true;

    // Store cancel function
    const cancel = () => { running = false; };
    logSource = { close: cancel };

    const poll = async () => {
      if (!running) return;
      try {
        const res = await command('onboard', ['logs', agentId]);
        const text = res.text || res.message || '';
        if (text) {
          output.textContent += text + '\n';
          output.scrollTop = output.scrollHeight;
        }
      } catch (e) {
        output.textContent += '[poll error: ' + e.message + ']\n';
      }
      if (running) setTimeout(poll, 2000);
    };
    poll();
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
      api('POST', '/command', { input: text })
        .then(res => {
          appendChat('Carrier', res.text || res.message || JSON.stringify(res));
        })
        .catch(e => {
          appendChat('Error', e.message);
        });
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
    try {
      const res = await api('GET', '/api/v1/setup');
      if (res.provider) {
        el.textContent = 'Provider: ' + res.provider + (res.configured ? ' (configured)' : '');
      } else {
        el.textContent = 'No provider configured.';
      }
    } catch (e) {
      el.textContent = 'Could not load settings.';
    }
  }

  // --- Init ---
  function init() {
    initLogin();
    checkHealth();
    setInterval(checkHealth, 30000);

    window.addEventListener('hashchange', () => navigate(location.hash));

    // Check if token is valid
    if (token) {
      fetch('/healthz', { headers: { 'X-Gateway-Token': token } })
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
