import { initializeI18n } from '@shared/i18n/language';

const { i18n, ready } = initializeI18n({
  'en': () => import('./locales/en').then(module => module.en),
  'zh-CN': () => import('./locales/zh-CN').then(module => module.zhCN),
});

/** Settles once the saved or browser language's translations have loaded; the application renders after it. */
export const i18nReady = ready;

export default i18n;
