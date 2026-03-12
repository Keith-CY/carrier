import type { SettingsData } from './useSettingsData';
import { SettingsGatewayCard } from './components/SettingsGatewayCard';
import { SettingsProviderCard } from './components/SettingsProviderCard';

export function SettingsSection({ data }: { data: SettingsData }) {
  return (
    <section id="view-settings" className="view">
      <h2>Settings</h2>
      <SettingsProviderCard data={data} />
      <SettingsGatewayCard data={data} />
    </section>
  );
}
