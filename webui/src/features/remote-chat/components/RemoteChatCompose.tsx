import type { RemoteChatData } from '../useRemoteChatData';

export function RemoteChatCompose({ data }: { data: RemoteChatData }) {
  return (
    <div className="card remote-chat-compose">
      <div className="chat-input-row">
        <input
          id="remote-chat-input"
          type="text"
          placeholder="Message remote instance…"
          autoComplete="off"
          value={data.input}
          onChange={(event) => data.setInput(event.target.value)}
          onKeyDown={data.onEnter}
        />
        <button id="remote-chat-send" type="button" onClick={() => void data.send()}>Send</button>
      </div>
    </div>
  );
}
