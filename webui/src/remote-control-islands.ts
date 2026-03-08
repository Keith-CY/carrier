(function () {
  'use strict';

  const React = window.React;
  const ReactDOM = window.ReactDOM;
  if (!React || !ReactDOM || typeof ReactDOM.createRoot !== 'function') {
    return;
  }

  const h = React.createElement;
  const roots = new WeakMap();

  function getRoot(container) {
    if (!container) return null;
    let root = roots.get(container);
    if (!root) {
      root = ReactDOM.createRoot(container);
      roots.set(container, root);
    }
    return root;
  }

  function hostEndpoint(host) {
    if (host && host.authMode === 'ssh_config') {
      return host.sshConfigHost || host.host || '-';
    }
    const user = host && host.user ? host.user : 'user';
    const hostname = host && host.host ? host.host : '-';
    return user + '@' + hostname;
  }

  function hostMetaText(host, hostOperation) {
    const keyRef = host && host.keyRef ? String(host.keyRef) : '';
    const keyPath = host && host.keyPath ? String(host.keyPath) : '';
    const labels = host && Array.isArray(host.labels) ? host.labels.map((item) => String(item || '').trim()).filter(Boolean) : [];
    const lines = [
      'id: ' + String(host && host.id ? host.id : '-'),
      'endpoint: ' + hostEndpoint(host),
      'auth: ' + String(host && host.authMode ? host.authMode : '-'),
      'key: ' + (keyRef ? ('uploaded:' + keyRef) : (keyPath || '-')),
      'runtime: ' + String(host && host.runtimeMode ? host.runtimeMode : '-'),
      'labels: ' + (labels.length ? labels.join(', ') : '-'),
      'health: ' + String(host && host.lastHealth ? host.lastHealth : 'unknown'),
    ];
    if (hostOperation && typeof hostOperation === 'object') {
      if (hostOperation.operation) {
        lines.push('last op: ' + String(hostOperation.operation) + ' (' + String(hostOperation.success ? 'ok' : 'error') + ')');
      }
      if (hostOperation.requestId) {
        lines.push('requestId: ' + String(hostOperation.requestId));
      }
      if (hostOperation.durationMs != null) {
        const duration = Math.round(Number(hostOperation.durationMs) || 0);
        lines.push('duration: ' + String(duration) + 'ms');
      }
      if (!hostOperation.success && hostOperation.error) {
        lines.push('last error: ' + String(hostOperation.error));
      }
      if (hostOperation.at) {
        lines.push('updated at: ' + String(hostOperation.at));
      }
    }
    return lines.join('\n');
  }

  function profileMetaText(profile) {
    return [
      'id: ' + String(profile && profile.id ? profile.id : '-'),
      'provider/model: ' + String(profile && profile.provider ? profile.provider : '-') + '/' + String(profile && profile.model ? profile.model : '-'),
      'enabled: ' + String(!!(profile && profile.enabled)),
    ].join('\n');
  }

  function bindingMetaText(binding) {
    return [
      'id: ' + String(binding && binding.id ? binding.id : '-'),
      'profileId: ' + String(binding && binding.profileId ? binding.profileId : '-'),
      'syncMode: ' + String(binding && binding.syncMode ? binding.syncMode : 'always_push'),
    ].join('\n');
  }

  function ServersList(props) {
    const hosts = Array.isArray(props.hosts) ? props.hosts : [];
    if (!hosts.length) {
      return h('div', { className: 'card' }, 'No remote servers configured.');
    }

    const onCheck = typeof props.onCheck === 'function' ? props.onCheck : function () {};
    const onDelete = typeof props.onDelete === 'function' ? props.onDelete : function () {};
    const onManage = typeof props.onManage === 'function' ? props.onManage : function () {};
    const onEdit = typeof props.onEdit === 'function' ? props.onEdit : function () {};
    const hostOperations = props && props.hostOperations && typeof props.hostOperations === 'object' ? props.hostOperations : {};

    return h(
      React.Fragment,
      null,
      hosts.map((host, index) => {
        const hostID = String(host && host.id ? host.id : 'host-' + String(index));
        const hostOperation = hostOperations[hostID];
        return h(
          'div',
          { className: 'agent-card', key: hostID },
          [
            h('h4', { key: 'title' }, String(host && (host.name || host.id) ? (host.name || host.id) : hostID)),
            h('div', { className: 'instance-meta', style: { whiteSpace: 'pre-line' }, key: 'meta' }, hostMetaText(host, hostOperation)),
            h(
              'div',
              { className: 'btn-row', key: 'actions' },
              [
                h(
                  'button',
                  {
                    className: 'btn-sm btn-secondary',
                    key: 'check',
                    onClick: function () {
                      onCheck(hostID);
                    },
                  },
                  'Check',
                ),
                h(
                  'button',
                  {
                    className: 'btn-sm',
                    key: 'manage',
                    onClick: function () {
                      onManage(hostID);
                    },
                  },
                  'Manage',
                ),
                h(
                  'button',
                  {
                    className: 'btn-sm btn-secondary',
                    key: 'edit',
                    onClick: function () {
                      onEdit(hostID);
                    },
                  },
                  'Edit',
                ),
                h(
                  'button',
                  {
                    className: 'btn-sm btn-danger',
                    key: 'delete',
                    onClick: function () {
                      onDelete(hostID);
                    },
                  },
                  'Delete',
                ),
              ],
            ),
          ],
        );
      }),
    );
  }

  function ProfilesList(props) {
    const profiles = Array.isArray(props.profiles) ? props.profiles : [];
    const onTestProfile = typeof props.onTestProfile === 'function' ? props.onTestProfile : function () {};
    const onDeleteProfile = typeof props.onDeleteProfile === 'function' ? props.onDeleteProfile : function () {};
    const onEditProfile = typeof props.onEditProfile === 'function' ? props.onEditProfile : function () {};

    if (!profiles.length) {
      return h('div', { className: 'card' }, 'No provider profiles configured.');
    }

    return h(
      React.Fragment,
      null,
      profiles.map((profile, index) => {
        const profileID = String(profile && profile.id ? profile.id : 'profile-' + String(index));
        return h(
          'div',
          { className: 'agent-card', key: profileID },
          [
            h('h4', { key: 'title' }, String(profile && (profile.name || profile.id) ? (profile.name || profile.id) : profileID)),
            h('div', { className: 'instance-meta', style: { whiteSpace: 'pre-line' }, key: 'meta' }, profileMetaText(profile)),
            h(
              'div',
              { className: 'btn-row', key: 'actions' },
              [
                h(
                  'button',
                  {
                    className: 'btn-sm btn-secondary',
                    key: 'test',
                    onClick: function () {
                      onTestProfile(profileID);
                    },
                  },
                  'Test',
                ),
                h(
                  'button',
                  {
                    className: 'btn-sm btn-secondary',
                    key: 'edit',
                    onClick: function () {
                      onEditProfile(profileID);
                    },
                  },
                  'Edit',
                ),
                h(
                  'button',
                  {
                    className: 'btn-sm btn-danger',
                    key: 'delete',
                    onClick: function () {
                      onDeleteProfile(profileID);
                    },
                  },
                  'Delete',
                ),
              ],
            ),
          ],
        );
      }),
    );
  }

  function BindingsList(props) {
    const bindings = Array.isArray(props.bindings) ? props.bindings : [];
    const onDeleteBinding = typeof props.onDeleteBinding === 'function' ? props.onDeleteBinding : function () {};
    if (!bindings.length) {
      return h('div', { className: 'card' }, 'No provider bindings configured.');
    }

    return h(
      React.Fragment,
      null,
      bindings.map((binding, index) => {
        const bindingID = String(binding && binding.id ? binding.id : 'binding-' + String(index));
        return h(
          'div',
          { className: 'agent-card', key: bindingID },
          [
            h('h4', { key: 'title' }, String(binding && binding.targetType ? binding.targetType : '-') + ': ' + String(binding && binding.targetId ? binding.targetId : '-')),
            h('div', { className: 'instance-meta', style: { whiteSpace: 'pre-line' }, key: 'meta' }, bindingMetaText(binding)),
            h(
              'div',
              { className: 'btn-row', key: 'actions' },
              [
                h(
                  'button',
                  {
                    className: 'btn-sm btn-danger',
                    key: 'delete',
                    onClick: function () {
                      onDeleteBinding(bindingID);
                    },
                  },
                  'Delete',
                ),
              ],
            ),
          ],
        );
      }),
    );
  }

  function renderServersList(container, hosts, actions, hostOperations) {
    const root = getRoot(container);
    if (!root) return false;
    root.render(
      h(ServersList, {
        hosts: hosts,
        hostOperations: hostOperations,
        onCheck: actions && actions.onCheck,
        onManage: actions && actions.onManage,
        onEdit: actions && actions.onEdit,
        onDelete: actions && actions.onDelete,
      }),
    );
    return true;
  }

  function renderProfilesAndBindings(profilesContainer, bindingsContainer, profiles, bindings, actions) {
    const profilesRoot = getRoot(profilesContainer);
    const bindingsRoot = getRoot(bindingsContainer);
    if (!profilesRoot || !bindingsRoot) return false;

    profilesRoot.render(
      h(ProfilesList, {
        profiles: profiles,
        onTestProfile: actions && actions.onTestProfile,
        onEditProfile: actions && actions.onEditProfile,
        onDeleteProfile: actions && actions.onDeleteProfile,
      }),
    );
    bindingsRoot.render(
      h(BindingsList, {
        bindings: bindings,
        onDeleteBinding: actions && actions.onDeleteBinding,
      }),
    );
    return true;
  }

  window.CarrierRemoteControlIslands = {
    renderServersList,
    renderProfilesAndBindings,
  };
})();
