import { initializeI18n } from '@shared/i18n/language';

import { translationLoaders } from './resources';

const { i18n, ready } = initializeI18n(translationLoaders);

/** Settles once the saved or browser language's translations have loaded; the application renders after it. */
export const i18nReady = ready;

export default i18n;
