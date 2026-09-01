import type { TFunction } from 'i18next';

import { useTranslation } from 'react-i18next';
import { useCallback, useEffect, useRef, useState } from 'react';
import {
  isLosslessNumber,
  parse as parseLosslessJSON,
  stringify as stringifyLosslessJSON,
} from 'lossless-json';

import type { CanonicalRevisionPage, CanonicalSnapshot } from '@/api/api-client';

import { useApiClient } from '@/api/api-client-context';

export interface CanonicalDraft {
  [key: string]: unknown;
}

type LoadState
  = | { status: 'loading'; snapshot: null; draft: null; error: null }
    | { status: 'error'; snapshot: null; draft: null; error: unknown }
    | { status: 'ready'; snapshot: CanonicalSnapshot; draft: CanonicalDraft; error: null };

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

function parseDraft(snapshot: CanonicalSnapshot): CanonicalDraft {
  return parseCanonicalDraft(snapshot.document_json);
}

export function useCanonicalConfiguration() {
  const client = useApiClient();
  const { i18n, t } = useTranslation();
  const [state, setState] = useState<LoadState>({ status: 'loading', snapshot: null, draft: null, error: null });
  const [revisions, setRevisions] = useState<CanonicalRevisionPage | null>(null);
  const [revisionError, setRevisionError] = useState<unknown>(null);
  const [loadingOlderRevisions, setLoadingOlderRevisions] = useState(false);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<unknown>(null);
  const [message, setMessage] = useState('');
  const loadingOlderRevisionsRef = useRef(false);

  const load = useCallback(async (signal?: AbortSignal) => {
    try {
      const [snapshot, revisionPage] = await Promise.all([
        client.getCanonical(signal),
        client.listRevisions({ limit: 8 }, signal),
      ]);
      if (signal?.aborted) return;
      setState({ status: 'ready', snapshot, draft: parseDraft(snapshot), error: null });
      setRevisions(revisionPage);
      setRevisionError(null);
    } catch (error) {
      if (signal?.aborted || (error instanceof DOMException && error.name === 'AbortError')) return;
      setState({
        status: 'error', snapshot: null, draft: null,
        error: localizeCanonicalDraftError(error, i18n.t),
      });
    }
  }, [client, i18n]);

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, [load]);

  const applyUpdate = useCallback((change: (draft: CanonicalDraft) => CanonicalDraft) => {
    if (saving) return;
    setState((current) => current.status === 'ready'
      ? { ...current, draft: change(current.draft) }
      : current);
    setMessage('');
    setSaveError(null);
  }, [saving]);

  const save = useCallback(async () => {
    if (state.status !== 'ready') return;
    setSaving(true);
    setSaveError(null);
    setMessage('');
    try {
      const result = await client.replaceCanonical(
        encodeCanonicalDraft(state.draft),
        state.snapshot.id,
      );
      const next = result.revision;
      setState({ status: 'ready', snapshot: next, draft: parseDraft(next), error: null });
      setMessage(result.no_change
        ? t('configuration.message.noChange')
        : t('configuration.message.saved', {
            sequence: new Intl.NumberFormat(i18n.language).format(next.sequence),
          }));
      try {
        setRevisions(await client.listRevisions({ limit: 8 }));
        setRevisionError(null);
      } catch (error) {
        setRevisionError(error);
      }
    } catch (error) {
      setSaveError(localizeCanonicalDraftError(error, t));
    } finally {
      setSaving(false);
    }
  }, [client, i18n.language, state, t]);

  const reset = useCallback(() => {
    setState((current) => current.status === 'ready'
      ? { ...current, draft: parseDraft(current.snapshot) }
      : current);
    setMessage('');
    setSaveError(null);
  }, []);

  const restore = useCallback(async (reference: string, baseRevisionID: string) => {
    if (state.status !== 'ready') return;
    setSaving(true);
    setSaveError(null);
    setMessage('');
    try {
      const result = await client.restoreRevision(reference, baseRevisionID);
      const next = result.revision;
      setState({ status: 'ready', snapshot: next, draft: parseDraft(next), error: null });
      setMessage(t('configuration.message.restored', {
        reference,
        sequence: new Intl.NumberFormat(i18n.language).format(next.sequence),
      }));
      setRevisions(await client.listRevisions({ limit: 8 }));
      setRevisionError(null);
    } catch (error) {
      setSaveError(localizeCanonicalDraftError(error, t));
    } finally {
      setSaving(false);
    }
  }, [client, i18n.language, state, t]);

  const loadOlderRevisions = useCallback(async () => {
    const beforeSequence = revisions?.next_before_sequence;
    if (beforeSequence === undefined || loadingOlderRevisionsRef.current) return;
    loadingOlderRevisionsRef.current = true;
    setLoadingOlderRevisions(true);
    try {
      const page = await client.listRevisions({ beforeSequence, limit: 8 });
      setRevisions((current) => current === null
        ? page
        : {
            items: [...current.items, ...page.items],
            next_before_sequence: page.next_before_sequence,
          });
      setRevisionError(null);
    } catch (error) {
      setRevisionError(error);
    } finally {
      loadingOlderRevisionsRef.current = false;
      setLoadingOlderRevisions(false);
    }
  }, [client, revisions?.next_before_sequence]);

  return {
    state,
    revisions,
    revisionError,
    loadingOlderRevisions,
    saving,
    saveError,
    message,
    update: applyUpdate,
    save,
    reset,
    restore,
    loadOlderRevisions,
  };
}
