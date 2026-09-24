import type { PanelBackup, PanelSettingsView } from '../api-client';

import { ApiRequestError } from '../api-client';

export interface DemoPanelSecrets { github: string; management: string }

export function demoBackupSettings(view: PanelSettingsView, secrets: DemoPanelSecrets) {
  const p = view.preferences;
  const s = view.service;
  return {
    panel: {
      public_node_host: p.public_node_host,
      language: p.language,
      appearance: p.appearance,
    },
    server: {
      host: p.listen_host,
      port: p.listen_port,
      external_origin: p.external_origin,
      base_path: s.base_path,
    },
    data_dir: s.data_dir,
    auth: { token: secrets.management, secure_cookie: s.secure_cookie },
    github: { token: secrets.github, catalog_refresh_interval_hours: s.catalog_refresh_interval_hours },
    traffic: {
      quota_gib: p.traffic_quota_gib,
      period_months: s.traffic_period_months,
      sample_retention_days: s.sample_retention_days,
    },
    subscription: {
      private_source_cidrs: s.private_source_cidrs,
    },
    logs: {
      core_retention_days: s.core_log_retention_days ?? 7,
      core_max_files: s.core_log_max_files ?? 0,
      core_max_file_size_mib: s.core_log_max_file_size_mib ?? 32,
    },
  };
}

export function demoRestoreSettings(
  backup: PanelBackup, revision: number,
): { view: PanelSettingsView; secrets: DemoPanelSecrets } {
  const native = backup.panel_settings as unknown as ReturnType<typeof demoBackupSettings>;
  if (backup.format !== 'sing-box-panel-backup' || backup.version !== 1 || !native?.server || !native.auth || !native.panel || !native.traffic || !native.github || !native.subscription || !native.logs || typeof backup.sing_box_configuration !== 'string' || typeof native.auth.token !== 'string') {
    throw new ApiRequestError('Invalid configuration backup.', { status: 422, code: 'panel_backup_invalid' });
  }
  if (!hasOnlyFields(native.panel, ['public_node_host', 'language', 'appearance'])
    || !hasOnlyFields(native.subscription, ['private_source_cidrs'])
    || !hasOnlyFields(native.logs, ['core_retention_days', 'core_max_files', 'core_max_file_size_mib'])) {
    throw new ApiRequestError('Invalid configuration backup.', { status: 422, code: 'panel_backup_invalid' });
  }
  return {
    secrets: { management: native.auth.token, github: native.github.token },
    view: {
      revision,
      preferences: {
        listen_host: native.server.host,
        listen_port: native.server.port,
        external_origin: native.server.external_origin,
        public_node_host: native.panel.public_node_host,
        traffic_quota_gib: native.traffic.quota_gib,
        language: native.panel.language,
        appearance: native.panel.appearance,
      },
      service: {
        data_dir: native.data_dir,
        base_path: native.server.base_path,
        secure_cookie: native.auth.secure_cookie,
        catalog_refresh_interval_hours: native.github.catalog_refresh_interval_hours,
        traffic_period_months: native.traffic.period_months,
        sample_retention_days: native.traffic.sample_retention_days,
        private_source_cidrs: native.subscription.private_source_cidrs,
        core_log_retention_days: native.logs.core_retention_days ?? 7,
        core_log_max_files: native.logs.core_max_files ?? 0,
        core_log_max_file_size_mib: native.logs.core_max_file_size_mib ?? 32,
      },
      github_token_configured: Boolean(native.github.token),
      restart_required: false,
    },
  };
}

function hasOnlyFields(value: unknown, fields: readonly string[]): boolean {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
    && Object.keys(value).every(key => fields.includes(key));
}
