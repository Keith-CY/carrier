import { useQuery } from '@tanstack/react-query';
import { useFeatures } from '../../app/useFeatures';
import { apiGet } from '../../lib/api';
import { formatPercent, toFiniteNumber } from '../../lib/format';
import { useSession } from '../../app/session';

function buildSettingsSummary(
  transportPayload: any,
  remoteMetricsPayload: any,
  remoteControlPlaneEnabled: boolean,
) {
  const lines = ['Daemon mode - provider settings managed via config.json.'];
  const transport = transportPayload?.transport && typeof transportPayload.transport === 'object'
    ? transportPayload.transport
    : null;

  if (transport && transport.selected_mode) {
    let summary = `Telegram transport: ${String(transport.selected_mode)}`;
    if (transport.reason_code) summary += ` (reason: ${String(transport.reason_code)})`;
    lines.push(summary);
    if (transport.hint) lines.push(`Hint: ${String(transport.hint)}`);
  } else {
    lines.push('Telegram transport status unavailable.');
  }

  if (!remoteControlPlaneEnabled) {
    lines.push('Remote control plane disabled by feature flag.');
    return lines.join(' ');
  }

  const metrics = remoteMetricsPayload?.metrics && typeof remoteMetricsPayload.metrics === 'object'
    ? remoteMetricsPayload.metrics
    : null;
  if (!metrics) {
    lines.push('Remote metrics unavailable.');
    return lines.join(' ');
  }

  const totals = metrics.totals || {};
  const repair = metrics.repair || {};
  const chat = metrics.chatStream || {};
  const rollout = metrics.rollout || {};
  const rolloutState = String(rollout.state || 'unknown');
  const rolloutReasons = Array.isArray(rollout.reasons) ? rollout.reasons : [];
  const totalOps = toFiniteNumber(totals.total, 0);

  lines.push(
    `Remote metrics: ops=${totalOps}, op success rate=${formatPercent(totals.successRate)}, repair triggered=${toFiniteNumber(repair.triggered, 0)}, repair success=${formatPercent(repair.successRate)}, chat stream failure=${formatPercent(chat.failureRate)}.`,
  );
  lines.push(
    `Remote rollout gate: state=${rolloutState}, can promote=${rollout.canPromote ? 'yes' : 'no'}${rolloutReasons.length ? `, reasons=${rolloutReasons.join('; ')}` : ''}.`,
  );

  return lines.join(' ');
}

export function SettingsPage() {
  const { featureFlags } = useFeatures();
  const { logout } = useSession();

  const transportQuery = useQuery({
    queryKey: ['settings-transport'],
    queryFn: () => apiGet<any>('/api/v1/telegram/transport'),
    retry: false,
  });

  const remoteMetricsQuery = useQuery({
    queryKey: ['settings-remote-metrics'],
    queryFn: () => apiGet<any>('/api/v1/remote/metrics'),
    enabled: featureFlags.remoteControlPlaneEnabled,
    retry: false,
  });

  const summary = buildSettingsSummary(
    transportQuery.isError ? null : transportQuery.data,
    remoteMetricsQuery.isError ? null : remoteMetricsQuery.data,
    featureFlags.remoteControlPlaneEnabled,
  );

  return (
    <section id="view-settings" className="view">
      <h2>Settings</h2>
      <div className="card">
        <h3>Provider Configuration</h3>
        <div id="settings-provider" className="text-dim">
          {summary}
        </div>
      </div>
      <div className="card" style={{ marginTop: '1rem' }}>
        <h3>Gateway</h3>
        <p className="text-dim">Token: ••••••</p>
        <button id="settings-logout" className="btn-sm btn-danger" type="button" onClick={logout}>
          Clear Token &amp; Logout
        </button>
      </div>
    </section>
  );
}
