import { useState } from 'react';
import { apiPatch, apiPost } from '../../lib/api';
import {
  DEFAULT_EDITOR,
  parseCSV,
  updateEditorFromHost,
  type HostEditorState,
  type HostRecord,
  type MessageState,
} from './model';

type UseHostEditorDataArgs = {
  canManageHosts: boolean;
  refresh: () => Promise<void>;
  setServersMessage: (message: MessageState) => void;
};

export function useHostEditorData({ canManageHosts, refresh, setServersMessage }: UseHostEditorDataArgs) {
  const [editor, setEditor] = useState<HostEditorState>(DEFAULT_EDITOR);
  const [editingHostId, setEditingHostId] = useState('');
  const [editorBusy, setEditorBusy] = useState(false);

  function resetEditor(clearForm: boolean) {
    setEditingHostId('');
    if (clearForm) setEditor(DEFAULT_EDITOR);
  }

  async function handleSaveHost() {
    if (!canManageHosts) {
      setServersMessage({ type: 'error', text: 'Current role cannot modify remote hosts.' });
      return;
    }
    const payload = {
      name: editor.name.trim(),
      host: editor.host.trim(),
      port: parseInt(editor.port.trim() || '22', 10) || 22,
      user: editor.user.trim(),
      authMode: editor.authMode.trim(),
      keyPath: editor.keyPath.trim(),
      sshConfigHost: editor.sshConfigHost.trim(),
      runtimeMode: editor.runtimeMode.trim() || 'on_demand',
      labels: parseCSV(editor.labels),
    };
    setEditorBusy(true);
    try {
      if (editingHostId) {
        await apiPatch(`/api/v1/remote/hosts/${encodeURIComponent(editingHostId)}`, payload);
        setServersMessage({ type: 'success', text: `Remote host updated: ${editingHostId}` });
      } else {
        await apiPost('/api/v1/remote/hosts', payload);
        setServersMessage({ type: 'success', text: 'Remote host saved.' });
      }
      resetEditor(true);
      await refresh();
    } catch (error) {
      setServersMessage({ type: 'error', text: `Save failed: ${(error as Error).message}` });
    } finally {
      setEditorBusy(false);
    }
  }

  function startEditingHost(host: HostRecord) {
    const hostId = String(host?.id || '');
    setEditingHostId(hostId);
    setEditor(updateEditorFromHost(host));
    setServersMessage({ type: 'info', text: `Editing remote host: ${hostId}` });
  }

  return {
    editor,
    setEditor,
    editingHostId,
    setEditingHostId,
    editorBusy,
    resetEditor,
    handleSaveHost,
    startEditingHost,
  };
}

