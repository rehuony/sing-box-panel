import { useTranslation } from 'react-i18next';

/** A download and an upload value, each after its arrow in secondary text. */
export function UpDown({ download, upload }: { download: string; upload: string }) {
  const { t } = useTranslation();
  return (
    <span className='inline-flex min-w-0 flex-wrap gap-x-3'>
      <span className='whitespace-nowrap'>
        <span aria-hidden='true' className='text-[var(--color-text-secondary)]'>↓ </span>
        <span className='sr-only'>{t('telemetry.metric.download')}</span>
        {download}
      </span>
      <span className='whitespace-nowrap'>
        <span aria-hidden='true' className='text-[var(--color-text-secondary)]'>↑ </span>
        <span className='sr-only'>{t('telemetry.metric.upload')}</span>
        {upload}
      </span>
    </span>
  );
}
