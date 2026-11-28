import { cn } from '@shared/lib/utils';
import { useTranslation } from 'react-i18next';
import { trafficDirectionText, trafficValueText } from '@shared/lib/telemetry-format';

/**
 * A download and an upload value on one line, each after its arrow in its
 * direction's colour; a value takes that colour too unless it is zero. A
 * value not known reads 0. A line too narrow for both ends in an ellipsis,
 * the whole pair on hover.
 */
export function UpDown({ className, download, format, upload }: {
  download: number | null;
  upload: number | null;
  format: (value: number) => string;
  /** Classes of the line, such as its size. */
  className?: string;
}) {
  const { t } = useTranslation();
  const down = format(download ?? 0);
  const up = format(upload ?? 0);
  return (
    <span className={cn('block min-w-0 truncate', className)} title={`↓ ${down}  ↑ ${up}`}>
      <span aria-hidden='true' className={trafficDirectionText.download}>↓ </span>
      <span className='sr-only'>{t('telemetry.metric.download')}</span>
      <span className={trafficValueText('download', download)}>{down}</span>
      <span aria-hidden='true' className={cn('ms-3', trafficDirectionText.upload)}>↑ </span>
      <span className='sr-only'>{t('telemetry.metric.upload')}</span>
      <span className={trafficValueText('upload', upload)}>{up}</span>
    </span>
  );
}
