import { TriangleAlert } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@shared/components/ui/button';
import { toast } from '@shared/components/ui/toast-manager';
import { ScrollArea } from '@shared/components/ui/scroll-area';
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

import { nodePath } from '@/utils/node-path';
import { describeRequestError, toastFailure } from '@/components/error-notice';

import type { IssueGroup, IssueItem } from './issue-list';

import { IssueList } from './issue-list';
import { SubscriptionContentState } from './subscription-content-state';

type Diagnostic = SubscriptionPreview['result']['diagnostics'][number];

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
  const groups = useIssueGroups(issues, nodes);
  const failure = error ? describeRequestError(error) : '';
  const issueCount = issues.length + (failure ? 1 : 0);
  async function copy() {
    if (!preview?.result.content) return;
    try {
      await navigator.clipboard.writeText(preview.result.content);
      toast.add({ title: t('channels.copied'), type: 'success' });
    } catch (reason) {
      toastFailure(t('channels.copyFailed'), reason);
    }
  }
  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className='flex h-[min(640px,calc(100dvh-32px))] flex-col overflow-hidden' size='workspace'>
        <DialogHeader>
          <div className='flex flex-wrap items-center gap-2 pr-8'>
            <DialogTitle className='pr-0'>{t('channels.preview')}</DialogTitle>
            {issueCount > 0 && (
              <Popover>
                <PopoverTrigger render={<Button variant='ghost' size='sm' />} aria-label={t('channels.previewIssues', { count: issueCount })}>
                  <TriangleAlert data-icon='inline-start' className='text-[var(--color-warning-text)]' />
                  {t('channels.previewIssues', { count: issueCount })}
                </PopoverTrigger>
                <PopoverContent align='start' className='max-h-[min(22.5rem,var(--available-height))] w-[min(22.5rem,calc(100vw-2rem),var(--available-width))] gap-0 overflow-hidden p-0'>
                  <PopoverTitle className='sr-only'>{t('channels.previewIssues', { count: issueCount })}</PopoverTitle>
                  <ScrollArea>
                    {failure && (
                      <div className='grid gap-0.5 border-b px-3 py-2.5'>
                        <p className='font-medium'>{t('channels.previewFailed')}</p>
                        <p className='wrap-anywhere text-[var(--color-text-secondary)]'>{failure}</p>
                      </div>
                    )}
                    {groups.length > 0 && <IssueList groups={groups} />}
                  </ScrollArea>
                </PopoverContent>
              </Popover>
            )}
          </div>
          <DialogDescription className='sr-only'>{preview?.result.format ?? t('channels.preview')}</DialogDescription>
        </DialogHeader>
        <ScrollArea className='-mx-6 min-h-0 flex-1' contentClassName='flex flex-col px-6 py-0.5'>
          {preview
            ? <pre className='m-0 flex-1 rounded-[var(--radius-control)] border-none bg-[var(--color-surface-raised)] p-4 font-mono text-xs leading-[1.7] whitespace-pre text-foreground' key={preview.result.format}>{preview.result.content}</pre>
            : <SubscriptionContentState kind='preview' title={t('channels.previewFailed')} />}
        </ScrollArea>
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
 * The preview's issues grouped by cause, the fixed detail code where the
 * client knows it, else the code, biggest first. Each names its node: a pool
 * node by its display name, a fleet node's inbound by the fleet node with the
 * inbound's tag as its detail; a field path is the detail of a pool node. A
 * fleet node's issues open its configuration.
 */
function useIssueGroups(issues: readonly Diagnostic[], nodes: readonly SubscriptionNode[]): IssueGroup[] {
  const { t, i18n } = useTranslation();
  const known = (key: string | undefined) => key !== undefined && i18n.exists(`subscriptions.issues.${key}`);
  const pool = new Map(nodes.map(node => [node.id, node]));
  const byCause = new Map<string, IssueItem[]>();
  issues.forEach((issue, index) => {
    const cause = known(issue.reason) ? issue.reason! : issue.code;
    const node = issue.node_id === undefined ? undefined : pool.get(issue.node_id);
    const origin = issue.origin_node_name ?? issue.origin_node_id;
    let name = node?.display_name ?? origin ?? issue.node_name;
    name ??= issue.item_index >= 0 ? t('channels.previewNode', { position: issue.item_index + 1 }) : t('subscriptions.pool.diagnostics.unknownNode');
    const fleetNode = node?.origin.kind === 'inbound' ? node.origin.node_id : issue.origin_node_id;
    // A whole configuration's issue points at no field.
    const path = issue.item_index >= 0 ? issue.field_path : undefined;
    byCause.set(cause, [...byCause.get(cause) ?? [], {
      key: `${index}`,
      name,
      detail: node || !origin ? path : issue.node_name ?? path,
      to: fleetNode ? nodePath(fleetNode, 'configuration') : undefined,
    }]);
  });
  return [...byCause]
    .map(([cause, items]) => ({
      key: cause,
      title: known(cause) ? t(`subscriptions.issues.${cause}`) : t('subscriptions.issues.unknown', { code: cause }),
      items,
    }))
    .sort((left, right) => right.items.length - left.items.length);
}
