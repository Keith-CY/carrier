import { formatPercent, toFiniteNumber } from '../../lib/format';

export function buildSettingsSummary(
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
