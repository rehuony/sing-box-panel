import type { PanelPreferences, PanelServiceSettings } from '@/api/api-client';

export const settingsCategories = ['service', 'traffic', 'logs', 'interface', 'maintenance', 'backup'] as const;
export type SettingsCategory = typeof settingsCategories[number];

export function managementTokenError(token: string) {
  const bytes = new TextEncoder().encode(token).length;
  return bytes < 8
    ? 'panelSettings.tokenTooShort'
    : bytes > 8192
      ? 'panelSettings.tokenTooLong'
      : /^[\s\u0085]|[\s\u0085]$/u.test(token) || /[\0\r\n]/.test(token)
        ? 'panelSettings.tokenInvalid'
        : undefined;
}

// Preserve links to sections that now belong to a broader category.
export const settingsHashValues = [...settingsCategories, 'access', 'authentication', 'publication', 'storage', 'updates', 'languageGroup'] as const;

export function resolveSettingsCategory(value: typeof settingsHashValues[number]): SettingsCategory {
  switch (value) {
    case 'access':
    case 'authentication':
    case 'publication':
      return 'service';
    case 'storage':
    case 'updates':
      return 'maintenance';
    case 'languageGroup':
      return 'interface';
    default:
      return value;
  }
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

function publicIPv4(parts: number[]) {
  const [a, b] = parts;
  return !(a === 10 || a === 127 || (a === 169 && b === 254)
    || (a === 172 && b >= 16 && b <= 31) || (a === 192 && b === 168)
    || (a >= 224 && a <= 239) || parts.every(n => n === 0) || parts.every(n => n === 255));
}

function validPublishedHost(host: string) {
  const ip = host.replace(/^\[|\]$/g, '');
  if (validIP(ip)) {
    if (!ip.includes(':')) return publicIPv4(ip.split('.').map(Number));
    const normalized = new URL(`http://[${ip}]`).hostname.slice(1, -1);
    if (normalized.startsWith('::ffff:')) {
      const tail = normalized.slice(7).split(':').map(part => Number.parseInt(part, 16));
      return publicIPv4([tail[0] >> 8, tail[0] & 255, tail[1] >> 8, tail[1] & 255]);
    }
    return !['::', '::1'].includes(normalized) && !/^(?:f[cd]|ff|fe[89ab])/i.test(normalized);
  }
  return host.length <= 253 && host.includes('.') && host.replace(/\.$/, '').split('.').every(label => /^[a-z\d](?:[a-z\d-]{0,61}[a-z\d])?$/i.test(label));
}

export function invalidSettingsField(
  p: PanelPreferences, s: PanelServiceSettings, github: string,
): { category: SettingsCategory; field: string } | null {
  const integer = (value: number, min: number, max: number) => Number.isInteger(value) && value >= min && value <= max;
  const fail = (category: SettingsCategory, field: string) => ({ category, field });
  if (p.listen_host !== 'localhost' && !validIP(p.listen_host)) return fail('service', 'listen-host');
  if (!integer(p.listen_port, 1, 65535)) return fail('service', 'listen-port');
  if (p.external_origin) {
    try {
      const url = new URL(p.external_origin);
      if (!['http:', 'https:'].includes(url.protocol) || url.origin !== p.external_origin) return fail('service', 'origin');
    } catch {
      return fail('service', 'origin');
    }
  }
  if (s.base_path && (!/^\/[\w./~-]+$/.test(s.base_path) || s.base_path.endsWith('/') || s.base_path.includes('//') || s.base_path.split('/').some(part => part === '.' || part === '..'))) return fail('service', 'base-path');
  if (s.secure_cookie !== p.external_origin.startsWith('https://')) return fail('service', 'secure-cookie');
  if (!s.data_dir.startsWith('/') || s.data_dir.includes('\0')) return fail('maintenance', 'data-dir');
  if (!integer(s.catalog_refresh_interval_hours, 1, 720)) return fail('maintenance', 'catalog-refresh-interval');
  if (new TextEncoder().encode(github).length > 8192 || /[\0\r\n]/.test(github)) return fail('maintenance', 'github-token');
  if (p.public_node_host && !validPublishedHost(p.public_node_host)) return fail('service', 'public-host');
  if (p.traffic_quota_gib !== null && !integer(p.traffic_quota_gib, 0, 8589934591)) return fail('traffic', 'quota');
  if (!integer(s.traffic_period_months, 1, 120)) return fail('traffic', 'traffic-period');
  if (!integer(s.sample_retention_days, 1, 366)) return fail('traffic', 'sample-retention');
  if (!integer(s.core_log_retention_days ?? 7, 1, 3650)) return fail('logs', 'core-log-retention');
  if (!integer(s.core_log_max_files ?? 0, 0, 1024)) return fail('logs', 'core-log-max-files');
  if (!integer(s.core_log_max_file_size_mib ?? 32, 1, 1024)) return fail('logs', 'core-log-size');
  if (!/^#[\dA-F]{6}$/i.test(p.appearance.color)) return fail('interface', 'accent-color');
  if (!integer(p.appearance.radius, 0, 32)) return fail('interface', 'radius');
  return null;
}
