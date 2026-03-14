import { useEffect, useState } from 'react';
import { apiGet, type ProviderAuthStatusPayload } from '../../lib/api';
import { flattenProviderCatalog } from './model';
import type { WizardProvider } from './state';

type Args = {
  enabled: boolean;
  addMode: boolean;
  setSelectedProvider: (value: WizardProvider | null) => void;
};

export function useOnboardingProviderData({ enabled, addMode, setSelectedProvider }: Args) {
  const [providers, setProviders] = useState<WizardProvider[]>([]);
  const [providerMsg, setProviderMsg] = useState('');
  const [providerLoading, setProviderLoading] = useState(true);
  const [carrierDefaultProvider, setCarrierDefaultProvider] = useState<any>(null);

  useEffect(() => {
    if (!enabled) return;
    setProviderLoading(true);
    setProviderMsg('');
    void Promise.all([
      apiGet<any>('/api/v1/providers'),
      apiGet<ProviderAuthStatusPayload>('/api/v1/auth/providers'),
    ])
      .then(([payload, authStatusPayload]) => {
        const flattened = flattenProviderCatalog(payload, authStatusPayload);
        const defaultProvider = payload?.carrier_default_provider || null;
        setProviders(flattened);
        setCarrierDefaultProvider(defaultProvider);
        if (addMode && defaultProvider?.reusable) {
          const matched = flattened.find(
            (provider) =>
              String(provider.id || '').trim().toLowerCase() ===
              String(defaultProvider.id || '').trim().toLowerCase(),
          );
          if (matched) setSelectedProvider(matched);
        }
      })
      .catch((error) => {
        setProviders([]);
        setCarrierDefaultProvider(null);
        setProviderMsg(`Error loading providers: ${error.message}`);
      })
      .finally(() => setProviderLoading(false));
  }, [addMode, enabled, setSelectedProvider]);

  return {
    providers,
    providerMsg,
    providerLoading,
    carrierDefaultProvider,
  };
}
