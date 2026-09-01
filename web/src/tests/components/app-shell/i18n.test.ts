import { afterEach, describe, expect, it } from 'vitest';

import { translationResources } from '@/i18n/resources';
import i18n, {
  LANGUAGE_STORAGE_KEY,
  resolveBrowserLanguage,
  resolveInitialLanguage,
  setAppLanguage,
  supportedLanguages,
} from '@/i18n';

function collectLeafKeys(value: unknown, prefix = ''): string[] {
  if (value === null || typeof value !== 'object') return prefix === '' ? [] : [prefix];
  return Object.entries(value as Record<string, unknown>).flatMap(([key, child]) =>
    collectLeafKeys(child, prefix === '' ? key : `${prefix}.${key}`));
}

const requiredDynamicKeys = [
  ...['complete', 'partial', 'missing'].map((value) => `dashboard.coverage.${value}`),
  ...['running', 'stopped', 'failed', 'unknown'].map((value) => `dashboard.timeline.state.${value}`),
  ...['queued', 'running', 'succeeded', 'failed', 'canceled', 'superseded', 'canceling']
    .map((value) => `dashboard.tasks.status.${value}`),
  ...['trace', 'debug', 'info', 'warn', 'error', 'fatal']
    .map((value) => `observability.logs.levelOption.${value}`),
  ...['off', 'connecting', 'live', 'ended', 'error']
    .map((value) => `observability.logs.stream.${value}`),
  ...['start', 'stop', 'restart'].map((value) => `telemetry.control.${value}`),
  ...['stop', 'restart'].flatMap((action) =>
    ['title', 'description', 'action'].map((value) => `telemetry.confirm.${action}.${value}`)),
  ...['queued', 'running', 'succeeded', 'failed', 'canceled', 'superseded']
    .map((value) => `telemetry.taskStatus.${value}`),
  ...['payload', 'result', 'failure'].map((value) => `tasks.detail.${value}`),
];

describe('application language', () => {
  afterEach(async () => {
    window.localStorage.removeItem(LANGUAGE_STORAGE_KEY);
    await setAppLanguage('en');
    window.localStorage.removeItem(LANGUAGE_STORAGE_KEY);
  });

  it('uses the first supported browser language when no preference is saved', () => {
    const emptyStorage = { getItem: () => null };
    expect(resolveInitialLanguage(emptyStorage, ['fr-FR', 'zh-Hans-CN', 'en-US'])).toBe('zh-CN');
    expect(resolveInitialLanguage(emptyStorage, ['fr-FR', 'en-GB'])).toBe('en');
    expect(resolveBrowserLanguage(['zh-Hant-TW'])).toBe('en');
  });

  it('prefers only an explicitly persisted supported language', () => {
    expect(resolveInitialLanguage({ getItem: () => 'zh-CN' }, ['en-US'])).toBe('zh-CN');
    expect(resolveInitialLanguage({ getItem: () => 'fr-FR' }, ['zh-CN'])).toBe('zh-CN');
  });

  it('persists explicit language changes and synchronizes the document language', async () => {
    await setAppLanguage('zh-CN');

    expect(window.localStorage.getItem(LANGUAGE_STORAGE_KEY)).toBe('zh-CN');
    expect(i18n.resolvedLanguage).toBe('zh-CN');
    expect(document.documentElement.lang).toBe('zh-CN');
  });

  it('keeps the English and Simplified Chinese resource keysets identical', () => {
    const englishKeys = collectLeafKeys(translationResources.en.translation).sort();
    const chineseKeys = collectLeafKeys(translationResources['zh-CN'].translation).sort();

    expect(supportedLanguages).toEqual(['en', 'zh-CN']);
    expect(chineseKeys).toEqual(englishKeys);
    expect(new Set(englishKeys).size).toBe(englishKeys.length);
  });

  it.each(supportedLanguages)('provides every runtime-composed key in %s', (language) => {
    for (const key of requiredDynamicKeys) {
      expect(i18n.exists(key, { lng: language }), `${language} is missing ${key}`).toBe(true);
    }
  });
});
