import type { BackendModule, ResourceKey } from 'i18next';

import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';

export const LANGUAGE_STORAGE_KEY = 'sing-box-panel.language';
export const supportedLanguages = ['en', 'zh-CN'] as const;

export type SupportedLanguage = typeof supportedLanguages[number];

/** Reads each language's translations, which the application keeps in a chunk of their own. */
export type TranslationLoaders = Readonly<Record<SupportedLanguage, () => Promise<ResourceKey>>>;

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

/** Reads a language's translations when i18next first needs them. */
function translationBackend(loaders: TranslationLoaders): BackendModule {
  return {
    type: 'backend',
    init: () => undefined,
    read: (language, _namespace, callback) => {
      if (!isSupportedLanguage(language)) {
        callback(null, {});
        return;
      }
      loaders[language]().then(
        translations => callback(null, translations),
        (reason: unknown) => callback(reason instanceof Error ? reason : String(reason), null),
      );
    },
  };
}

/**
 * Initializes the application's i18next instance in the saved or browser
 * language and keeps the document language in step. Only that language's
 * translations are read, and another's when the language changes to it;
 * ready settles once the first language has loaded, so the application
 * renders after it.
 */
export function initializeI18n(loaders: TranslationLoaders): { i18n: typeof i18n; ready: Promise<unknown> } {
  const ready = i18n
    .use(translationBackend(loaders))
    .use(initReactI18next)
    .init({
      // Every language holds every key, which tests and types check, so none falls back to another.
      fallbackLng: false,
      // The language is set at once, so t answers as soon as its translations are there.
      initAsync: false,
      interpolation: { escapeValue: false },
      lng: resolveInitialLanguage(),
      load: 'currentOnly',
      nonExplicitSupportedLngs: false,
      supportedLngs: [...supportedLanguages],
    });
  i18n.on('languageChanged', synchronizeDocumentLanguage);
  synchronizeDocumentLanguage(i18n.resolvedLanguage ?? i18n.language);
  return { i18n, ready };
}

/** Changes the application language and remembers the choice. */
export async function setAppLanguage(language: SupportedLanguage): Promise<void> {
  const storage = browserStorage();
  try {
    storage?.setItem(LANGUAGE_STORAGE_KEY, language);
  } catch {
    // Language selection still applies when persistence is unavailable.
  }
  // A language whose translations cannot be read is not switched to, so no key shows in their place.
  await i18n.loadLanguages(language);
  if (i18n.hasResourceBundle(language, 'translation')) await i18n.changeLanguage(language);
}
