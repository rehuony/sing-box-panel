import { describe, expect, it } from 'vitest';

import { demoBackupSettings } from '@/api/demo/demo-panel-backup';
import { createMockApiClient } from '@/tests/api/mock-api-client';
import { backupFile } from '@/pages/panel-settings-page/panel-backup';
import { invalidSettingsField, passwordError, resolveSettingsCategory } from '@/pages/panel-settings-page/settings-categories';

describe('settings validation', () => {
  it.each(['short', '密'.repeat(11), 'x'.repeat(129)])('rejects passwords outside character boundaries', password => {
    expect(passwordError(password)).toBe('panelSettings.passwordLength');
  });
  it.each(['', '密'.repeat(12), '密'.repeat(128), '  password with spaces  '])('accepts valid passwords or unchanged input', password => {
    expect(passwordError(password)).toBeUndefined();
  });
  it.each([
    ['access', 'service'], ['authentication', 'service'], ['publication', 'service'],
    ['storage', 'maintenance'], ['updates', 'maintenance'], ['languageGroup', 'interface'],
  ] as const)('maps the retained %s hash to %s', (hash, category) => {
    expect(resolveSettingsCategory(hash)).toBe(category);
  });
  it('returns the category and field that prevent submission', async () => {
    const view = await createMockApiClient().getPanelSettings();
    expect(invalidSettingsField(view.preferences, view.service, '')).toBeNull();
    expect(invalidSettingsField({ ...view.preferences, external_origin: 'https://panel.example.com' }, view.service, '')).toBeNull();
    expect(invalidSettingsField({ ...view.preferences, public_node_host: 'https://invalid.example.com' }, view.service, ''))
      .toEqual({ category: 'service', field: 'public-host' });
    expect(invalidSettingsField(view.preferences, { ...view.service, catalog_refresh_interval_hours: 0 }, ''))
      .toEqual({ category: 'maintenance', field: 'catalog-refresh-interval' });
    expect(invalidSettingsField(view.preferences, { ...view.service, traffic_period_months: 0 }, ''))
      .toEqual({ category: 'traffic', field: 'traffic-period' });
  });
});

describe('backup import boundary', () => {
  async function fixture() {
    const view = await createMockApiClient().getPanelSettings();
    return {
      format: 'sing-box-panel-backup', version: 2, exported_at: '2026-09-23T01:00:00Z',
      panel_settings: demoBackupSettings(view, { github: '', passwordHash: 'demo-password-hash' }),
      sing_box_configuration: '  { unfinished raw text',
    };
  }
  it('preserves incomplete configuration text and accepts only the supported envelope', async () => {
    const backup = await fixture();
    expect(backupFile(backup)).toBe(backup);
    for (const value of [null, {}, [], { ...backup, version: 1 }, { ...backup, exported_at: 'invalid' }, { ...backup, panel_settings: {} }, {
      ...backup,
      panel_settings: { ...backup.panel_settings, auth: { ...backup.panel_settings.auth, secure_cookie: false } },
    }]) {
      expect(backupFile(value)).toBeNull();
    }
  });
  it('rejects oversized configuration or settings before preparing a restore', async () => {
    const backup = await fixture();
    expect(backupFile({ ...backup, sing_box_configuration: 'é'.repeat(1024 * 1024 + 1) })).toBeNull();
    expect(backupFile({ ...backup, panel_settings: { ...backup.panel_settings, extra: 'x'.repeat(1024 * 1024) } })).toBeNull();
  });
});
