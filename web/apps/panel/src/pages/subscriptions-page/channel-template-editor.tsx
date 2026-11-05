import { useTranslation } from 'react-i18next';
import { useEffect, useRef, useState } from 'react';
import { Button } from '@shared/components/ui/button';
import { toast } from '@shared/components/ui/toast-manager';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@shared/components/ui/dialog';

import type { ChannelNativeTemplate, SubscriptionPreview } from '@/api/api-client';

import { useUnsavedChanges } from '@/hooks/use-unsaved-changes';
import { describeRequestError } from '@/components/error-notice';
import { channelTemplateDefaults } from '@/constants/channel-templates';

import { ChannelPreview } from './channel-preview';

interface Props {
  onClose: () => void;
  template?: ChannelNativeTemplate;
  format: ChannelNativeTemplate['format'];
  onApply: (template: ChannelNativeTemplate) => void;
  onPreview: (template: ChannelNativeTemplate, signal: AbortSignal) => Promise<SubscriptionPreview>;
}
export function ChannelTemplateEditor({ template, format, onClose, onPreview, onApply }: Props) {
  const { t } = useTranslation();
  const baseline = template?.content ?? channelTemplateDefaults[format];
  const [content, setContent] = useState(baseline);
  const [busy, setBusy] = useState(false);
  const [previewError, setPreviewError] = useState<unknown>(null);
  const [preview, setPreview] = useState<SubscriptionPreview | null>(null);
  const requestRef = useRef<AbortController | null>(null);
  useEffect(() => () => requestRef.current?.abort(), []);
  useUnsavedChanges(content !== baseline, onClose, busy);
  async function act(kind: 'validate' | 'preview') {
    if (requestRef.current) return;
    const controller = new AbortController();
    requestRef.current = controller;
    setBusy(true);
    setPreviewError(null);
    try {
      const draft = { format, content };
      const result = await onPreview(draft, controller.signal);
      if (controller.signal.aborted) return;
      if (kind === 'preview') {
        setPreview(result);
      } else {
        toast.add({ title: t('channels.valid'), type: 'success' });
      }
    } catch (reason) {
      if (!controller.signal.aborted) {
        if (kind === 'preview') {
          setPreview(null);
          setPreviewError(reason);
        } else {
          toast.add({ title: describeRequestError(reason), type: 'error' });
        }
      }
    } finally {
      requestRef.current = null;
      if (!controller.signal.aborted) setBusy(false);
    }
  }
  return (
    <Dialog open onOpenChange={(open, details) => {
      if (busy) details.cancel();
      else if (!open) onClose();
    }}>
      <DialogContent className='flex h-[min(640px,calc(100dvh-32px))] w-240 max-w-[calc(100vw-32px)] flex-col overflow-hidden sm:max-w-240'>
        <DialogHeader>
          <DialogTitle>{t('channels.editTemplate')}</DialogTitle>
          <DialogDescription>{t('channels.templateDraftHint')}</DialogDescription>
        </DialogHeader>
        <textarea
          aria-label={t('channels.templateCode')}
          className='block w-full max-w-full overflow-auto rounded-[var(--radius-control)] border-none bg-[var(--color-surface-raised)] p-4 font-mono text-xs leading-[1.7] whitespace-pre text-foreground min-h-25 flex-1 resize-none'
          value={content}
          disabled={busy}
          autoComplete='off'
          spellCheck={false}
          onChange={(event) => setContent(event.target.value)}
        />
        <DialogFooter>
          <Button variant='outline' disabled={busy} onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button
            variant='outline'
            disabled={busy}
            onClick={() => void act('validate')}
          >
            {t('channels.validate')}
          </Button>
          <Button variant='outline' disabled={busy} onClick={() => void act('preview')}>
            {t('channels.preview')}
          </Button>
          <Button variant='default' disabled={busy} onClick={() => {
            onApply({ format, content });
            onClose();
          }}>
            {t('channels.applyTemplate')}
          </Button>
        </DialogFooter>
        {(preview || previewError != null) && (
          <ChannelPreview preview={preview} error={previewError} onClose={() => {
            setPreview(null);
            setPreviewError(null);
          }} />
        )}
      </DialogContent>
    </Dialog>
  );
}
