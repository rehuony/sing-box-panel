import { TriangleAlert } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import type { SubscriptionPreview } from '@/api/api-client';

import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Separator } from '@/components/ui/separator';
import { toast } from '@/components/ui/toast-manager';
import { describeRequestError } from '@/components/error-notice';
import { Popover, PopoverContent, PopoverTitle, PopoverTrigger } from '@/components/ui/popover';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';

import { SubscriptionContentState } from './subscription-content-state';

interface Props {
  error?: unknown;
  onClose: () => void;
  preview: SubscriptionPreview | null;
}
export function ChannelPreview({ preview, error, onClose }: Props) {
  const { t } = useTranslation();
  const issues = preview?.result.diagnostics ?? [];
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
      <DialogContent className='channel-preview-dialog'>
        <DialogHeader>
          <div className='flex flex-wrap items-center gap-2 pr-8'>
            <DialogTitle className='pr-0'>{t('channels.preview')}</DialogTitle>
            {issueCount > 0 && (
              <Popover>
                <PopoverTrigger render={<Button variant='ghost' size='sm' />} aria-label={t('channels.previewIssues', { count: issueCount })}>
                  <TriangleAlert data-icon='inline-start' className='text-status-warning-foreground' />
                  {t('channels.previewIssues', { count: issueCount })}
                </PopoverTrigger>
                <PopoverContent align='start' className='channel-preview-issues'>
                  <PopoverTitle>{t('channels.previewIssues', { count: issueCount })}</PopoverTitle>
                  {failure && (
                    <div className='grid gap-1'>
                      <strong>{t('channels.previewFailed')}</strong>
                      <p className='wrap-anywhere'>{failure}</p>
                    </div>
                  )}
                  {issues.length > 0 && (
                    <ul className='grid gap-3'>
                      {issues.map((issue, index) => (
                        <li key={`${issue.node_id ?? issue.collection}-${issue.item_index}-${issue.code}`} className='min-w-0'>
                          {index > 0 && <Separator className='mb-3' />}
                          <PreviewIssue issue={issue} />
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
        <div className='channel-dialog-scroll'>
          {preview
            ? <pre className='channel-preview-code' key={preview.result.format}>{preview.result.content}</pre>
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

function PreviewIssue({ issue }: { issue: SubscriptionPreview['result']['diagnostics'][number] }) {
  const { t, i18n } = useTranslation();
  const detail = issue.reason && i18n.exists(`channels.previewReasons.${issue.reason}`) ? issue.reason : issue.code;
  const reason = i18n.exists(`channels.previewReasons.${detail}`) ? detail : 'unknown';
  const path = issue.field_path ?? `${issue.collection}[${issue.item_index}]`;
  return (
    <div className='grid gap-2 wrap-anywhere'>
      <div className='flex flex-wrap items-center gap-2'>
        <strong>{issue.node_name ?? t('channels.previewNode', { position: issue.item_index + 1 })}</strong>
        {issue.node_type && <Badge variant='secondary'>{issue.node_type}</Badge>}
        <span className='text-muted-foreground'>{t('channels.previewOmitted')}</span>
      </div>
      <p>{t(`channels.previewReasons.${reason}`, { format: issue.format === 'loon' ? 'Loon' : issue.format === 'mihomo' ? 'Mihomo' : 'sing-box' })}</p>
      <p className='text-muted-foreground'>{t(`channels.previewActions.${reason}`)}</p>
      <div className='grid gap-1 text-xs text-muted-foreground'>
        <span>
          {t('channels.previewLocation')}
          {' '}
          <code>{path}</code>
        </span>
        <span>
          {t('channels.previewCode')}
          {' '}
          <code>{issue.code}</code>
        </span>
      </div>
    </div>
  );
}
