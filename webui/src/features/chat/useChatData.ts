import { KeyboardEvent, useState } from 'react';

type ChatMessage = {
  id: number;
  sender: string;
  text: string;
};

export function useChatData() {
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

  return {
    input,
    setInput,
    messages,
    send,
    onKeyDown,
  };
}

export type ChatData = ReturnType<typeof useChatData>;
