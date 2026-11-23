import { TriangleAlert } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@shared/components/ui/button';
import { Separator } from '@shared/components/ui/separator';
import { toast } from '@shared/components/ui/toast-manager';
import { Popover, PopoverContent, PopoverTitle, PopoverTrigger } from '@shared/components/ui/popover';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@shared/components/ui/dialog';

import type { SubscriptionNode, SubscriptionPreview } from '@/api/api-client';

import { describeRequestError } from '@/components/error-notice';

import { SubscriptionContentState } from './subscription-content-state';

type Diagnostic = SubscriptionPreview['result']['diagnostics'][number];

/** The affected nodes an issue entry names before it gives only their count. */
const NAMED_NODES = 10;

interface Props {
  error?: unknown;
  onClose: () => void;
  /** The pool, which names the affected nodes. */
  nodes: readonly SubscriptionNode[];
  preview: SubscriptionPreview | null;
}
export function ChannelPreview({ preview, error, nodes, onClose }: Props) {
  const { t } = useTranslation();
  const issues = preview?.result.diagnostics ?? [];
  const byCode = new Map<string, Diagnostic[]>();
  for (const issue of issues) byCode.set(issue.code, [...byCode.get(issue.code) ?? [], issue]);
  const failure = error ? describeRequestError(error) : '';
  const issueCount = issues.length + (failure ? 1 : 0);
  async function copy() {
    if (!preview?.result.content) return;
    try {
      await navigator.clipboard.writeText(preview.result.content);
      toast.add({ title: t('channels.copied'), type: 'success' });
    } catch (reason) {
      toast.add({ title: describeRequestError(reason), type: 'error' });
    }
  }
  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className='flex h-[min(640px,calc(100dvh-32px))] w-240 max-w-[calc(100vw-32px)] flex-col overflow-hidden sm:max-w-240'>
        <DialogHeader>
          <div className='flex flex-wrap items-center gap-2 pr-8'>
            <DialogTitle className='pr-0'>{t('channels.preview')}</DialogTitle>
            {issueCount > 0 && (
              <Popover>
                <PopoverTrigger render={<Button variant='ghost' size='sm' />} aria-label={t('channels.previewIssues', { count: issueCount })}>
                  <TriangleAlert data-icon='inline-start' className='text-[var(--color-warning-text)]' />
                  {t('channels.previewIssues', { count: issueCount })}
                </PopoverTrigger>
                <PopoverContent align='start' className='max-h-[min(32rem,var(--available-height))] w-[min(28rem,calc(100vw-2rem),var(--available-width))] overflow-y-auto overscroll-contain p-4'>
                  <PopoverTitle>{t('channels.previewIssues', { count: issueCount })}</PopoverTitle>
                  {failure && (
                    <div className='grid gap-1'>
                      <strong>{t('channels.previewFailed')}</strong>
                      <p className='wrap-anywhere'>{failure}</p>
                    </div>
                  )}
                  {byCode.size > 0 && (
                    <ul className='grid gap-3'>
                      {[...byCode].map(([code, group], index) => (
                        <li key={code} className='min-w-0'>
                          {index > 0 && <Separator className='mb-3' />}
                          <PreviewIssue code={code} issues={group} nodes={nodes} />
                        </li>
                      ))}
                    </ul>
                  )}
                </PopoverContent>
              </Popover>
            )}
          </div>
          <DialogDescription className='sr-only'>{preview?.result.format ?? t('channels.preview')}</DialogDescription>
        </DialogHeader>
        <div className='-mx-6 flex min-h-0 flex-1 flex-col overflow-auto overscroll-contain px-6 py-0.5'>
          {preview
            ? <pre className='m-0 min-h-full overflow-auto rounded-[var(--radius-control)] border-none bg-[var(--color-surface-raised)] p-4 font-mono text-xs leading-[1.7] whitespace-pre text-foreground' key={preview.result.format}>{preview.result.content}</pre>
            : <SubscriptionContentState kind='preview' title={t('channels.previewFailed')} />}

        </div>
        <DialogFooter>
          <Button variant='outline' onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button disabled={!preview?.result.content} onClick={() => void copy()}>
            {t('channels.copy')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/**
 * The issues of one code: why the nodes were left out and what to do, said
 * once, with their count and the nodes by their pool names.
 */
function PreviewIssue({ code, issues, nodes }: {
  code: string;
  issues: Diagnostic[];
  nodes: readonly SubscriptionNode[];
}) {
  const { t, i18n } = useTranslation();
  const known = (key: string) => i18n.exists(`channels.previewReasons.${key}`);
  const reasons = new Set(issues.map(issue => issue.reason && known(issue.reason) ? issue.reason : code));
  const shared = reasons.size === 1 ? [...reasons][0] : code;
  const reason = known(shared) ? shared : 'unknown';
  const pool = new Map(nodes.map(node => [node.id, node.display_name]));
  const names = [...new Set(issues.map(issue => (issue.node_id === undefined ? undefined : pool.get(issue.node_id))
    ?? issue.node_name ?? issue.node_id ?? t('channels.previewNode', { position: issue.item_index + 1 })))];
  const shown = names.slice(0, NAMED_NODES).join(t('subscriptions.sources.protocolSeparator'));
  return (
    <div className='grid gap-2 wrap-anywhere'>
      <div className='flex items-baseline gap-2'>
        <strong className='min-w-0 flex-1'>{t(`channels.previewReasons.${reason}`, { format: issues[0].format })}</strong>
        <span className='flex-none font-[family-name:var(--font-data)] text-xs text-muted-foreground tabular-nums'>{issues.length}</span>
      </div>
      <p className='text-muted-foreground'>{t(`channels.previewActions.${reason}`)}</p>
      <p>
        {shown}
        {names.length > NAMED_NODES ? ` ${t('channels.previewMoreNodes', { total: names.length, rest: names.length - NAMED_NODES })}` : ''}
      </p>
      <code className='text-xs text-muted-foreground'>{code}</code>
    </div>
  );
}
