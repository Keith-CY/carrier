import { PageShell } from '../../app/page-shell';
import type { MemoryData } from './useMemoryData';
import { MemoryActionsCard } from './components/MemoryActionsCard';
import { MemoryPackagesCard } from './components/MemoryPackagesCard';
import { MemoryResultsCard } from './components/MemoryResultsCard';
import { MemorySearchCard } from './components/MemorySearchCard';

export function MemorySection({ data }: { data: MemoryData }) {
  return (
    <PageShell
      id="view-memory"
      eyebrow="Observe"
      title="Memory"
      description="Search curated records, inspect memory packages, and manage long-lived context without losing operational clarity."
    >
      <MemorySearchCard data={data} />
      <MemoryActionsCard data={data} />
      <MemoryPackagesCard data={data} />
      <MemoryResultsCard data={data} />
    </PageShell>
  );
}
