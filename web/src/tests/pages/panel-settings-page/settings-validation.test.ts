import { describe, expect, it } from 'vitest';

import { demoBackupSettings } from '@/api/demo/demo-panel-backup';
import { createMockApiClient } from '@/tests/api/mock-api-client';
import { backupFile } from '@/pages/panel-settings-page/panel-backup';
import { invalidSettingsField, managementTokenError, resolveSettingsCategory } from '@/pages/panel-settings-page/settings-categories';

describe('settings validation', () => {
  it.each([
    ['1234567', 'tokenTooShort'], ['ééé', 'tokenTooShort'],
    ['12345678 ', 'tokenInvalid'], ['\uFEFF12345678', 'tokenInvalid'],
    ['1234\0' + '5678', 'tokenInvalid'], ['1234\n5678', 'tokenInvalid'],
    ['x'.repeat(8193), 'tokenTooLong'],
  ])('rejects invalid management token %#', (token, code) => {
    expect(managementTokenError(token)).toBe(`panelSettings.${code}`);
  });
  it.each(['12345678', 'éééé', 'é'.repeat(4096)])('accepts a token inside UTF-8 byte boundaries %#', token => {
    expect(managementTokenError(token)).toBeUndefined();
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
      format: 'sing-box-panel-backup', version: 1, exported_at: '2026-09-23T01:00:00Z',
      panel_settings: demoBackupSettings(view, { github: '', management: 'test-token' }),
      sing_box_configuration: '  { unfinished raw text',
    };
  }
  it('preserves incomplete configuration text and accepts only the supported envelope', async () => {
    const backup = await fixture();
    expect(backupFile(backup)).toBe(backup);
    for (const value of [null, {}, [], { ...backup, version: 2 }, { ...backup, exported_at: 'invalid' }, { ...backup, panel_settings: {} }]) {
      expect(backupFile(value)).toBeNull();
    }
  });
  it('rejects oversized configuration or settings before preparing a restore', async () => {
    const backup = await fixture();
    expect(backupFile({ ...backup, sing_box_configuration: 'é'.repeat(1024 * 1024 + 1) })).toBeNull();
    expect(backupFile({ ...backup, panel_settings: { ...backup.panel_settings, extra: 'x'.repeat(1024 * 1024) } })).toBeNull();
  });
});
