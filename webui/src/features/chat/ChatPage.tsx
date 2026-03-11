import { KeyboardEvent, useState } from 'react';

type ChatMessage = {
  id: number;
  sender: string;
  text: string;
};

export function ChatPage() {
  const [input, setInput] = useState('');
  const [messages, setMessages] = useState<ChatMessage[]>([]);

  const send = () => {
    const text = input.trim();
    if (!text) return;
    setMessages((current) => current.concat(
      { id: current.length * 2 + 1, sender: 'You', text },
      {
        id: current.length * 2 + 2,
        sender: 'Carrier',
        text: 'Chat is not available in daemon mode. Use the Dashboard to manage agents.',
      },
    ));
    setInput('');
  };

  const onKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'Enter') send();
  };

  return (
    <section id="view-chat" className="view">
      <h2>Chat</h2>
      <div id="chat-messages" className="chat-messages">
        {messages.map((message) => (
          <div key={message.id} className="chat-msg">
            <span className="sender">{message.sender}:</span>
            <span className="body"> {message.text}</span>
          </div>
        ))}
      </div>
      <div className="chat-input-row">
        <input
          id="chat-input"
          type="text"
          placeholder="Type a message…"
          autoComplete="off"
          value={input}
          onChange={(event) => setInput(event.target.value)}
          onKeyDown={onKeyDown}
        />
        <button id="chat-send" type="button" onClick={send}>Send</button>
      </div>
    </section>
  );
}
