import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { ChannelNativeTemplate, SubscriptionPreview } from '@/api/api-client';

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

import { ChannelPreview } from './channel-preview';

interface Props {
  onClose: () => void;
  format: 'sing-box' | 'mihomo';
  template?: ChannelNativeTemplate;
  onSave: (template: ChannelNativeTemplate) => Promise<void>;
  onPreview: (template: ChannelNativeTemplate) => Promise<SubscriptionPreview>;
}
export function ChannelTemplateEditor({ template, format, onClose, onPreview, onSave }: Props) {
  const { t } = useTranslation();
  const [content, setContent] = useState(
    template?.content ?? (format === 'sing-box' ? '{\n  "log": { "level": "info" }\n}' : 'log-level: info\n'),
  );
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [preview, setPreview] = useState<SubscriptionPreview | null>(null);
  const [validated, setValidated] = useState<string | null>(null);
  async function act(kind: 'validate' | 'preview' | 'save') {
    setBusy(true);
    setError('');
    try {
      const draft = { format, content };
      const result = await onPreview(draft);
      setValidated(content);
      if (kind === 'save') {
        await onSave(draft);
        onClose();
      } else if (kind === 'preview') {
        setPreview(result);
      } else {
        toast.add({ title: t('channels.valid'), type: 'success' });
      }
    } catch (reason) {
      setError(describeRequestError(reason));
      setValidated(null);
    } finally {
      setBusy(false);
    }
  }
  return (
    <Dialog open onOpenChange={(open) => !open && !busy && onClose()}>
      <DialogContent className='channel-template-dialog'>
        <DialogHeader>
          <DialogTitle>{t('channels.editTemplate')}</DialogTitle>
          <DialogDescription className='sr-only'>{format === 'sing-box' ? 'JSON' : 'YAML'}</DialogDescription>
        </DialogHeader>
        <textarea
          aria-label={t('channels.templateCode')}
          className='subscription-code-input channel-template-code'
          value={content}
          autoComplete='off'
          spellCheck={false}
          onChange={(event) => setContent(event.target.value)}
        />
        {error && (
          <p className='subscription-form-error' role='alert'>
            {error}
          </p>
        )}
        <DialogFooter>
          <Button variant='outline' disabled={busy} onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button
            variant='outline'
            disabled={busy || validated === content}
            onClick={() => void act('validate')}
          >
            {t('channels.validate')}
          </Button>
          <Button variant='outline' disabled={busy} onClick={() => void act('preview')}>
            {t('channels.preview')}
          </Button>
          <Button variant='default' disabled={busy} onClick={() => void act('save')}>
            {t('channels.saveTemplate')}
          </Button>
        </DialogFooter>
        {preview && <ChannelPreview preview={preview} onClose={() => setPreview(null)} />}
      </DialogContent>
    </Dialog>
  );
}
