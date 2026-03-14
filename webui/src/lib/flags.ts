export type FeatureFlags = {
  remoteControlPlaneEnabled: boolean;
  remoteChatEnabled: boolean;
  providerBindingEnabled: boolean;
};

export type AuthzState = {
  role: string;
  permissions: {
    viewExecutions: boolean;
    launchExecutions: boolean;
    approveExecutions: boolean;
    managePolicies: boolean;
    manageProviders: boolean;
    manageHosts: boolean;
  };
};

export const DEFAULT_FEATURE_FLAGS: FeatureFlags = {
  remoteControlPlaneEnabled: false,
  remoteChatEnabled: false,
  providerBindingEnabled: false,
};

export const DEFAULT_AUTHZ: AuthzState = {
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

function toBool(value: unknown, fallback: boolean): boolean {
  if (typeof value === 'boolean') return value;
  if (typeof value === 'string') {
    const normalized = value.trim().toLowerCase();
    if (['true', '1', 'yes', 'on'].includes(normalized)) return true;
    if (['false', '0', 'no', 'off'].includes(normalized)) return false;
  }
  if (typeof value === 'number') return value !== 0;
  return fallback;
}

export function normalizeFeatureFlags(payload: any): FeatureFlags {
  const source = payload && typeof payload === 'object' && payload.features && typeof payload.features === 'object'
    ? payload.features
    : payload;
  return {
    remoteControlPlaneEnabled: toBool(source?.remoteControlPlaneEnabled, DEFAULT_FEATURE_FLAGS.remoteControlPlaneEnabled),
    remoteChatEnabled: toBool(source?.remoteChatEnabled, DEFAULT_FEATURE_FLAGS.remoteChatEnabled),
    providerBindingEnabled: toBool(source?.providerBindingEnabled, DEFAULT_FEATURE_FLAGS.providerBindingEnabled),
  };
}

export function normalizeAuthz(payload: any): AuthzState {
  const source = payload && typeof payload === 'object' && payload.authz && typeof payload.authz === 'object'
    ? payload.authz
    : {};
  const permissions = source.permissions && typeof source.permissions === 'object' ? source.permissions : {};
  return {
    role: String(source.role || DEFAULT_AUTHZ.role).trim().toLowerCase() || DEFAULT_AUTHZ.role,
    permissions: {
      viewExecutions: toBool(permissions.viewExecutions, DEFAULT_AUTHZ.permissions.viewExecutions),
      launchExecutions: toBool(permissions.launchExecutions, DEFAULT_AUTHZ.permissions.launchExecutions),
      approveExecutions: toBool(permissions.approveExecutions, DEFAULT_AUTHZ.permissions.approveExecutions),
      managePolicies: toBool(permissions.managePolicies, DEFAULT_AUTHZ.permissions.managePolicies),
      manageProviders: toBool(permissions.manageProviders, DEFAULT_AUTHZ.permissions.manageProviders),
      manageHosts: toBool(permissions.manageHosts, DEFAULT_AUTHZ.permissions.manageHosts),
    },
  };
}
