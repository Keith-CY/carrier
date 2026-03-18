import type { RemoteChatData } from '../useRemoteChatData';

export function RemoteChatThread({ data }: { data: RemoteChatData }) {
  return (
    <div className="card remote-chat-thread">
      <div id="remote-chat-messages" className="chat-messages">
        {data.messages.length ? data.messages.map((message) => (
          <div key={message.id} className={`chat-msg chat-msg--${message.role}`}>
            <span className="sender">
              {message.role === 'user' ? 'You' : message.role === 'assistant' ? 'Agent' : 'Carrier'}:
            </span>
            <span className="body"> {message.text}</span>
          </div>
        )) : (
          <div className="chat-msg chat-msg--system">
            <span className="sender">Carrier:</span>
            <span className="body"> Start a session to inspect or steer a selected remote runtime.</span>
          </div>
        )}
      </div>
    </div>
  );
}
