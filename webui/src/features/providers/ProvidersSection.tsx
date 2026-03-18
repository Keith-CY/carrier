import { PageShell } from '../../app/page-shell';
import { useProvidersData } from './useProvidersData';
import { ProviderBindingsCard } from './components/ProviderBindingsCard';
import { ProviderBindingsList } from './components/ProviderBindingsList';
import { ProviderProfileEditor } from './components/ProviderProfileEditor';
import { ProviderProfilesList } from './components/ProviderProfilesList';
import { ResolutionPreviewCard } from './components/ResolutionPreviewCard';

export function ProvidersSection() {
  const data = useProvidersData();

  return (
    <PageShell
      id="view-profiles"
      eyebrow="Configure"
      title="Providers"
      titleId="profiles-title"
      description="Curate reusable model credentials, binding rules, and resolution previews from a single registry surface."
      actions={(
        <button id="profiles-refresh" className="btn-sm btn-secondary" onClick={() => void data.refreshAll()}>Refresh</button>
      )}
    >
      <div id="providers-shell">
        <ProviderProfileEditor data={data} />
        <ProviderBindingsCard data={data} />
        <ResolutionPreviewCard data={data} />
        <ProviderProfilesList data={data} />
        <ProviderBindingsList data={data} />
      </div>
    </PageShell>
  );
}
