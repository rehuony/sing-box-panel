import { TriangleAlert } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import type { SubscriptionPreview } from '@/api/api-client';

import { Button } from '@/components/ui/button';
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
    if (!preview) return;
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
          <div className='flex items-center gap-3 pr-8'>
            <DialogTitle>{t('channels.preview')}</DialogTitle>
            {issueCount > 0 && (
              <Popover>
                <PopoverTrigger render={<Button variant='ghost' size='sm' />} aria-label={t('channels.previewIssues', { count: issueCount })}>
                  <TriangleAlert className='text-status-warning-foreground' />
                  {t('channels.previewIssues', { count: issueCount })}
                </PopoverTrigger>
                <PopoverContent align='start' className='max-h-72 w-80 overflow-y-auto'>
                  <PopoverTitle>{t('channels.previewIssues', { count: issueCount })}</PopoverTitle>
                  {failure && <p>{failure}</p>}
                  {issues.length > 0 && (
                    <ul className='grid gap-2'>
                      {issues.map(issue => (
                        <li key={`${issue.collection}-${issue.item_index}-${issue.code}`}>
                          {issue.collection}
                          [
                          {issue.item_index}
                          ] ·
                          {issue.code}
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
            : <p className='text-muted-foreground'>{t('channels.previewUnavailable')}</p>}

        </div>
        <DialogFooter>
          <Button variant='outline' disabled={!preview} onClick={() => void copy()}>
            {t('channels.copy')}
          </Button>
          <Button variant='outline' onClick={onClose}>
            {t('common.close')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
