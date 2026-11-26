import { expect, it } from 'vitest';

import { compactVersion } from '@/utils/compact-version';

it('keeps a release and its commit from a development build, and short versions whole', () => {
  expect(compactVersion('v0.5.5-0.20261126051431-9230819beb95+dirty')).toBe('v0.5.5…9230819');
  expect(compactVersion('0.5.5-0.20261126051431-9230819beb95')).toBe('0.5.5…9230819');
  expect(compactVersion('v1.13.0-beta.5')).toBe('v1.13.0-beta.5');
  expect(compactVersion('v1.2.3+dirty')).toBe('v1.2.3+dirty');
  expect(compactVersion('v1.2.3+build.20261126.4567890')).toBe('v1.2.3…4567890');
});
