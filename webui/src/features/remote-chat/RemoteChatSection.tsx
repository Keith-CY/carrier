import { PageShell } from '../../app/page-shell';
import type { RemoteChatData } from './useRemoteChatData';
import { RemoteChatCompose } from './components/RemoteChatCompose';
import { RemoteChatThread } from './components/RemoteChatThread';
import { RemoteChatToolbar } from './components/RemoteChatToolbar';

export function RemoteChatSection({ data }: { data: RemoteChatData }) {
  const hosts = Array.isArray(data.hosts) ? data.hosts : [];
  const instances = Array.isArray(data.instances) ? data.instances : [];
  const profiles = Array.isArray(data.profiles) ? data.profiles : [];
  const hostLabel = hosts.find((option) => option.value === data.hostId)?.label || data.hostId || 'n/a';
  const agentLabel = instances.find((option) => option.value === data.agentId)?.label || data.agentId || 'n/a';
  const profileLabel = profiles.find((option) => option.value === data.profileId)?.label || data.profileId || 'n/a';

  return (
    <PageShell
      id="view-remote-chat"
      className="page-remote-chat"
      eyebrow="Operate"
      title="Remote Chat"
      description="Switch between remote targets, carry session context, and intervene directly without leaving the control surface."
      stats={[
        { label: 'Target', value: data.target },
        { label: 'Host', value: hostLabel },
        { label: 'Instance', value: agentLabel },
        { label: 'Profile', value: profileLabel },
      ]}
    >
      <div className="remote-chat-layout">
        <RemoteChatToolbar data={data} />
        <div className="remote-chat-stack">
          <RemoteChatThread data={data} />
          <RemoteChatCompose data={data} />
        </div>
      </div>
    </PageShell>
  );
}
