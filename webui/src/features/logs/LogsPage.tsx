import { LogsSection } from './LogsSection';
import { useLogsData } from './useLogsData';

export function LogsPage() {
  const data = useLogsData();
  return <LogsSection data={data} />;
}
