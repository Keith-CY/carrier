export function HighlightedText({ text, query }: { text: string; query: string }) {
  if (!query) return <>{text}</>;

  const lower = text.toLowerCase();
  const parts: Array<{ key: string; value: string; match: boolean }> = [];
  let cursor = 0;
  let matchIndex = 0;
  while (cursor < text.length) {
    const index = lower.indexOf(query, cursor);
    if (index === -1) {
      parts.push({ key: `tail-${cursor}`, value: text.slice(cursor), match: false });
      break;
    }
    if (index > cursor) {
      parts.push({ key: `plain-${cursor}`, value: text.slice(cursor, index), match: false });
    }
    parts.push({ key: `mark-${matchIndex++}`, value: text.slice(index, index + query.length), match: true });
    cursor = index + query.length;
  }

  return (
    <>
      {parts.map((part) => (part.match ? <mark key={part.key} className="log-highlight">{part.value}</mark> : <span key={part.key}>{part.value}</span>))}
    </>
  );
}
