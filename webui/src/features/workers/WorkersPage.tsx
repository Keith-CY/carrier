import { useWorkersData } from './useWorkersData';
import { WorkersSection } from './WorkersSection';

export function WorkersPage() {
  const data = useWorkersData();
  return <WorkersSection data={data} />;
}
