import type { SettingsData } from '../useSettingsData';

export function SettingsGatewayCard({ data }: { data: SettingsData }) {
  return (
    <div className="card" style={{ marginTop: '1rem' }}>
      <h3>Gateway</h3>
      <p className="text-dim">Token: ••••••</p>
      <button id="settings-logout" className="btn-sm btn-danger" type="button" onClick={data.logout}>
        Clear Token &amp; Logout
      </button>
    </div>
  );
}
