import { ChatSection } from './ChatSection';
import { useChatData } from './useChatData';

export function ChatPage() {
  const data = useChatData();
  return <ChatSection data={data} />;
}
