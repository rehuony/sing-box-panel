import type { TFunction } from 'i18next';

import type { ApiClient, PanelBackupCategory, PanelBackupCounts, PanelRestorePlan } from '@/api/api-client';

/** Every category in the order they are shown. */
export const BACKUP_CATEGORIES: readonly PanelBackupCategory[] = ['settings', 'nodes', 'subscriptions', 'rule_sets', 'keys', 'history', 'logs'];

/** What a backup holds unless asked otherwise: everything but the log. */
export const DEFAULT_CATEGORIES: readonly PanelBackupCategory[] = BACKUP_CATEGORIES.filter(category => category !== 'logs');

/** Downloads a stored backup with the session's cookie. */
export function downloadBackup(client: ApiClient, name: string) {
  const anchor = document.createElement('a');
  anchor.href = client.panelBackupDownloadUrl(name);
  anchor.download = name;
  anchor.click();
}

/**
 * What a backup holds, as its row says it: nothing for the usual backup,
 * the log when that is all it adds, else the categories it holds.
 */
export function contentsLabel(t: TFunction, categories: readonly PanelBackupCategory[]) {
  if (categories.length === 0) return t('panelSettings.backupUnreadable');
  const usual = DEFAULT_CATEGORIES.every(category => categories.includes(category));
  if (usual) return categories.includes('logs') ? t('panelSettings.backupWithLogs') : '';
  return categoryList(t, categories);
}

export function categoryList(t: TFunction, categories: readonly PanelBackupCategory[]) {
  return categories.map(category => t(`panelSettings.backupCategory.${category}`)).join(t('panelSettings.backupListSeparator'));
}

/** The records a backup holds of one category, as its tile counts them; empty for the settings. */
export function categoryCount(t: TFunction, locale: string, category: PanelBackupCategory, counts: PanelBackupCounts) {
  const number = new Intl.NumberFormat(locale);
  switch (category) {
    case 'settings': return '';
    case 'nodes': return number.format(counts.nodes);
    case 'subscriptions': return `${number.format(counts.sources)} · ${number.format(counts.channels)}`;
    case 'rule_sets': return number.format(counts.rule_sets);
    case 'keys': return number.format(counts.keys);
    case 'history': return t('panelSettings.backupHistoryDays', { count: counts.history_days });
    case 'logs': return number.format(counts.log_entries);
  }
}

/** What a partial restore changes beyond its categories, in one line; empty when nothing. */
export function effectsLine(t: TFunction, effects: PanelRestorePlan['effects']) {
  return (Object.keys(effects) as (keyof PanelRestorePlan['effects'])[])
    .filter(effect => effects[effect] > 0)
    .map(effect => t(`panelSettings.backupEffect.${effect}`, { count: effects[effect] }))
    .join(t('panelSettings.backupEffectSeparator'));
}
