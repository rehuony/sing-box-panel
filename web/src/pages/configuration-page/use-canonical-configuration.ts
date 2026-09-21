import type { TFunction } from 'i18next';

import { useTranslation } from 'react-i18next';
import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  isLosslessNumber,
  parse as parseLosslessJSON,
  stringify as stringifyLosslessJSON,
} from 'lossless-json';

import type { ConfigurationFile } from '@/api/api-client';

import { toast } from '@/components/ui/toast-manager';
import { useApiClient } from '@/api/api-client-context';
import { useUnsavedChanges } from '@/hooks/use-unsaved-changes';
import { useCanonicalDraftSession } from '@/stores/canonical-draft.store';

export interface CanonicalDraft {
  [key: string]: unknown;
}

type CanonicalDraftErrorCode = 'notEncoded' | 'object';

class CanonicalDraftError extends Error {
  constructor(readonly code: CanonicalDraftErrorCode) {
    super(code);
    this.name = 'CanonicalDraftError';
  }
}

export function localizeCanonicalDraftError(error: unknown, t: TFunction): unknown {
  return error instanceof CanonicalDraftError
    ? new Error(t(`configuration.advanced.error.${error.code}`))
    : error;
}

export function parseCanonicalDraft(documentJSON: string): CanonicalDraft {
  const parsed = parseLosslessJSON(documentJSON);
  if (parsed === null || Array.isArray(parsed) || typeof parsed !== 'object' || isLosslessNumber(parsed)) {
    throw new CanonicalDraftError('object');
  }
  return parsed as CanonicalDraft;
}

export function encodeCanonicalValue(value: unknown, indentation?: number): string {
  const encoded = stringifyLosslessJSON(value, null, indentation);
  if (encoded === undefined) throw new CanonicalDraftError('notEncoded');
  return encoded;
}

export function encodeCanonicalDraft(draft: CanonicalDraft, indentation?: number): string {
  return encodeCanonicalValue(draft, indentation);
}

type FileState
  = | { status: 'loading'; file: null; content: string; error: null }
    | { status: 'error'; file: null; content: string; error: unknown }
    | { status: 'ready'; file: ConfigurationFile; content: string; error: null };

export function useCanonicalConfiguration() {
  const client = useApiClient();
  const { t } = useTranslation();
  const session = useCanonicalDraftSession();
  const [restored] = useState(() => session.current?.dirty ? session.current : null);
  const [state, setState] = useState<FileState>(() => restored === null
    ? { status: 'loading', file: null, content: '', error: null }
    : { status: 'ready', file: restored.file, content: restored.content, error: null });
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (restored !== null) return;
    const controller = new AbortController();
    void client.getConfigurationFile(controller.signal).then(file => {
      if (!controller.signal.aborted) setState({ status: 'ready', file, content: file.content, error: null });
    }).catch((error: unknown) => {
      if (!controller.signal.aborted) setState({ status: 'error', file: null, content: '', error });
    });
    return () => controller.abort();
  }, [client, restored]);

  const parsed = useMemo(() => {
    try {
      return { draft: parseCanonicalDraft(state.content), error: null };
    } catch (error) {
      return { draft: null, error: localizeCanonicalDraftError(error, t) };
    }
  }, [state.content, t]);
  const dirty = state.status === 'ready' && state.content !== state.file.content;
  useEffect(() => {
    if (state.status === 'ready') session.current = { file: state.file, content: state.content, dirty };
  }, [dirty, session, state]);

  const updateText = useCallback((content: string) => {
    if (!saving) setState(current => current.status === 'ready' ? { ...current, content } : current);
  }, [saving]);
  const update = useCallback((change: (draft: CanonicalDraft) => CanonicalDraft) => {
    if (saving) return;
    setState(current => {
      if (current.status !== 'ready') return current;
      try {
        return { ...current, content: encodeCanonicalDraft(change(parseCanonicalDraft(current.content)), 2) };
      } catch {
        return current;
      }
    });
  }, [saving]);

  const save = useCallback(async () => {
    if (state.status !== 'ready' || saving) return null;
    setSaving(true);
    try {
      const file = await client.saveConfigurationFile({ revision: state.file.revision, content: state.content });
      setState({ status: 'ready', file, content: file.content, error: null });
      toast.add({ title: t('configuration.file.saved'), type: 'success' });
      return file;
    } catch (error) {
      toast.add({ title: t('configuration.error.notSaved'), description: error instanceof Error ? error.message : undefined, type: 'error' });
      return null;
    } finally {
      setSaving(false);
    }
  }, [client, saving, state, t]);
  const reset = useCallback(() => {
    if (state.status !== 'ready') return;
    session.current = { file: state.file, content: state.file.content, dirty: false };
    setState({ ...state, content: state.file.content });
  }, [session, state]);
  useUnsavedChanges(dirty, reset, saving);

  return { state, draft: parsed.draft, editorError: parsed.error, dirty, saving, save, reset, update, updateText };
}
