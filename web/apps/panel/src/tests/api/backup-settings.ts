import type { PanelSettingsView } from '@/api/api-client';

// The panel settings a backup carries for view, in the server's native form.
export function backupSettings(view: PanelSettingsView, secrets: { github: string; passwordHash: string }) {
  const p = view.preferences;
  const s = view.service;
  return {
    panel: {
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
    auth: { email: view.admin_email, password_hash: secrets.passwordHash },
    github: { token: secrets.github, catalog_refresh_interval_hours: s.catalog_refresh_interval_hours },
    traffic: {
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
