import type { TranslationLoaders } from '@shared/i18n/language';

/** Each language's translations, in a chunk of their own that loads when the language is first used. */
export const translationLoaders: TranslationLoaders = {
  'en': () => import('./locales/en').then(module => module.en),
  'zh-CN': () => import('./locales/zh-CN').then(module => module.zhCN),
};
