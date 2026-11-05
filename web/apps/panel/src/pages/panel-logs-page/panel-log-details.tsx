import { useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@shared/components/ui/button';
import { toast } from '@shared/components/ui/toast-manager';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@shared/components/ui/dialog';

import type { PanelLog } from '@/api/api-client';

import { PanelLogLevel } from '@/components/panel-log-level';
import { formatLogTime, logTitle } from '@/utils/panel-log-presentation';

import { metadataLabel, metadataValue } from './panel-log-presentation';

// Each row is a term and its value; below 480px the value moves under its term.
const detailList = 'm-0 grid gap-3 [&>div]:grid [&>div]:grid-cols-1 [&>div]:gap-1 min-[480px]:[&>div]:grid-cols-[6.25rem_minmax(0,1fr)] min-[480px]:[&>div]:gap-3 [&_dt]:text-muted-foreground';
const detailValue = 'm-0 min-w-0 whitespace-pre-wrap wrap-anywhere';

export function PanelLogDetails({ item, open, onOpenChange, onClosed, returnFocus }: {
  item: PanelLog | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onClosed: () => void;
  returnFocus: () => HTMLElement | null;
}) {
  const { t, i18n } = useTranslation();
  const titleRef = useRef<HTMLHeadingElement>(null);
  const language = i18n.resolvedLanguage;
  const raw = item ? JSON.stringify(item, null, 2) : '';
  async function copy() {
    try {
      await navigator.clipboard.writeText(raw);
      toast.add({ title: t('productLogs.copied'), type: 'success' });
    } catch {
      toast.add({ title: t('productLogs.copyFailed'), type: 'error' });
    }
  }
  return (
    <Dialog open={open} onOpenChange={onOpenChange} onOpenChangeComplete={value => {
      if (!value) onClosed();
    }}>
      <DialogContent className='flex w-220 max-w-[calc(100vw-2rem)] flex-col gap-0 overflow-hidden p-0' initialFocus={titleRef} finalFocus={returnFocus}>
        {item && (
          <>
            <DialogHeader className='shrink-0 gap-3 border-b p-4 sm:p-6'>
              <DialogTitle ref={titleRef} tabIndex={-1}>{t('productLogs.detailTitle')}</DialogTitle>
              <div className='flex items-baseline gap-3 wrap-anywhere [&>[data-slot=badge]]:shrink-0'>
                <PanelLogLevel level={item.level} />
                <DialogDescription className='line-clamp-2 wrap-anywhere'>{logTitle(item, t)}</DialogDescription>
              </div>
            </DialogHeader>
            <div className='grid min-h-0 gap-6 overflow-auto p-4 sm:p-6' tabIndex={0}>
              <div className='grid grid-cols-1 gap-6 md:grid-cols-2 [&>section]:min-w-0 [&>section+section]:border-t [&>section+section]:pt-6 md:[&>section+section]:border-t-0 md:[&>section+section]:border-l md:[&>section+section]:pt-0 md:[&>section+section]:pl-6'>
                <section aria-label={t('productLogs.basicInfo')}>
                  <h3 className='mb-3 text-sm font-semibold'>{t('productLogs.basicInfo')}</h3>
                  <dl className={detailList}>
                    <div>
                      <dt>{t('productLogs.columns.summary')}</dt>
                      <dd className={detailValue}>{logTitle(item, t)}</dd>
                    </div>
                    <div>
                      <dt>{t('productLogs.columns.time')}</dt>
                      <dd className='min-w-0 wrap-anywhere'><time dateTime={item.time}>{formatLogTime(item.time, language, true)}</time></dd>
                    </div>
                    <div>
                      <dt>{t('productLogs.columns.source')}</dt>
                      <dd className='min-w-0 wrap-anywhere'>{t(`productLogs.sources.${item.source}`, { defaultValue: item.source })}</dd>
                    </div>
                    {item.status && (
                      <div>
                        <dt>{t('productLogs.runtimeState')}</dt>
                        <dd className='min-w-0 wrap-anywhere'>{t(`productLogs.states.${item.status}`, { defaultValue: item.status })}</dd>
                      </div>
                    )}
                    <div>
                      <dt>{t('productLogs.eventCode')}</dt>
                      <dd className='min-w-0 wrap-anywhere'><code className='text-xs'>{item.code}</code></dd>
                    </div>
                    <div>
                      <dt>{t('productLogs.logID')}</dt>
                      <dd className='min-w-0 wrap-anywhere'><code className='text-xs'>{item.id}</code></dd>
                    </div>
                  </dl>
                </section>
                <section aria-label={t('productLogs.context')}>
                  <h3 className='mb-3 text-sm font-semibold'>{t('productLogs.context')}</h3>
                  {Object.keys(item.metadata).length
                    ? (
                        <dl className={detailList}>
                          {Object.entries(item.metadata).map(([key, value]) => (
                            <div key={key}>
                              <dt>{metadataLabel(key, t)}</dt>
                              <dd className={detailValue}>{metadataValue(key, value, item, t, language)}</dd>
                            </div>
                          ))}
                        </dl>
                      )
                    : <p className='m-0 text-muted-foreground'>{t('productLogs.noContext')}</p>}
                </section>
              </div>
              <details className='min-w-0 rounded-[var(--radius-control)] border p-3' key={item.id}>
                <summary className='font-medium'>{t('productLogs.rawRecord')}</summary>
                <pre className='mt-3 mb-0 font-mono text-xs whitespace-pre-wrap wrap-anywhere' tabIndex={0} aria-label={t('productLogs.rawRecord')}>{raw}</pre>
              </details>
            </div>
            <DialogFooter className='shrink-0 border-t px-4 py-4 sm:px-6' showCloseButton>
              <Button onClick={() => void copy()}>{t('productLogs.copyLog')}</Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}
