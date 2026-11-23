import { describe, expect, it } from 'vitest';

import { backupSettings } from '@/tests/api/backup-settings';
import { createMockApiClient } from '@/tests/api/mock-api-client';
import { backupFile } from '@/pages/panel-settings-page/panel-backup';
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

describe('backup import boundary', () => {
  async function fixture() {
    const view = await createMockApiClient().getPanelSettings();
    return {
      format: 'sing-box-panel-backup', version: 3, exported_at: '2026-09-23T01:00:00Z',
      panel_settings: backupSettings(view, { github: '', passwordHash: 'test-password-hash' }),
    };
  }
  it('accepts only the supported envelope', async () => {
    const backup = await fixture();
    expect(backupFile(backup)).toBe(backup);
    for (const value of [null, {}, [], { ...backup, version: 2 }, { ...backup, exported_at: 'invalid' }, { ...backup, panel_settings: {} }, {
      ...backup,
      panel_settings: { ...backup.panel_settings, auth: { ...backup.panel_settings.auth, secure_cookie: false } },
    }]) {
      expect(backupFile(value)).toBeNull();
    }
  });
  it('rejects oversized settings before preparing a restore', async () => {
    const backup = await fixture();
    expect(backupFile({ ...backup, panel_settings: { ...backup.panel_settings, extra: 'x'.repeat(1024 * 1024) } })).toBeNull();
  });
});
