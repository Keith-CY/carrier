import { useExecutionsData } from './useExecutionsData';
import { ExecutionsSection } from './ExecutionsSection';

export function ExecutionsPage() {
  const data = useExecutionsData();
  return <ExecutionsSection data={data} />;
}
