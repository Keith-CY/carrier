import type { MemoryData } from './useMemoryData';
import { MemoryActionsCard } from './components/MemoryActionsCard';
import { MemoryPackagesCard } from './components/MemoryPackagesCard';
import { MemoryResultsCard } from './components/MemoryResultsCard';
import { MemorySearchCard } from './components/MemorySearchCard';

export function MemorySection({ data }: { data: MemoryData }) {
  return (
    <section id="view-memory" className="view">
      <MemorySearchCard data={data} />
      <MemoryActionsCard data={data} />
      <MemoryPackagesCard data={data} />
      <MemoryResultsCard data={data} />
    </section>
  );
}
