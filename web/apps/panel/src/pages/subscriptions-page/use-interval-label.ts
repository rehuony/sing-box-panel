import { useTranslation } from 'react-i18next';

/** The interval a URL source refreshes at, in its largest whole unit. */
export function useIntervalLabel() {
  const { t } = useTranslation();
  return (minutes: number) => {
    if (minutes === 0) return t('subscriptions.sources.interval.manual');
    if (minutes === 1440) return t('subscriptions.sources.interval.day');
    if (minutes === 60) return t('subscriptions.sources.interval.hour');
    if (minutes % 60 === 0) return t('subscriptions.sources.interval.hours', { count: minutes / 60 });
    return t('subscriptions.sources.interval.minutes', { count: minutes });
  };
}
