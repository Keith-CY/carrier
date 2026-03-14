import { DashboardSection } from './DashboardSection';
import { useDashboardData } from './useDashboardData';

export function DashboardPage() {
  const data = useDashboardData();
  return <DashboardSection data={data} />;
}
