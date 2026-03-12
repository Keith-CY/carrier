import type { RemoteChatData } from './useRemoteChatData';
import { RemoteChatCompose } from './components/RemoteChatCompose';
import { RemoteChatThread } from './components/RemoteChatThread';
import { RemoteChatToolbar } from './components/RemoteChatToolbar';

export function RemoteChatSection({ data }: { data: RemoteChatData }) {
  return (
    <section id="view-remote-chat" className="view view-remote-chat-surface">
      <h2>Remote Chat</h2>
      <RemoteChatToolbar data={data} />
      <RemoteChatThread data={data} />
      <RemoteChatCompose data={data} />
    </section>
  );
}
