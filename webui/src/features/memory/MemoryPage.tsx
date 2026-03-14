import { MemorySection } from './MemorySection';
import { useMemoryData } from './useMemoryData';

export function MemoryPage() {
  const data = useMemoryData();
  return <MemorySection data={data} />;
}
