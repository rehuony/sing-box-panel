import type { RJSFSchema } from '@rjsf/utils';

import { cn } from '@shared/lib/utils';
import { Link } from 'react-router-dom';
import { Braces, Copy } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@shared/components/ui/button';
import { toast } from '@shared/components/ui/toast-manager';
import { useEffect, useMemo, useRef, useState } from 'react';
import { createPrecompiledValidator } from '@rjsf/validator-ajv8';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@shared/components/ui/tabs';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@shared/components/ui/dialog';

import type { ReviewedSchemaResolution } from '@/schemas/resolve-reviewed-schema';
import type { SubscriptionNodeDetail, SubscriptionNodeSummary } from '@/api/api-client';

import { nodePath } from '@/utils/node-path';
import { useApiClient } from '@/api/api-client-context';
import { useUnsavedChanges } from '@/hooks/use-unsaved-changes';
import { loadReviewedSchema, reviewedSchemaManifest } from '@/schemas';
import { describeRequestError, ErrorNotice } from '@/components/error-notice';

import { SubscriptionNodeForm } from './subscription-node-form';
import { SubscriptionContentState } from './subscription-content-state';
import { collectionItemSchema, schemaProperties } from '../configuration-page/schema-ui';
import {
  encodeCanonicalValue,
  parseCanonicalDraft,
} from '../configuration-page/use-canonical-configuration';

const blank = encodeCanonicalValue({ type: 'socks', tag: '', server: '', server_port: 1080 }, 2);

function formatJSON(raw: string): string {
  try {
    return encodeCanonicalValue(parseCanonicalDraft(raw), 2);
  } catch {
    // Keep incomplete input intact until it can be parsed without losing data.
    return raw;
  }
}

interface NodeEditorProps {
  onClose: () => void;
  onSaved: () => void;
  node: SubscriptionNodeSummary | null;
  candidates?: SubscriptionNodeSummary[];
}

export function SubscriptionNodeEditor({
  node,
  candidates = [],
  onClose,
  onSaved,
}: NodeEditorProps) {
  const { t } = useTranslation();
  const client = useApiClient();
  const [detail, setDetail] = useState<SubscriptionNodeDetail | null>(null);
  const [raw, setRaw] = useState(blank);
  const [mode, setMode] = useState<'form' | 'json' | 'import'>(
    node?.origin === 'manual' || !node ? 'form' : 'json',
  );
  const [resolution, setResolution] = useState<ReviewedSchemaResolution | null>(null);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(node !== null);
  const [schemaLoading, setSchemaLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [importText, setImportText] = useState('');
  const [confirmDelete, setConfirmDelete] = useState(false);
  const activeRef = useRef(true);
  const editable = !node || node.origin === 'manual';
  const editorLoading = loading || (mode === 'form' && schemaLoading);
  const baseline = detail ? formatJSON(detail.outbound_json) : blank;
  useUnsavedChanges(editable && !loading && (raw !== baseline || importText !== ''), () => {
    setRaw(baseline);
    setImportText('');
    onClose();
  }, busy);
  useEffect(() => {
    activeRef.current = true;
    const controller = new AbortController();
    if (node) {
      void client
        .getSubscriptionNode(node.id, controller.signal)
        .then((value) => {
          if (controller.signal.aborted) return;
          setDetail(value);
          setRaw(formatJSON(value.outbound_json));
          setLoading(false);
        })
        .catch((reason) => {
          if (!controller.signal.aborted) {
            setError(describeRequestError(reason));
            setLoading(false);
          }
        });
    }
    const versions = Object.keys(reviewedSchemaManifest);
    const version = versions.sort((a, b) => b.localeCompare(a, undefined, { numeric: true }))[0];
    void loadReviewedSchema(version)
      .then((schema) => {
        if (!schema || controller.signal.aborted) return;
        setResolution({
          schema: schema.schema,
          createValidator: (value) =>
            createPrecompiledValidator(schema.validateFns as never, value),
        });
      })
      .catch((reason) => {
        if (!controller.signal.aborted) setError(describeRequestError(reason));
      })
      .finally(() => {
        if (!controller.signal.aborted) setSchemaLoading(false);
      });
    return () => {
      activeRef.current = false;
      controller.abort();
    };
  }, [client, node]);
  const parsed = useMemo(() => {
    try {
      return parseCanonicalDraft(raw);
    } catch {
      return null;
    }
  }, [raw]);
  const itemSchema = resolution
    ? collectionItemSchema(schemaProperties(resolution.schema).outbounds ?? {}, resolution.schema)
    : null;

  async function save() {
    if (!editable || !parsed || busy || (node && !detail)) return;
    setBusy(true);
    setError('');
    try {
      const formatted = encodeCanonicalValue(parsed, 2);
      setRaw(formatted);
      if (node) await client.updateSubscriptionNode(node.id, formatted, detail?.revision ?? 0);
      else await client.createSubscriptionNode(formatted);
      if (!activeRef.current) return;
      toast.add({ title: t('subscriptions.nodes.saved'), type: 'success' });
      onSaved();
      onClose();
    } catch (reason) {
      if (activeRef.current) setError(describeRequestError(reason));
    } finally {
      if (activeRef.current) setBusy(false);
    }
  }
  async function parseImport() {
    setBusy(true);
    setError('');
    try {
      const result = await client.parseSubscriptionNode(importText);
      if (!activeRef.current) return;
      setRaw(formatJSON(result.outbound_json));
      setMode('form');
    } catch (reason) {
      if (activeRef.current) setError(describeRequestError(reason));
    } finally {
      if (activeRef.current) setBusy(false);
    }
  }
  async function remove() {
    if (!detail || busy) return;
    setBusy(true);
    setError('');
    try {
      await client.deleteSubscriptionNode(detail.id, detail.revision ?? 0);
      if (!activeRef.current) return;
      toast.add({ title: t('subscriptions.nodes.deleted'), type: 'success' });
      onSaved();
      onClose();
    } catch (reason) {
      if (activeRef.current) setError(describeRequestError(reason));
    } finally {
      if (activeRef.current) setBusy(false);
    }
  }
  async function copyJSON() {
    try {
      await navigator.clipboard.writeText(raw);
      if (activeRef.current) toast.add({ title: t('subscriptions.nodes.copied'), type: 'success' });
    } catch (reason) {
      if (activeRef.current) toast.add({ title: describeRequestError(reason), type: 'error' });
    }
  }
  return (
    <Dialog
      onOpenChange={(open) => {
        if (!open && !busy) onClose();
      }}
      open
    >
      <DialogContent className={cn('[&_[data-slot=dialog-footer]]:mt-auto [&_[data-slot=dialog-footer]]:flex-none data-read-only:flex data-read-only:h-[min(440px,calc(100dvh-32px))] data-read-only:w-190 data-read-only:max-w-[calc(100vw-32px)] data-read-only:flex-col data-read-only:overflow-hidden sm:data-read-only:max-w-190', editable && 'configuration-entry-dialog')} data-read-only={!editable || undefined}>
        <DialogHeader>
          <DialogTitle>
            {confirmDelete
              ? t('subscriptions.nodes.delete')
              : node
                ? node.name
                : t('subscriptions.nodes.add')}
          </DialogTitle>
          <DialogDescription className='sr-only'>
            {t('subscriptions.nodes.editorDescription')}
          </DialogDescription>
        </DialogHeader>
        {error && (
          <ErrorNotice error={error} />
        )}
        {editorLoading
          ? <SubscriptionContentState loading />
          : node && !detail
            ? null
            : confirmDelete
              ? (
                  <p className='px-6'>{t('subscriptions.nodes.deletePrompt', { name: node?.name })}</p>
                )
              : (
                  <Tabs
                    className='min-h-0 flex-1 gap-4'
                    value={mode}
                    onValueChange={(value) => {
                      if (value === 'json') setRaw(formatJSON(raw));
                      setMode(value as typeof mode);
                    }}
                  >
                    {editable && (
                      <TabsList className='mx-6 max-w-[calc(100%-48px)] flex-none' aria-label={t('subscriptions.nodes.editor')}>
                        {(['form', 'json', ...(!node ? ['import'] : [])] as const).map((value) => (
                          <TabsTrigger disabled={busy} key={value} value={value}>
                            {t(`subscriptions.nodes.${value}`)}
                          </TabsTrigger>
                        ))}
                      </TabsList>
                    )}
                    <TabsContent
                      className='flex min-h-0 min-w-0 flex-1 flex-col overflow-auto overscroll-contain px-6 py-1 data-visual:overflow-hidden data-visual:p-0 in-data-read-only:-mx-6'
                      data-visual={(mode === 'form' && Boolean(parsed && itemSchema && resolution)) || undefined}
                      key={mode}
                      value={mode}
                      role={editable ? 'tabpanel' : 'presentation'}
                    >
                      {mode === 'import'
                        ? (
                            <textarea
                              aria-label={t('subscriptions.nodes.import')}
                              className='block min-h-70 w-full max-w-full overflow-auto rounded-[var(--radius-control)] border-none bg-[var(--color-surface-raised)] p-4 font-mono text-xs leading-[1.7] whitespace-pre text-foreground min-h-0 flex-1 resize-none'
                              onChange={(event) => setImportText(event.target.value)}
                              value={importText}
                            />
                          )
                        : mode === 'form' && parsed && itemSchema && resolution
                          ? (
                              <SubscriptionNodeForm
                                candidates={candidates
                                  .filter((value) => value.origin === 'manual' && value.id !== node?.id)
                                  .map((value) => value.tag)}
                                data={parsed}
                                disabled={busy}
                                onChange={setRaw}
                                resolution={resolution}
                                schema={itemSchema as RJSFSchema}
                              />
                            )
                          : (
                              <div className='flex min-h-0 flex-1 flex-col overflow-hidden rounded-[var(--radius-control)] bg-[var(--color-surface-raised)]'>
                                <div className='flex flex-none items-center justify-between gap-2 pt-1.5 pr-2 pl-4'>
                                  <span className='font-mono text-xs text-muted-foreground'>JSON</span>
                                  <div className='flex gap-1'>
                                    {editable && (
                                      <Button
                                        aria-label={t('configuration.advanced.format')}
                                        title={t('configuration.advanced.format')}
                                        disabled={busy || !parsed}
                                        onClick={() => setRaw(formatJSON(raw))}
                                        size='icon-xs'
                                        variant='ghost'
                                      >
                                        <Braces aria-hidden='true' />
                                      </Button>
                                    )}
                                    <Button
                                      aria-label={t('subscriptions.nodes.copy')}
                                      title={t('subscriptions.nodes.copy')}
                                      onClick={() => void copyJSON()}
                                      size='icon-xs'
                                      variant='ghost'
                                    >
                                      <Copy aria-hidden='true' />
                                    </Button>
                                  </div>
                                </div>
                                {editable
                                  ? (
                                      <textarea
                                        aria-label={t('subscriptions.nodes.json')}
                                        className='block min-h-70 w-full max-w-full overflow-auto rounded-[var(--radius-control)] border-none bg-[var(--color-surface-raised)] p-4 font-mono text-xs leading-[1.7] whitespace-pre text-foreground min-h-0 flex-1 resize-none pt-2'
                                        disabled={busy}
                                        onBlur={() => setRaw(formatJSON(raw))}
                                        onChange={(event) => setRaw(event.target.value)}
                                        spellCheck={false}
                                        value={raw}
                                      />
                                    )
                                  : (
                                      <pre aria-label={t('subscriptions.nodes.json')} className='block min-h-70 w-full max-w-full overflow-auto rounded-[var(--radius-control)] border-none bg-[var(--color-surface-raised)] p-4 font-mono text-xs leading-[1.7] whitespace-pre text-foreground m-0 min-h-0 flex-1 resize-none pt-2' role='region' tabIndex={0}>
                                        {raw}
                                      </pre>
                                    )}
                              </div>
                            )}
                    </TabsContent>
                  </Tabs>
                )}
        <DialogFooter>
          {confirmDelete
            ? (
                <>
                  <Button disabled={busy} onClick={() => setConfirmDelete(false)} variant='outline'>
                    {t('common.cancel')}
                  </Button>
                  <Button disabled={busy} onClick={() => void remove()} variant='destructive'>
                    {t('subscriptions.nodes.delete')}
                  </Button>
                </>
              )
            : (
                <>
                  <Button disabled={busy} onClick={onClose} variant='outline'>
                    {t('common.cancel')}
                  </Button>
                  {node?.origin === 'local' && node.node_id !== undefined && (
                    <Button
                      render={<Link to={`${nodePath(node.node_id, 'configuration')}?inbound=${encodeURIComponent(node.name)}`} />}
                      variant='default'
                    >
                      {t('node.tabs.configuration')}
                    </Button>
                  )}
                  {node?.origin === 'manual' && (
                    <Button
                      disabled={busy || !detail}
                      onClick={() => setConfirmDelete(true)}
                      variant='destructive'
                    >
                      {t('subscriptions.nodes.delete')}
                    </Button>
                  )}
                  {editable && (
                    <Button
                      disabled={busy || editorLoading || (node !== null && !detail) || (mode === 'import' ? !importText.trim() : !parsed)}
                      onClick={() => void (mode === 'import' ? parseImport() : save())}
                      variant='default'
                    >
                      {t(mode === 'import' ? 'subscriptions.nodes.parse' : 'subscriptions.nodes.save')}
                    </Button>
                  )}
                </>
              )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
