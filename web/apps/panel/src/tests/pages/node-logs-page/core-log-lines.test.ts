import { expect, it } from 'vitest';

import { appendCoreText, emptyCoreText, parseCoreLines } from '@/pages/node-logs-page/core-log-lines';

it('keeps message colors with levels, including multiline errors, and bounds the buffer', () => {
  const lines = parseCoreLines(appendCoreText(
    emptyCoreText,
    '+0800 2026-09-19 12:00:00 ERROR TLS handshake\nEOF\nINFO connected\n',
  ));
  expect(lines[0]).toMatchObject({
    prefix: '+0800 2026-09-19 12:00:00 ',
    message: 'ERROR TLS handshake',
    level: 'error',
  });
  expect(lines[1].level).toBe('error');
  expect(lines[2].level).toBe('info');
  expect(appendCoreText(emptyCoreText, `${'old\n'.repeat(2500)}INFO newest\n`).text.split('\n')).toHaveLength(2001);
});

it('keeps each line\'s identity while a full buffer moves, and after a reset', () => {
  const full = appendCoreText(emptyCoreText, Array.from({ length: 2000 }, (_, index) => `INFO ${index}\n`).join(''));
  const ids = (text: typeof full) => new Map(parseCoreLines(text).map(line => [line.message, line.id]));
  const before = ids(full);
  // A line arrives in two chunks; it keeps one identity, and the line it pushes out leaves.
  const next = appendCoreText(appendCoreText(full, 'INFO new'), 'est\n');
  const after = ids(next);
  expect(after.has('INFO 0')).toBe(false);
  for (const [message, id] of after) {
    if (message !== 'INFO newest') expect(id).toBe(before.get(message));
  }
  expect(after.get('INFO newest')).toBe(ids(appendCoreText(full, 'INFO new')).get('INFO new'));
  const reset = parseCoreLines(appendCoreText(next, 'INFO again\n', true));
  expect(reset[0].id).toBeGreaterThan(Math.max(...after.values()));
});
