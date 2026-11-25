import type { PanelPreferences, PanelServiceSettings } from '@/api/api-client';

export function passwordError(password: string) {
  if (password === '') return undefined;
  const length = [...password].length;
  return length < 12 || length > 128 ? 'panelSettings.passwordLength' : undefined;
}

function validIP(value: string) {
  if (value.includes(':')) {
    try {
      return new URL(`http://[${value}]`).hostname !== '';
    } catch {
      return false;
    }
  }
  return /^\d+\.\d+\.\d+\.\d+$/.test(value) && value.split('.').every(part => Number(part) <= 255);
}

/** The id of the first field the server cannot accept, checked before saving. */
export function invalidSettingsField(
  p: PanelPreferences, s: PanelServiceSettings, github: string, email?: string, password = '',
): string | null {
  const integer = (value: number, min: number, max: number) => Number.isInteger(value) && value >= min && value <= max;
  if (p.listen_host !== 'localhost' && !validIP(p.listen_host)) return 'listen-host';
  if (!integer(p.listen_port, 1, 65535)) return 'listen-port';
  if (p.external_origin) {
    try {
      const url = new URL(p.external_origin);
      if (!['http:', 'https:'].includes(url.protocol) || url.origin !== p.external_origin) return 'origin';
    } catch {
      return 'origin';
    }
  }
  if (s.base_path && (!/^\/[\w./~-]+$/.test(s.base_path) || s.base_path.endsWith('/') || s.base_path.includes('//') || s.base_path.split('/').some(part => part === '.' || part === '..'))) return 'base-path';
  if (!s.data_dir.startsWith('/') || s.data_dir.includes('\0')) return 'data-dir';
  if (email !== undefined && (!/^[^\s@]+@[^\s@]+$/.test(email.trim()) || new TextEncoder().encode(email.trim()).length > 254)) return 'admin-email';
  if (passwordError(password)) return 'new-password';
  if (!/^#[\dA-F]{6}$/i.test(p.appearance.color)) return 'accent-color';
  if (!integer(p.appearance.radius, 0, 32)) return 'radius';
  if (!integer(s.sample_retention_days, 1, 366)) return 'sample-retention';
  if (!integer(s.runtime_history_retention_days ?? 90, 1, 3650)) return 'runtime-history-retention';
  if (!integer(s.core_log_retention_days ?? 7, 1, 3650)) return 'core-log-retention';
  if (!integer(s.core_log_max_files ?? 0, 0, 1024)) return 'core-log-max-files';
  if (!integer(s.core_log_max_file_size_mib ?? 32, 1, 1024)) return 'core-log-size';
  if (new TextEncoder().encode(github).length > 8192 || /[\0\r\n]/.test(github)) return 'github-token';
  if (!integer(s.catalog_refresh_interval_hours, 1, 720)) return 'catalog-refresh-interval';
  return null;
}
