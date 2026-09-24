import { useRef } from 'react';
import { useTranslation } from 'react-i18next';

import type { PanelLog } from '@/api/api-client';

import { Button } from '@/components/ui/button';
import { toast } from '@/components/ui/toast-manager';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';

import { PanelLogLevel } from './panel-log-level';
import { formatLogTime, logTitle, metadataLabel, metadataValue } from './panel-log-presentation';

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
      <DialogContent className='panel-log-dialog' initialFocus={titleRef} finalFocus={returnFocus}>
        {item && (
          <>
            <DialogHeader className='panel-log-dialog-header'>
              <DialogTitle ref={titleRef} tabIndex={-1}>{t('productLogs.detailTitle')}</DialogTitle>
              <div className='panel-log-dialog-summary'>
                <PanelLogLevel level={item.level} />
                <DialogDescription className='line-clamp-2 wrap-anywhere'>{logTitle(item, t)}</DialogDescription>
              </div>
            </DialogHeader>
            <div className='panel-log-dialog-body' tabIndex={0}>
              <div className='panel-log-detail-columns'>
                <section aria-label={t('productLogs.basicInfo')}>
                  <h3>{t('productLogs.basicInfo')}</h3>
                  <dl className='panel-log-detail'>
                    <div>
                      <dt>{t('productLogs.columns.summary')}</dt>
                      <dd className='panel-log-value'>{logTitle(item, t)}</dd>
                    </div>
                    <div>
                      <dt>{t('productLogs.columns.time')}</dt>
                      <dd><time dateTime={item.time}>{formatLogTime(item.time, language, true)}</time></dd>
                    </div>
                    <div>
                      <dt>{t('productLogs.columns.source')}</dt>
                      <dd>{t(`productLogs.sources.${item.source}`, { defaultValue: item.source })}</dd>
                    </div>
                    {item.status && (
                      <div>
                        <dt>{t('productLogs.runtimeState')}</dt>
                        <dd>{t(`productLogs.states.${item.status}`, { defaultValue: item.status })}</dd>
                      </div>
                    )}
                    <div>
                      <dt>{t('productLogs.eventCode')}</dt>
                      <dd><code>{item.code}</code></dd>
                    </div>
                    <div>
                      <dt>{t('productLogs.logID')}</dt>
                      <dd><code>{item.id}</code></dd>
                    </div>
                  </dl>
                </section>
                <section aria-label={t('productLogs.context')}>
                  <h3>{t('productLogs.context')}</h3>
                  {Object.keys(item.metadata).length
                    ? (
                        <dl className='panel-log-detail'>
                          {Object.entries(item.metadata).map(([key, value]) => (
                            <div key={key}>
                              <dt>{metadataLabel(key, t)}</dt>
                              <dd className='panel-log-value'>{metadataValue(key, value, item, t, language)}</dd>
                            </div>
                          ))}
                        </dl>
                      )
                    : <p className='panel-log-muted'>{t('productLogs.noContext')}</p>}
                </section>
              </div>
              <details className='panel-log-raw' key={item.id}>
                <summary>{t('productLogs.rawRecord')}</summary>
                <pre tabIndex={0} aria-label={t('productLogs.rawRecord')}>{raw}</pre>
              </details>
            </div>
            <DialogFooter className='panel-log-dialog-footer' showCloseButton>
              <Button variant='secondary' onClick={() => void copy()}>{t('productLogs.copyLog')}</Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}
