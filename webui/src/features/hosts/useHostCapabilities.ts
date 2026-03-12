import { useFeatures } from '../../app/useFeatures';

export function useHostCapabilities() {
  const { featureFlags, authz, isLoading: featuresLoading } = useFeatures();

  return {
    featureFlags,
    authz,
    featuresLoading,
    canManageHosts: !!authz.permissions.manageHosts,
  };
}
