import { useQuery } from '@tanstack/react-query';
import { useFeatures } from '../../app/useFeatures';
import { useSession } from '../../app/session';
import { apiGet } from '../../lib/api';
import { buildSettingsSummary } from './model';

export function useSettingsData() {
  const { featureFlags } = useFeatures();
  const { logout } = useSession();

  const transportQuery = useQuery({
    queryKey: ['settings-transport'],
    queryFn: () => apiGet<any>('/api/v1/telegram/transport'),
    retry: false,
  });

  const remoteMetricsQuery = useQuery({
    queryKey: ['settings-remote-metrics'],
    queryFn: () => apiGet<any>('/api/v1/remote/metrics'),
    enabled: featureFlags.remoteControlPlaneEnabled,
    retry: false,
  });

  const summary = buildSettingsSummary(
    transportQuery.isError ? null : transportQuery.data,
    remoteMetricsQuery.isError ? null : remoteMetricsQuery.data,
    featureFlags.remoteControlPlaneEnabled,
  );

  return {
    summary,
    logout,
  };
}

export type SettingsData = ReturnType<typeof useSettingsData>;
