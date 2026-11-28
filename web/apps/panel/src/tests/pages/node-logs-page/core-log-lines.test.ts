import { expect, it } from 'vitest';

import { appendCoreText, emptyCoreText } from '@/pages/node-logs-page/core-log-lines';

it('keeps message colors with levels, including multiline errors, and bounds the buffer', () => {
  const { lines } = appendCoreText(
    emptyCoreText,
    '+0800 2026-09-19 12:00:00 ERROR TLS handshake\nEOF\nINFO connected\n',
  );
  expect(lines[0]).toMatchObject({
    prefix: '+0800 2026-09-19 12:00:00 ',
    message: 'ERROR TLS handshake',
    level: 'error',
  });
  expect(lines[1].level).toBe('error');
  expect(lines[2].level).toBe('info');
  const bounded = appendCoreText(emptyCoreText, `${'old\n'.repeat(2500)}INFO newest\n`).lines;
  expect(bounded).toHaveLength(2000);
  expect(bounded.at(-1)?.message).toBe('INFO newest');
});

it('keeps each line\'s identity while a full buffer moves, and after a reset', () => {
  const full = appendCoreText(emptyCoreText, Array.from({ length: 2000 }, (_, index) => `INFO ${index}\n`).join(''));
  const byMessage = (text: typeof full) => new Map(text.lines.map(line => [line.message, line]));
  const before = byMessage(full);
  // A line arrives in two chunks; it keeps one identity, and the line it pushes out leaves.
  const started = appendCoreText(full, 'INFO new');
  const next = appendCoreText(started, 'est\n');
  const after = byMessage(next);
  expect(after.has('INFO 0')).toBe(false);
  // Lines kept from before are not parsed again, so a view drawing them can skip them.
  for (const [message, line] of after) {
    if (message !== 'INFO newest') expect(line).toBe(before.get(message));
  }
  expect(after.get('INFO newest')?.id).toBe(byMessage(started).get('INFO new')?.id);
  const reset = appendCoreText(next, 'INFO again\n', true).lines;
  expect(reset).toHaveLength(1);
  expect(reset[0].id).toBeGreaterThan(Math.max(...[...after.values()].map(line => line.id)));
});
