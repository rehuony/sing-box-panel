import { useTranslation } from 'react-i18next';

import type { SubscriptionPreview } from '@/api/api-client';

import { Button } from '@/components/ui/button';
import { toast } from '@/components/ui/toast-manager';
import { describeRequestError } from '@/components/error-notice';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';

export function ChannelPreview({ preview, onClose }: { preview: SubscriptionPreview; onClose: () => void }) {
  const { t } = useTranslation();
  async function copy() {
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
          <DialogTitle>{t('channels.preview')}</DialogTitle>
          <DialogDescription className='sr-only'>{preview.result.format}</DialogDescription>
        </DialogHeader>
        <div className='channel-dialog-scroll'>
          <pre className='channel-preview-code' key={preview.result.format}>
            {preview.result.content}
          </pre>
          {preview.result.diagnostics.length > 0 && (
            <details>
              <summary>{t('channels.failedNodes', { count: preview.result.diagnostics.length })}</summary>
              <ul>
                {preview.result.diagnostics.map(issue => (
                  <li key={`${issue.collection}-${issue.item_index}-${issue.code}`}>
                    {issue.collection}
                    [
                    {issue.item_index}
                    ] ·
                    {issue.code}
                  </li>
                ))}
              </ul>
            </details>
          )}
        </div>
        <DialogFooter>
          <Button variant='secondary' onClick={() => void copy()}>
            {t('channels.copy')}
          </Button>
          <Button variant='secondary' onClick={onClose}>
            {t('common.close')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
