import { SettingsSection } from './SettingsSection';
import { useSettingsData } from './useSettingsData';

export function SettingsPage() {
  const data = useSettingsData();
  return <SettingsSection data={data} />;
}
