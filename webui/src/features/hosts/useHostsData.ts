import { useHostEditorData } from './useHostEditorData';
import { useHostCapabilities } from './useHostCapabilities';
import { useHostManageOperations } from './useHostManageOperations';
import { useHostManageState } from './useHostManageState';
import { useHostsInventoryData } from './useHostsInventoryData';
import { useSelectedHost } from './useSelectedHost';

export function useHostsData() {
  const { featureFlags, authz, featuresLoading, canManageHosts } = useHostCapabilities();
  const inventory = useHostsInventoryData({
    remoteControlPlaneEnabled: featureFlags.remoteControlPlaneEnabled,
    featuresLoading,
    canManageHosts,
  });
  const editorData = useHostEditorData({
    canManageHosts,
    refresh: inventory.refresh,
    setServersMessage: inventory.setServersMessage,
  });
  const manageState = useHostManageState(inventory.hosts);
  const manageOps = useHostManageOperations({
    hosts: inventory.hosts,
    refresh: inventory.refresh,
    selectedHostId: manageState.selectedHostId,
    manageForm: manageState.manageForm,
    manageBusy: manageState.manageBusy,
    configText: manageState.configText,
    setServersMessage: inventory.setServersMessage,
    setManageMessage: manageState.setManageMessage,
    setManageBusy: manageState.setManageBusy,
    setHostOps: manageState.setHostOps as any,
    setOpMeta: manageState.setOpMeta,
    setInstancesText: manageState.setInstancesText,
    setInstanceStatusText: manageState.setInstanceStatusText,
    setLogsText: manageState.setLogsText,
    setConfigText: manageState.setConfigText,
    setSessionsText: manageState.setSessionsText,
    setMemoryText: manageState.setMemoryText,
    setManageForm: manageState.setManageForm,
    editingHostId: editorData.editingHostId,
    resetEditor: editorData.resetEditor,
  });

  const selectedHost = useSelectedHost(inventory.hosts, manageState.selectedHostId);

  return {
    featureFlags,
    authz,
    featuresLoading,
    canManageHosts,
    ...inventory,
    ...editorData,
    ...manageState,
    selectedHostId: manageState.selectedHostId,
    selectedHost,
    ...manageOps,
  };
}

export type HostsData = ReturnType<typeof useHostsData>;
