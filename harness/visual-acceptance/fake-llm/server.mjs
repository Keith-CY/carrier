import http from 'node:http';

const port = Number(process.env.FAKE_LLM_PORT || 8080);

function collectTextParts(value) {
  if (typeof value === 'string') return [value];
  if (Array.isArray(value)) return value.flatMap(collectTextParts);
  if (!value || typeof value !== 'object') return [];
  return Object.values(value).flatMap(collectTextParts);
}

function extractPrompt(body) {
  if (Array.isArray(body?.messages)) {
    const last = [...body.messages].reverse().find((message) => String(message?.role || '').trim() === 'user');
    return collectTextParts(last?.content).join(' ').trim();
  }
  if (Array.isArray(body?.input)) {
    return collectTextParts(body.input).join(' ').trim();
  }
  return '';
}

function buildReply(prompt) {
  const normalized = String(prompt || '').trim().toLowerCase();
  if (!normalized) return 'visual acceptance ok';
  if (normalized.includes('say exactly remote openclaw ok')) return 'remote openclaw ok';
  if (normalized.includes('triage')) return 'triage summary ready';
  if (normalized.includes('incident')) return 'incident diagnosis ready';
  return `ack: ${String(prompt || '').trim()}`;
}

function writeJson(res, status, payload) {
  res.writeHead(status, { 'Content-Type': 'application/json' });
  res.end(JSON.stringify(payload));
}

function writeStreamingChat(res, reply) {
  res.writeHead(200, {
    'Content-Type': 'text/event-stream',
    'Cache-Control': 'no-cache',
    Connection: 'keep-alive',
  });
  const created = Math.floor(Date.now() / 1000);
  res.write(`data: ${JSON.stringify({
    id: 'chatcmpl-fake',
    object: 'chat.completion.chunk',
    created,
    model: 'fake-model',
    choices: [{ index: 0, delta: { role: 'assistant', content: reply }, finish_reason: null }],
  })}\n\n`);
  res.write(`data: ${JSON.stringify({
    id: 'chatcmpl-fake',
    object: 'chat.completion.chunk',
    created,
    model: 'fake-model',
    choices: [{ index: 0, delta: {}, finish_reason: 'stop' }],
  })}\n\n`);
  res.write('data: [DONE]\n\n');
  res.end();
}

function writeStreamingResponse(res, reply) {
  res.writeHead(200, {
    'Content-Type': 'text/event-stream',
    'Cache-Control': 'no-cache',
    Connection: 'keep-alive',
  });
  res.write(`event: response.output_text.delta\ndata: ${JSON.stringify({ delta: reply })}\n\n`);
  res.write('event: response.completed\ndata: {}\n\n');
  res.end();
}

const server = http.createServer((req, res) => {
  if (req.method === 'GET' && req.url === '/healthz') {
    writeJson(res, 200, { ok: true });
    return;
  }

  if (req.method !== 'POST') {
    writeJson(res, 404, { error: 'not found' });
    return;
  }

  let raw = '';
  req.on('data', (chunk) => {
    raw += chunk.toString('utf8');
  });
  req.on('end', () => {
    let body = {};
    try {
      body = raw ? JSON.parse(raw) : {};
    } catch {
      writeJson(res, 400, { error: 'invalid json' });
      return;
    }

    const reply = buildReply(extractPrompt(body));

    if (req.url === '/v1/chat/completions' || req.url === '/chat/completions') {
      if (body?.stream) {
        writeStreamingChat(res, reply);
        return;
      }
      writeJson(res, 200, {
        id: 'chatcmpl-fake',
        object: 'chat.completion',
        created: Math.floor(Date.now() / 1000),
        model: 'fake-model',
        choices: [{ index: 0, message: { role: 'assistant', content: reply }, finish_reason: 'stop' }],
      });
      return;
    }

    if (req.url === '/v1/responses' || req.url === '/responses') {
      if (body?.stream) {
        writeStreamingResponse(res, reply);
        return;
      }
      writeJson(res, 200, {
        id: 'resp-fake',
        object: 'response',
        output_text: reply,
        output: [{ type: 'message', role: 'assistant', content: [{ type: 'output_text', text: reply }] }],
      });
      return;
    }

    writeJson(res, 404, { error: 'unknown endpoint' });
  });
});

server.listen(port, '0.0.0.0', () => {
  process.stdout.write(`[fake-llm] listening on ${port}\n`);
});
