import type { SettingsData } from '../useSettingsData';

export function SettingsProviderCard({ data }: { data: SettingsData }) {
  return (
    <div className="card">
      <h3>Provider Configuration</h3>
      <div id="settings-provider" className="text-dim">
        {data.summary}
      </div>
    </div>
  );
}
