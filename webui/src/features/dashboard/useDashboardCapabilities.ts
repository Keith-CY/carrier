import { useFeatures } from '../../app/useFeatures';

export function useDashboardCapabilities() {
  const { featureFlags, authz } = useFeatures();

  return {
    featureFlags,
    authz,
    canViewExecutions: featureFlags.remoteControlPlaneEnabled && authz.permissions.viewExecutions,
    canLaunchExecutions: featureFlags.remoteControlPlaneEnabled && authz.permissions.launchExecutions,
  };
}

