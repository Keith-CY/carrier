import { expect, test } from '@playwright/test';
import { daemonJSON } from './fullstack_helpers';

test.describe.configure({ mode: 'serial' });

test.describe('Fullstack Agent Media Runtime', () => {
  test('accepts audio attachment metadata and returns bounded unsupported output without a media runtime', async ({ request }) => {
    const payload = await daemonJSON(request, 'POST', '/api/v1/agents/picoclaw/chat', {
      message: 'Please handle this voice note.',
      attachments: [
        {
          kind: 'audio',
          name: 'voice.ogg',
          mimeType: 'audio/ogg',
          externalId: 'fullstack-audio-1',
        },
      ],
    });

    expect(String(payload.agentId || '')).toBe('picoclaw');
    expect(String(payload.action || '')).toBe('unsupported');
    expect(String(payload.message || '')).toContain('Audio attachments are not supported');
  });
});
