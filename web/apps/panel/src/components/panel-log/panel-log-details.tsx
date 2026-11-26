import { useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@shared/components/ui/button';
import { toast } from '@shared/components/ui/toast-manager';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@shared/components/ui/dialog';

import type { LogEntry, NodeRuntimeTransition } from '@/api/api-client';

import { PanelLogLevel } from '@/components/panel-log-level';
import { formatLogTime, logTitle } from '@/utils/panel-log-presentation';

import { RuntimeStateBadge } from './runtime-state-badge';
import { metadataLabel, metadataValue } from './panel-log-presentation';

// Each row is a term and its value; below 480px the value moves under its term.
const detailList = 'm-0 grid gap-3 [&>div]:grid [&>div]:grid-cols-1 [&>div]:gap-1 min-[480px]:[&>div]:grid-cols-[6.25rem_minmax(0,1fr)] min-[480px]:[&>div]:gap-3 [&_dt]:text-muted-foreground';
const detailValue = 'm-0 min-w-0 whitespace-pre-wrap wrap-anywhere';
/** Context keys that hold an identifier or a digest; their values take the data face, like the record's own ID. */
const identifierKey = /(?:^|_)(?:id|sha256)$/;

/** What the details show: an entry of the log, or one of a node's runtime transitions. */
export type PanelLogRecord = { kind: 'log'; entry: LogEntry } | { kind: 'state'; transition: NodeRuntimeTransition };

/** A record's fields as the dialog shows them; a transition shows its node, state and process context. */
function present(record: PanelLogRecord) {
  if (record.kind === 'log') {
    const { entry } = record;
    return {
      code: entry.code, context: entry.metadata, id: entry.id, node: undefined, raw: entry,
      source: entry.source, state: undefined, time: entry.time, title: { code: entry.code, message: entry.message },
    };
  }
  const { activation_bundle_id, generation, node_id, pid, process_started_at, uncertain_since } = record.transition;
  const context = Object.fromEntries(
    Object.entries({ node_id, pid, process_started_at, generation, activation_bundle_id, uncertain_since })
      .filter(([, value]) => value !== undefined),
  );
  const { id, node_name, occurred_at, reason, state } = record.transition;
  return {
    code: reason, context, id: String(id), node: node_name, raw: record.transition,
    source: undefined, state, time: occurred_at, title: { code: reason },
  };
}

export function PanelLogDetails({ item, open, onOpenChange, onClosed, returnFocus }: {
  item: PanelLogRecord | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onClosed: () => void;
  returnFocus: () => HTMLElement | null;
}) {
  const { t, i18n } = useTranslation();
  const titleRef = useRef<HTMLHeadingElement>(null);
  const language = i18n.resolvedLanguage;
  const record = item ? present(item) : null;
  const raw = record ? JSON.stringify(record.raw, null, 2) : '';
  const log = item?.kind === 'log';
  async function copy() {
    try {
      await navigator.clipboard.writeText(raw);
      toast.add({ title: t(log ? 'productLogs.copied' : 'productLogs.recordCopied'), type: 'success' });
    } catch {
      toast.add({ title: t(log ? 'productLogs.copyFailed' : 'productLogs.recordCopyFailed'), type: 'error' });
    }
  }
  return (
    <Dialog open={open} onOpenChange={onOpenChange} onOpenChangeComplete={value => {
      if (!value) onClosed();
    }}>
      <DialogContent className='flex flex-col gap-0 overflow-hidden p-0' finalFocus={returnFocus} initialFocus={titleRef} size='workspace'>
        {item && record && (
          <>
            <DialogHeader className='shrink-0 gap-3 border-b p-4 sm:p-6'>
              <DialogTitle ref={titleRef} tabIndex={-1}>{t(log ? 'productLogs.detailTitle' : 'productLogs.stateDetailTitle')}</DialogTitle>
              <div className='flex items-baseline gap-3 wrap-anywhere [&>[data-slot=badge]]:shrink-0'>
                {item.kind === 'log' ? <PanelLogLevel level={item.entry.level} /> : <RuntimeStateBadge state={item.transition.state} />}
                <DialogDescription className='line-clamp-2 wrap-anywhere'>{logTitle(record.title, t)}</DialogDescription>
              </div>
            </DialogHeader>
            <div className='grid min-h-0 gap-6 overflow-auto p-4 sm:p-6' tabIndex={0}>
              <div className='grid grid-cols-1 gap-6 md:grid-cols-2 [&>section]:min-w-0 [&>section+section]:border-t [&>section+section]:pt-6 md:[&>section+section]:border-t-0 md:[&>section+section]:border-l md:[&>section+section]:pt-0 md:[&>section+section]:pl-6'>
                <section aria-label={t('productLogs.basicInfo')}>
                  <h3 className='mb-3 text-sm font-semibold'>{t('productLogs.basicInfo')}</h3>
                  <dl className={detailList}>
                    <div>
                      <dt>{t('productLogs.columns.summary')}</dt>
                      <dd className={detailValue}>{logTitle(record.title, t)}</dd>
                    </div>
                    <div>
                      <dt>{t('productLogs.columns.time')}</dt>
                      <dd className='min-w-0 wrap-anywhere'><time dateTime={record.time}>{formatLogTime(record.time, language, true)}</time></dd>
                    </div>
                    {record.source !== undefined && (
                      <div>
                        <dt>{t('productLogs.columns.source')}</dt>
                        <dd className='min-w-0 wrap-anywhere'>{t(`productLogs.sources.${record.source}`, { defaultValue: record.source })}</dd>
                      </div>
                    )}
                    {record.node !== undefined && (
                      <div>
                        <dt>{t('productLogs.stateColumns.node')}</dt>
                        <dd className='min-w-0 wrap-anywhere'>{record.node}</dd>
                      </div>
                    )}
                    {record.state !== undefined && (
                      <div>
                        <dt>{t('productLogs.runtimeState')}</dt>
                        <dd className='min-w-0 wrap-anywhere'>{t(`productLogs.states.${record.state}`)}</dd>
                      </div>
                    )}
                    <div>
                      <dt>{t('productLogs.eventCode')}</dt>
                      <dd className='min-w-0 wrap-anywhere'><code className='text-xs'>{record.code}</code></dd>
                    </div>
                    <div>
                      <dt>{t(log ? 'productLogs.logID' : 'productLogs.recordID')}</dt>
                      <dd className='min-w-0 wrap-anywhere'><code className='text-xs'>{record.id}</code></dd>
                    </div>
                  </dl>
                </section>
                <section aria-label={t('productLogs.context')}>
                  <h3 className='mb-3 text-sm font-semibold'>{t('productLogs.context')}</h3>
                  {Object.keys(record.context).length
                    ? (
                        <dl className={detailList}>
                          {Object.entries(record.context).map(([key, value]) => (
                            <div key={key}>
                              <dt>{metadataLabel(key, t)}</dt>
                              <dd className={detailValue}>
                                {identifierKey.test(key) && typeof value === 'string'
                                  ? <code className='text-xs'>{value}</code>
                                  : metadataValue(key, value, record.context, t, language)}
                              </dd>
                            </div>
                          ))}
                        </dl>
                      )
                    : <p className='m-0 text-muted-foreground'>{t('productLogs.noContext')}</p>}
                </section>
              </div>
              <details className='min-w-0 rounded-[var(--radius-control)] border p-3' key={record.id}>
                <summary className='font-medium'>{t('productLogs.rawRecord')}</summary>
                <pre className='mt-3 mb-0 font-mono text-xs whitespace-pre-wrap wrap-anywhere' tabIndex={0} aria-label={t('productLogs.rawRecord')}>{raw}</pre>
              </details>
            </div>
            <DialogFooter className='shrink-0 border-t px-4 py-4 sm:px-6' showCloseButton>
              <Button onClick={() => void copy()}>{t(log ? 'productLogs.copyLog' : 'productLogs.copyRecord')}</Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}
