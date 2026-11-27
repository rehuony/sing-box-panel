import { describe, expect, it } from 'vitest';

import { createMockApiClient } from '@/tests/api/mock-api-client';
import { invalidSettingsField, passwordError } from '@/pages/panel-settings-page/settings-validation';

describe('settings validation', () => {
  it.each(['short', '密'.repeat(11), 'x'.repeat(129)])('rejects passwords outside character boundaries', password => {
    expect(passwordError(password)).toBe('panelSettings.passwordLength');
  });
  it.each(['', '密'.repeat(12), '密'.repeat(128), '  password with spaces  '])('accepts valid passwords or unchanged input', password => {
    expect(passwordError(password)).toBeUndefined();
  });
  it('returns the field that prevents submission', async () => {
    const view = await createMockApiClient().getPanelSettings();
    expect(invalidSettingsField(view.preferences, view.service, '')).toBeNull();
    expect(invalidSettingsField({ ...view.preferences, external_origin: 'https://panel.example.com' }, view.service, '')).toBeNull();
    expect(invalidSettingsField(view.preferences, { ...view.service, catalog_refresh_interval_hours: 0 }, ''))
      .toBe('catalog-refresh-interval');
    expect(invalidSettingsField(view.preferences, { ...view.service, sample_retention_days: 0 }, ''))
      .toBe('sample-retention');
  });
});
