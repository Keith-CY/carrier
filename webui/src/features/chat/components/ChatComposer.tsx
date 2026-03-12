import type { ChatData } from '../useChatData';

export function ChatComposer({ data }: { data: ChatData }) {
  return (
    <div className="chat-input-row">
      <input
        id="chat-input"
        type="text"
        placeholder="Type a message…"
        autoComplete="off"
        value={data.input}
        onChange={(event) => data.setInput(event.target.value)}
        onKeyDown={data.onKeyDown}
      />
      <button id="chat-send" type="button" onClick={data.send}>Send</button>
    </div>
  );
}
