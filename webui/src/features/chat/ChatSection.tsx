import type { ChatData } from './useChatData';
import { ChatComposer } from './components/ChatComposer';
import { ChatMessages } from './components/ChatMessages';

export function ChatSection({ data }: { data: ChatData }) {
  return (
    <section id="view-chat" className="view">
      <h2>Chat</h2>
      <ChatMessages data={data} />
      <ChatComposer data={data} />
    </section>
  );
}
