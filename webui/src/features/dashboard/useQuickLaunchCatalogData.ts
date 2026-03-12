import { useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import { apiGet } from '../../lib/api';
import { flattenProviderCatalog } from './model';

export function useQuickLaunchCatalogData(enabled: boolean, templateId: string) {
  const providersQuery = useQuery({
    queryKey: ['provider-catalog'],
    queryFn: () => apiGet<any>('/api/v1/providers'),
    enabled,
  });
  const templatesQuery = useQuery({
    queryKey: ['execution-templates'],
    queryFn: () => apiGet<any>('/api/v1/templates'),
    enabled,
  });
  const hostsQuery = useQuery({
    queryKey: ['remote-hosts'],
    queryFn: () => apiGet<any>('/api/v1/remote/hosts'),
    enabled,
  });

  const providerOptions = useMemo(() => flattenProviderCatalog(providersQuery.data), [providersQuery.data]);
  const templates = useMemo(() => Array.isArray(templatesQuery.data?.templates) ? templatesQuery.data.templates : [], [templatesQuery.data]);
  const hosts = useMemo(() => Array.isArray(hostsQuery.data?.hosts) ? hostsQuery.data.hosts : [], [hostsQuery.data]);
  const selectedTemplate = useMemo(
    () => templates.find((item: any) => String(item?.id || '').trim() === templateId) || null,
    [templates, templateId],
  );
  const hostOptions = useMemo(() => {
    const items = [{ id: 'local', name: 'local' }].concat(hosts);
    const seen = new Set<string>();
    return items.filter((item: any) => {
      const id = String(item?.id || '').trim();
      if (!id || seen.has(id)) return false;
      seen.add(id);
      return true;
    });
  }, [hosts]);

  return {
    templatesQuery,
    providerOptions,
    templates,
    selectedTemplate,
    hostOptions,
  };
}
