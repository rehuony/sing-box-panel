import { initializeI18n } from '@shared/i18n/language';

import { en } from './locales/en';
import { zhCN } from './locales/zh-CN';

const i18n = initializeI18n({
  'en': { translation: en },
  'zh-CN': { translation: zhCN },
});

export default i18n;
