import { useQuery } from '@tanstack/react-query';
import { useFeatures } from '../../app/useFeatures';
import { useSession } from '../../app/session';
import { apiGet, type ChannelStatusPayload, type ProviderAuthStatusPayload } from '../../lib/api';
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

  const providerAuthStatusQuery = useQuery({
    queryKey: ['settings-provider-auth-status'],
    queryFn: () => apiGet<ProviderAuthStatusPayload>('/api/v1/auth/providers'),
    retry: false,
  });

  const channelStatusQuery = useQuery({
    queryKey: ['settings-channel-status'],
    queryFn: () => apiGet<ChannelStatusPayload>('/api/v1/channels'),
    retry: false,
  });

  const summary = buildSettingsSummary(
    transportQuery.isError ? null : transportQuery.data,
    remoteMetricsQuery.isError ? null : remoteMetricsQuery.data,
    featureFlags.remoteControlPlaneEnabled,
    providerAuthStatusQuery.isError ? null : providerAuthStatusQuery.data,
    channelStatusQuery.isError ? null : channelStatusQuery.data,
  );

  return {
    summary,
    logout,
  };
}

export type SettingsData = ReturnType<typeof useSettingsData>;
