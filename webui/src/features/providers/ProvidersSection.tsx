import { useProvidersData } from './useProvidersData';
import { ProviderBindingsCard } from './components/ProviderBindingsCard';
import { ProviderBindingsList } from './components/ProviderBindingsList';
import { ProviderProfileEditor } from './components/ProviderProfileEditor';
import { ProviderProfilesList } from './components/ProviderProfilesList';
import { ResolutionPreviewCard } from './components/ResolutionPreviewCard';

export function ProvidersSection() {
  const data = useProvidersData();

  return (
    <section id="view-profiles" className="view">
      <div className="section-head">
        <h2 id="profiles-title">Providers</h2>
        <div className="section-actions">
          <button id="profiles-refresh" className="btn-sm btn-secondary" onClick={() => void data.refreshAll()}>Refresh</button>
        </div>
      </div>
      <div id="providers-shell">
        <ProviderProfileEditor data={data} />
        <ProviderBindingsCard data={data} />
        <ResolutionPreviewCard data={data} />
        <ProviderProfilesList data={data} />
        <ProviderBindingsList data={data} />
      </div>
    </section>
  );
}
