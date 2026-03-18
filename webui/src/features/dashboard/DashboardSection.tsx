import { PageShell } from '../../app/page-shell';
import type { DashboardData } from './useDashboardData';
import { AddAgentModal } from './components/AddAgentModal';
import { InstalledAgentsCard } from './components/InstalledAgentsCard';
import { QuickLaunchCard } from './components/QuickLaunchCard';
import { RecentExecutionsCard } from './components/RecentExecutionsCard';

export function DashboardSection({ data }: { data: DashboardData }) {
  const instances = Array.isArray(data.instances) ? data.instances : [];
  const recentExecutions = Array.isArray(data.recentExecutions) ? data.recentExecutions : [];
  const hostOptions = Array.isArray(data.hostOptions) ? data.hostOptions : [];
  const providerOptions = Array.isArray(data.providerOptions) ? data.providerOptions : [];

  return (
    <PageShell
      id="view-dashboard"
      className="page-dashboard"
      eyebrow="Operate"
      title="Dashboard"
      description="Plan work, watch the fleet, and supervise orchestration from a single operator-grade surface."
      stats={[
        { label: 'Installed Agents', value: String(instances.length) },
        { label: 'Running', value: String(data.runningInstances || 0) },
        { label: 'Recent Executions', value: String(recentExecutions.length) },
        { label: 'Active Now', value: String(data.activeRecentExecutions || 0) },
        { label: 'Target Hosts', value: String(hostOptions.length) },
        { label: 'Provider Profiles', value: String(providerOptions.length) },
      ]}
    >
      <QuickLaunchCard data={data} />
      <InstalledAgentsCard data={data} />
      <RecentExecutionsCard data={data} />
      <AddAgentModal data={data} />
    </PageShell>
  );
}
