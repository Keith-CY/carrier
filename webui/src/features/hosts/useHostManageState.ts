import { useEffect, useMemo, useState } from 'react';
import {
  DEFAULT_MANAGE_FORM,
  type HostRecord,
  type ManageFormState,
  type MessageState,
  type OperationSummary,
} from './model';

export function useHostManageState(hosts: HostRecord[]) {
  const [manageMessage, setManageMessage] = useState<MessageState>({ type: 'info', text: '' });
  const [selectedHostId, setSelectedHostId] = useState('');
  const [manageForm, setManageForm] = useState<ManageFormState>(DEFAULT_MANAGE_FORM);
  const [manageBusy, setManageBusy] = useState(false);
  const [hostOps, setHostOps] = useState<Record<string, OperationSummary>>({});
  const [opMeta, setOpMeta] = useState<OperationSummary | null>(null);
  const [instancesText, setInstancesText] = useState('');
  const [instanceStatusText, setInstanceStatusText] = useState('');
  const [logsText, setLogsText] = useState('');
  const [configText, setConfigText] = useState('');
  const [sessionsText, setSessionsText] = useState('');
  const [memoryText, setMemoryText] = useState('');

  const selectedHost = useMemo(
    () => hosts.find((host) => String(host?.id || '').trim() === selectedHostId) || null,
    [hosts, selectedHostId],
  );

  useEffect(() => {
    if (!selectedHostId) return;
    if (hosts.some((host) => String(host?.id || '').trim() === selectedHostId)) return;
    setSelectedHostId('');
    setOpMeta(null);
    setManageMessage({ type: 'info', text: '' });
  }, [hosts, selectedHostId]);

  function showManageHost(hostId: string) {
    setSelectedHostId(hostId);
    setInstancesText('');
    setInstanceStatusText('');
    setLogsText('');
    setConfigText('');
    setSessionsText('');
    setMemoryText('');
    setOpMeta(null);
    setManageMessage({ type: 'info', text: '' });
  }

  return {
    manageMessage,
    setManageMessage,
    selectedHostId,
    setSelectedHostId,
    selectedHost,
    manageForm,
    setManageForm,
    manageBusy,
    setManageBusy,
    hostOps,
    setHostOps,
    opMeta,
    setOpMeta,
    instancesText,
    setInstancesText,
    instanceStatusText,
    setInstanceStatusText,
    logsText,
    setLogsText,
    configText,
    setConfigText,
    sessionsText,
    setSessionsText,
    memoryText,
    setMemoryText,
    showManageHost,
  };
}

