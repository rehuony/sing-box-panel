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
import { orderConfigurationValue } from '@/utils/configuration-order';
import {
  useConfigurationSessionStore,
  useConfigurationSessionStoreApi,
} from '@/stores/configuration-session.store';

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

export function encodeCanonicalValue(value: unknown, indentation?: number, root = false): string {
  const encoded = stringifyLosslessJSON(orderConfigurationValue(value, root), null, indentation);
  if (encoded === undefined) throw new CanonicalDraftError('notEncoded');
  return encoded;
}

export function encodeCanonicalDraft(draft: CanonicalDraft, indentation?: number): string {
  return encodeCanonicalValue(draft, indentation, true);
}

type FileState
  = | { status: 'loading'; file: null; content: string; error: null }
    | { status: 'error'; file: null; content: string; error: unknown }
    | { status: 'ready'; file: ConfigurationFile; content: string; error: null };

type LoadState
  = | { status: 'loading'; error: null }
    | { status: 'error'; error: unknown }
    | { status: 'ready'; error: null };

export function useCanonicalConfiguration() {
  const client = useApiClient();
  const { t } = useTranslation();
  const store = useConfigurationSessionStoreApi();
  const draftSession = useConfigurationSessionStore(state => state.draft);
  const replaceDraft = useConfigurationSessionStore(state => state.replaceDraft);
  const resetDraft = useConfigurationSessionStore(state => state.resetDraft);
  const updateDraftContent = useConfigurationSessionStore(state => state.updateDraftContent);
  const [restored] = useState(() => {
    const current = store.getState().draft;
    return current !== null && current.content !== current.file.content;
  });
  const [loadState, setLoadState] = useState<LoadState>(() => restored
    ? { status: 'ready', error: null }
    : { status: 'loading', error: null });
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (restored) return;
    const controller = new AbortController();
    void client.getConfigurationFile(controller.signal).then(file => {
      if (!controller.signal.aborted) {
        replaceDraft({ file, content: file.content });
        setLoadState({ status: 'ready', error: null });
      }
    }).catch((error: unknown) => {
      if (!controller.signal.aborted) setLoadState({ status: 'error', error });
    });
    return () => controller.abort();
  }, [client, replaceDraft, restored]);

  const state = useMemo<FileState>(() => {
    if (loadState.status === 'error') return { status: 'error', file: null, content: '', error: loadState.error };
    if (loadState.status === 'loading' || draftSession === null) return { status: 'loading', file: null, content: '', error: null };
    return { status: 'ready', file: draftSession.file, content: draftSession.content, error: null };
  }, [draftSession, loadState]);

  const parsed = useMemo(() => {
    if (state.status !== 'ready') return { draft: null, error: null };
    try {
      return { draft: parseCanonicalDraft(state.content), error: null };
    } catch (error) {
      return { draft: null, error: localizeCanonicalDraftError(error, t) };
    }
  }, [state.content, state.status, t]);
  const dirty = state.status === 'ready' && state.content !== state.file.content;

  const updateText = useCallback((content: string) => {
    if (!saving) updateDraftContent(content);
  }, [saving, updateDraftContent]);
  const update = useCallback((change: (draft: CanonicalDraft) => CanonicalDraft) => {
    if (saving) return;
    updateDraftContent((content) => {
      try {
        return encodeCanonicalDraft(change(parseCanonicalDraft(content)), 2);
      } catch {
        return content;
      }
    });
  }, [saving, updateDraftContent]);

  const save = useCallback(async () => {
    const current = store.getState().draft;
    if (current === null || saving) return null;
    setSaving(true);
    try {
      const file = await client.saveConfigurationFile({ revision: current.file.revision, content: current.content });
      replaceDraft({ file, content: file.content });
      toast.add({ title: t('configuration.file.saved'), type: 'success' });
      return file;
    } catch (error) {
      toast.add({ title: t('configuration.error.notSaved'), description: error instanceof Error ? error.message : undefined, type: 'error' });
      return null;
    } finally {
      setSaving(false);
    }
  }, [client, replaceDraft, saving, store, t]);
  const reset = useCallback(() => {
    resetDraft();
  }, [resetDraft]);
  useUnsavedChanges(dirty, reset, saving, { allowSamePathNavigation: true });

  return { state, draft: parsed.draft, editorError: parsed.error, dirty, saving, save, reset, update, updateText };
}
