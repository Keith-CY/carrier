import { RemoteChatSection } from './RemoteChatSection';
import { useRemoteChatData } from './useRemoteChatData';

export function RemoteChatPage() {
  const data = useRemoteChatData();
  return <RemoteChatSection data={data} />;
}
