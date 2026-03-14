import { useQuery } from '@tanstack/react-query';
import { apiGet } from '../lib/api';
import { DEFAULT_AUTHZ, DEFAULT_FEATURE_FLAGS, normalizeAuthz, normalizeFeatureFlags } from '../lib/flags';
import { useSession } from './session';

type FeaturesPayload = {
  features?: Record<string, unknown>;
  authz?: Record<string, unknown>;
};

export function useFeatures(enabled = true) {
  const { authenticated } = useSession();
  const query = useQuery({
    queryKey: ['features'],
    queryFn: () => apiGet<FeaturesPayload>('/api/v1/features'),
    enabled: enabled && authenticated,
    staleTime: 15000,
    refetchInterval: 30000,
  });

  return {
    ...query,
    featureFlags: query.data ? normalizeFeatureFlags(query.data) : DEFAULT_FEATURE_FLAGS,
    authz: query.data ? normalizeAuthz(query.data) : DEFAULT_AUTHZ,
  };
}
