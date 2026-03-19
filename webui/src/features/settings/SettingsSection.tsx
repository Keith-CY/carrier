import { PageShell } from '../../app/page-shell';
import type { SettingsData } from './useSettingsData';
import { SettingsGatewayCard } from './components/SettingsGatewayCard';
import { SettingsProviderCard } from './components/SettingsProviderCard';

export function SettingsSection({ data }: { data: SettingsData }) {
  return (
    <PageShell
      id="view-settings"
      eyebrow="Configure"
      title="Settings"
      description="Tune gateway connectivity, provider defaults, and baseline WebUI configuration without leaving the application shell."
    >
      <SettingsProviderCard data={data} />
      <SettingsGatewayCard data={data} />
    </PageShell>
  );
}
