import type { DashboardData } from './useDashboardData';
import { AddAgentModal } from './components/AddAgentModal';
import { InstalledAgentsCard } from './components/InstalledAgentsCard';
import { QuickLaunchCard } from './components/QuickLaunchCard';
import { RecentExecutionsCard } from './components/RecentExecutionsCard';

export function DashboardSection({ data }: { data: DashboardData }) {
  return (
    <section id="view-dashboard" className="view">
      <QuickLaunchCard data={data} />
      <InstalledAgentsCard data={data} />
      <RecentExecutionsCard data={data} />
      <AddAgentModal data={data} />
    </section>
  );
}
