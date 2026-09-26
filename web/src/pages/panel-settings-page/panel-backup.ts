import type { PanelBackup } from '@/api/api-client';

function record(value: unknown): Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {};
}

export function backupFile(value: unknown): PanelBackup | null {
  const file = record(value);
  const settings = record(file.panel_settings);
  if (file.format !== 'sing-box-panel-backup' || file.version !== 1 || typeof file.exported_at !== 'string' || !Number.isFinite(Date.parse(file.exported_at)) || typeof file.sing_box_configuration !== 'string' || typeof settings.data_dir !== 'string' || typeof record(settings.auth).token !== 'string' || typeof record(settings.server).host !== 'string' || typeof record(settings.server).port !== 'number') return null;
  const backup = file as PanelBackup;
  if (new TextEncoder().encode(backup.sing_box_configuration).length > 2 * 1024 * 1024
    || new TextEncoder().encode(JSON.stringify(backup.panel_settings)).length > 1024 * 1024) {
    return null;
  }
  return backup;
}
