import { expect, it } from 'vitest';

import { appendCoreText, parseCoreLines } from '@/pages/observability-page/core-log-lines';

it('keeps message colors with levels, including multiline errors, and bounds the buffer', () => {
  const lines = parseCoreLines(
    '+0800 2026-09-19 12:00:00 ERROR TLS handshake\nEOF\nINFO connected\n',
  );
  expect(lines[0]).toMatchObject({
    prefix: '+0800 2026-09-19 12:00:00 ',
    message: 'ERROR TLS handshake',
    level: 'error',
  });
  expect(lines[1].level).toBe('error');
  expect(lines[2].level).toBe('info');
  expect(appendCoreText('old\n'.repeat(2500), 'INFO newest\n').split('\n')).toHaveLength(2001);
});
