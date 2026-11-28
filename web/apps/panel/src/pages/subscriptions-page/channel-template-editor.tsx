import { useTranslation } from 'react-i18next';
import { Button } from '@shared/components/ui/button';
import { CircleCheck, Eye, RotateCcw } from 'lucide-react';
import { toast } from '@shared/components/ui/toast-manager';
import { lazy, Suspense, useEffect, useRef, useState } from 'react';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@shared/components/ui/dialog';

import type { CodeEditorLanguage } from '@/components/code-editor';
import type { ChannelNativeTemplate, SubscriptionNode, SubscriptionPreview } from '@/api/api-client';

import { toastFailure } from '@/components/error-notice';
import { LoadingState } from '@/components/loading-state';
import { useUnsavedChanges } from '@/hooks/use-unsaved-changes';
import { channelTemplateDefaults } from '@/constants/channel-templates';

import { ChannelPreview } from './channel-preview';

interface Props {
  onClose: () => void;
  template?: ChannelNativeTemplate;
  /** The pool, which names the nodes a preview leaves out. */
  nodes: readonly SubscriptionNode[];
  format: ChannelNativeTemplate['format'];
  onApply: (template: ChannelNativeTemplate) => void;
  onPreview: (template: ChannelNativeTemplate, signal: AbortSignal) => Promise<SubscriptionPreview>;
}

// CodeMirror loads with the first template opened, not with the page.
const CodeEditor = lazy(() => import('@/components/code-editor').then(module => ({ default: module.CodeEditor })));

const editorLanguage: Record<ChannelNativeTemplate['format'], CodeEditorLanguage> = { 'sing-box': 'json', 'mihomo': 'yaml', 'loon': 'plain' };
const languageName: Record<ChannelNativeTemplate['format'], string> = { 'sing-box': 'JSON', 'mihomo': 'YAML', 'loon': 'Loon' };
const formatLabels = { 'sing-box': 'singBox', 'mihomo': 'mihomo', 'loon': 'loon' } as const;

/**
 * A channel's native template in a code editor, its client and whether it is
 * the default beside the title. The editor's bar names the language and its
 * lines and holds the tools: restoring the default, validating and previewing
 * the subscription; the footer only cancels or applies.
 */
export function ChannelTemplateEditor({ template, format, nodes, onClose, onPreview, onApply }: Props) {
  const { t } = useTranslation();
  const fallback = channelTemplateDefaults[format];
  const baseline = template?.content ?? fallback;
  const [content, setContent] = useState(baseline);
  const [busy, setBusy] = useState(false);
  const [previewError, setPreviewError] = useState<unknown>(null);
  const [preview, setPreview] = useState<SubscriptionPreview | null>(null);
  const requestRef = useRef<AbortController | null>(null);
  useEffect(() => () => requestRef.current?.abort(), []);
  useUnsavedChanges(content !== baseline, onClose, busy);
  const lines = content.replace(/\n$/, '').split('\n').length;

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
          toastFailure(t('channels.templateCheckFailed'), reason);
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
      <DialogContent className='flex h-[min(640px,calc(100dvh-32px))] flex-col overflow-hidden' size='workspace'>
        <DialogHeader>
          <DialogTitle className='flex min-w-0 items-center gap-2'>
            {t('channels.editTemplate')}
            <span className='rounded-[var(--radius-pill)] border px-2 text-xs leading-5 font-normal whitespace-nowrap text-[var(--color-text-secondary)]'>
              {`${t(`subscriptions.channel.format.${formatLabels[format]}`)} · ${t(content === fallback ? 'channels.defaultTemplate' : 'channels.customTemplate')}`}
            </span>
          </DialogTitle>
          <DialogDescription>{t('channels.templateDraftHint')}</DialogDescription>
        </DialogHeader>
        <div className='flex min-h-0 flex-1 flex-col overflow-hidden rounded-lg border bg-[var(--color-surface-raised)]'>
          <div className='flex h-10 flex-none items-center gap-1 border-b ps-3 pe-1.5 text-xs text-muted-foreground'>
            <span className='font-[family-name:var(--font-data)] text-[var(--color-text-secondary)]'>{languageName[format]}</span>
            <span className='me-auto tabular-nums'>{`· ${t('channels.templateLines', { count: lines })}`}</span>
            <Button onClick={() => setContent(fallback)} size='xs' type='button' variant='flat'>
              <RotateCcw aria-hidden='true' data-icon='inline-start' />
              {t('channels.restoreDefault')}
            </Button>
            <span aria-hidden='true' className='mx-1 h-4 w-px bg-border' />
            <Button onClick={() => void act('validate')} size='xs' type='button' variant='flat'>
              <CircleCheck aria-hidden='true' data-icon='inline-start' />
              {t('channels.validate')}
            </Button>
            <Button onClick={() => void act('preview')} size='xs' type='button' variant='flat'>
              <Eye aria-hidden='true' data-icon='inline-start' />
              {t('channels.preview')}
            </Button>
          </div>
          <div className='min-h-0 flex-1'>
            <Suspense fallback={<LoadingState className='h-full' fullScreen={false} label={t('common.loading')} />}>
              <CodeEditor disabled={busy} label={t('channels.templateCode')} language={editorLanguage[format]} onChange={setContent} value={content} />
            </Suspense>
          </div>
        </div>
        <DialogFooter>
          <Button onClick={onClose} type='button' variant='outline'>{t('common.cancel')}</Button>
          <Button onClick={() => {
            if (busy) return;
            onApply({ format, content });
            onClose();
          }} type='button'>
            {t('channels.applyTemplate')}
          </Button>
        </DialogFooter>
        {(preview || previewError != null) && (
          <ChannelPreview nodes={nodes} preview={preview} error={previewError} onClose={() => {
            setPreview(null);
            setPreviewError(null);
          }} />
        )}
      </DialogContent>
    </Dialog>
  );
}
