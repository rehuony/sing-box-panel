import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';

import { translationResources } from './resources';

export const LANGUAGE_STORAGE_KEY = 'sing-box-panel.language';
export const supportedLanguages = ['en', 'zh-CN'] as const;

export type SupportedLanguage = typeof supportedLanguages[number];

function isSupportedLanguage(value: string | null | undefined): value is SupportedLanguage {
  return supportedLanguages.includes(value as SupportedLanguage);
}

function browserStorage(): Pick<Storage, 'getItem' | 'setItem'> | null {
  try {
    return typeof window === 'undefined' ? null : window.localStorage;
  } catch {
    return null;
  }
}

function browserLanguages(): readonly string[] {
  if (typeof navigator === 'undefined') return [];
  return navigator.languages.length > 0 ? navigator.languages : [navigator.language];
}

export function resolveBrowserLanguage(languages: readonly string[]): SupportedLanguage {
  for (const candidate of languages) {
    const normalized = candidate.trim().toLowerCase();
    if (
      normalized === 'zh'
      || normalized === 'zh-cn'
      || normalized === 'zh-sg'
      || normalized.startsWith('zh-hans')
    ) {
      return 'zh-CN';
    }
    if (normalized === 'en' || normalized.startsWith('en-')) return 'en';
  }
  return 'en';
}

export function resolveInitialLanguage(
  storage: Pick<Storage, 'getItem'> | null = browserStorage(),
  languages: readonly string[] = browserLanguages(),
): SupportedLanguage {
  try {
    const stored = storage?.getItem(LANGUAGE_STORAGE_KEY);
    if (isSupportedLanguage(stored)) return stored;
  } catch {
    // Browser preference is still available when storage is blocked.
  }
  return resolveBrowserLanguage(languages);
}

function synchronizeDocumentLanguage(language: string) {
  if (typeof document !== 'undefined') {
    document.documentElement.lang = isSupportedLanguage(language) ? language : 'en';
  }
}

void i18n
  .use(initReactI18next)
  .init({
    fallbackLng: 'en',
    interpolation: { escapeValue: false },
    lng: resolveInitialLanguage(),
    nonExplicitSupportedLngs: false,
    resources: translationResources,
    supportedLngs: [...supportedLanguages],
  });

i18n.on('languageChanged', synchronizeDocumentLanguage);
synchronizeDocumentLanguage(i18n.resolvedLanguage ?? i18n.language);

export async function setAppLanguage(language: SupportedLanguage): Promise<void> {
  const storage = browserStorage();
  try {
    storage?.setItem(LANGUAGE_STORAGE_KEY, language);
  } catch {
    // Language selection still applies when persistence is unavailable.
  }
  await i18n.changeLanguage(language);
}

export default i18n;
