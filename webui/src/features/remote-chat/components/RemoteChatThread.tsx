import type { RemoteChatData } from '../useRemoteChatData';

export function RemoteChatThread({ data }: { data: RemoteChatData }) {
  return (
    <div className="card remote-chat-thread">
      <div id="remote-chat-messages" className="chat-messages">
        {data.messages.map((message) => (
          <div key={message.id} className="chat-msg">
            <span className="sender">
              {message.role === 'user' ? 'You' : message.role === 'assistant' ? 'Agent' : 'Carrier'}:
            </span>
            <span className="body"> {message.text}</span>
          </div>
        ))}
      </div>
    </div>
  );
}
