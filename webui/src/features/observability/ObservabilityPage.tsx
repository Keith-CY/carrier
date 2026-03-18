import type { ReactNode } from 'react';
import { PageShell } from '../../app/page-shell';
import {
  formatMetricsBreakdown,
  formatMilliseconds,
  formatPercent,
  formatUSD,
  toFiniteNumber,
} from '../../lib/format';
import { useObservabilityData } from './useObservabilityData';

type SummaryCardProps = {
  title: string;
  lines: ReactNode[];
};

function SummaryCard({ title, lines }: SummaryCardProps) {
  return (
    <div className="agent-card" style={{ scrollMarginTop: '96px' }}>
      <h4>{title}</h4>
      <div className="instance-meta">
        {lines.map((line, index) => (
          <div key={`${title}-${index}`}>{line}</div>
        ))}
      </div>
    </div>
  );
}

export function ObservabilityPage() {
  const {
    featureFlags,
    authz,
    group,
    setGroup,
    anomaliesOnly,
    setAnomaliesOnly,
    remoteMetricsQuery,
    orchestratorMetricsQuery,
    refreshMutation,
    remoteMetrics,
    orchestratorMetrics,
    providerMetrics,
    policyMetrics,
    operationGroups,
    visibleOperations,
    topTeam,
    topProject,
    topTemplate,
    topTrigger,
    topProvider,
    topModel,
    statusLine,
    drillDown,
    drillDownHref,
  } = useObservabilityData();

  return (
    <PageShell
      id="view-remote-observability"
      eyebrow="Observe"
      title="Observability"
      description="Read remote metrics, rollout posture, provider drift, and cost attribution without dropping into raw telemetry first."
      actions={(
        <button
          id="remote-observability-refresh"
          className="btn-sm btn-secondary"
          onClick={() => refreshMutation.mutate()}
        >
          Refresh
        </button>
      )}
      stats={[
        { label: 'Operation Group', value: group },
        { label: 'Visible Ops', value: String(visibleOperations.length) },
        { label: 'Top Provider', value: topProvider },
        { label: 'Top Model', value: topModel },
      ]}
    >
      <div className="form-grid remote-observability-controls">
        <div>
          <label htmlFor="remote-observability-group">Operation Group</label>
          <select id="remote-observability-group" value={group} onChange={(event) => setGroup(event.target.value)}>
            {operationGroups.map((value) => (
              <option key={value} value={value}>{value}</option>
            ))}
          </select>
        </div>
        <div>
          <label htmlFor="remote-observability-anomalies">Visibility</label>
          <label className="log-filter-pill">
            <input
              id="remote-observability-anomalies"
              type="checkbox"
              checked={anomaliesOnly}
              onChange={(event) => setAnomaliesOnly(event.target.checked)}
            />
            only anomalies
          </label>
        </div>
      </div>

      <p id="remote-observability-status" className="text-dim">
        {!featureFlags.remoteControlPlaneEnabled
          ? 'Remote control plane is disabled.'
          : !authz.permissions.viewExecutions
            ? 'Observability access is restricted for current role.'
            : remoteMetricsQuery.isLoading || orchestratorMetricsQuery.isLoading
              ? 'Loading observability…'
              : statusLine}
      </p>

      <div id="remote-observability-summary" className="agent-grid">
        <SummaryCard
          title="Operations"
          lines={[
            `success rate: ${formatPercent(remoteMetrics?.totals?.successRate)}`,
            `avg latency: ${formatMilliseconds(remoteMetrics?.totals?.avgLatencyMs)}`,
            `total: ${toFiniteNumber(remoteMetrics?.totals?.total)}`,
          ]}
        />
        <SummaryCard
          title="Rollout"
          lines={[
            `state: ${String(remoteMetrics?.rollout?.state || 'unknown')}`,
            `can promote: ${String(!!remoteMetrics?.rollout?.canPromote)}`,
            `reasons: ${Array.isArray(remoteMetrics?.rollout?.reasons) && remoteMetrics.rollout.reasons.length ? remoteMetrics.rollout.reasons.join(', ') : 'none'}`,
          ]}
        />
        <SummaryCard
          title="Executions"
          lines={[
            `running: ${toFiniteNumber(orchestratorMetrics?.executions?.running)}`,
            `retry count: ${toFiniteNumber(orchestratorMetrics?.executions?.retryCount)}`,
            `avg latency: ${formatMilliseconds(orchestratorMetrics?.executions?.avgLatencyMs)}`,
          ]}
        />
        <SummaryCard
          title="Workers"
          lines={[
            `total: ${toFiniteNumber(orchestratorMetrics?.workers?.total)}`,
            `busy: ${toFiniteNumber(orchestratorMetrics?.workers?.busy)}`,
            `stale: ${toFiniteNumber(orchestratorMetrics?.workers?.stale)}`,
          ]}
        />
        <SummaryCard
          title="Provider Failures"
          lines={[
            `requested: ${formatMetricsBreakdown(providerMetrics?.requestedFailures)}`,
            `resolved: ${formatMetricsBreakdown(providerMetrics?.resolvedFailures)}`,
          ]}
        />
        <SummaryCard
          title="Provider Usage"
          lines={[
            `estimated cost: ${formatUSD(providerMetrics?.totalEstimatedCostUsd)}`,
            `top provider: ${topProvider}`,
            `top model: ${topModel}`,
            `drift: ${formatMetricsBreakdown(providerMetrics?.driftStates)}`,
          ]}
        />
        <SummaryCard
          title="Cost Attribution"
          lines={[
            <a key="team" href={drillDownHref('team', topTeam)} className="summary-link" onClick={(event) => { event.preventDefault(); drillDown('team', topTeam); }} style={{ scrollMarginTop: '120px' }}>top team: {topTeam}</a>,
            <a key="project" href={drillDownHref('project', topProject)} className="summary-link" onClick={(event) => { event.preventDefault(); drillDown('project', topProject); }} style={{ scrollMarginTop: '120px' }}>top project: {topProject}</a>,
            <a key="template" href={drillDownHref('template', topTemplate)} className="summary-link" onClick={(event) => { event.preventDefault(); drillDown('template', topTemplate); }} style={{ scrollMarginTop: '120px' }}>top template: {topTemplate}</a>,
            <a key="trigger" href={drillDownHref('trigger', topTrigger)} className="summary-link" onClick={(event) => { event.preventDefault(); drillDown('trigger', topTrigger); }} style={{ scrollMarginTop: '120px' }}>top trigger: {topTrigger}</a>,
          ]}
        />
        <SummaryCard
          title="Policy Blocks"
          lines={[
            `allow: ${toFiniteNumber(policyMetrics?.allow)}`,
            `ask: ${toFiniteNumber(policyMetrics?.ask)}`,
            `deny: ${toFiniteNumber(policyMetrics?.deny)}`,
          ]}
        />
      </div>

      <div className="card" style={{ marginTop: '12px' }}>
        <h3>Operation Metrics</h3>
        <div className="metrics-table-wrap">
          <table className="metrics-table" id="remote-observability-ops-table">
            <thead>
              <tr>
                <th>Operation</th>
                <th>Total</th>
                <th>Success</th>
                <th>Failure</th>
                <th>Success Rate</th>
                <th>Avg Latency</th>
              </tr>
            </thead>
            <tbody id="remote-observability-ops-body">
              {visibleOperations.length ? visibleOperations.map((entry) => (
                <tr key={entry.name}>
                  <td>{entry.name}</td>
                  <td>{toFiniteNumber(entry.metrics?.total)}</td>
                  <td>{toFiniteNumber(entry.metrics?.success)}</td>
                  <td>{toFiniteNumber(entry.metrics?.failure)}</td>
                  <td>{formatPercent(entry.metrics?.successRate)}</td>
                  <td>{formatMilliseconds(entry.metrics?.avgLatencyMs)}</td>
                </tr>
              )) : (
                <tr>
                  <td colSpan={6}>No remote operation metrics match current filters.</td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>
    </PageShell>
  );
}
