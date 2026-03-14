import type { ChatData } from '../useChatData';

export function ChatMessages({ data }: { data: ChatData }) {
  return (
    <div id="chat-messages" className="chat-messages">
      {data.messages.map((message) => (
        <div key={message.id} className="chat-msg">
          <span className="sender">{message.sender}:</span>
          <span className="body"> {message.text}</span>
        </div>
      ))}
    </div>
  );
}
