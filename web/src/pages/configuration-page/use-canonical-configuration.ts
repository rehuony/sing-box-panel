import { useCallback, useEffect, useState } from 'react';
import {
  parse as parseLosslessJSON,
  stringify as stringifyLosslessJSON,
} from 'lossless-json';

import type { CanonicalRevisionPage, CanonicalSnapshot, JsonObject } from '@/api/api-client';

import { useApiClient } from '@/api/api-client-context';

export interface CanonicalDraft {
  schema_version: 2;
  configuration: JsonObject;
}

type LoadState
  = | { status: 'loading'; snapshot: null; draft: null; error: null }
    | { status: 'error'; snapshot: null; draft: null; error: unknown }
    | { status: 'ready'; snapshot: CanonicalSnapshot; draft: CanonicalDraft; error: null };

function parseDraft(snapshot: CanonicalSnapshot): CanonicalDraft {
  const parsed = parseLosslessJSON(snapshot.document_json);
  if (parsed === null || Array.isArray(parsed) || typeof parsed !== 'object') {
    throw new Error('The canonical revision is not an object.');
  }
  const envelope = parsed as Record<string, unknown>;
  if (envelope.schema_version !== 2 || envelope.configuration === null
    || Array.isArray(envelope.configuration) || typeof envelope.configuration !== 'object') {
    throw new Error('The canonical revision is not a schema-v2 configuration envelope.');
  }
  return envelope as unknown as CanonicalDraft;
}

function encodeDraft(draft: CanonicalDraft): string {
  const encoded = stringifyLosslessJSON(draft);
  if (encoded === undefined) throw new Error('The structured configuration cannot be encoded.');
  return encoded;
}

export function useCanonicalConfiguration() {
  const client = useApiClient();
  const [state, setState] = useState<LoadState>({ status: 'loading', snapshot: null, draft: null, error: null });
  const [revisions, setRevisions] = useState<CanonicalRevisionPage | null>(null);
  const [revisionError, setRevisionError] = useState<unknown>(null);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<unknown>(null);
  const [message, setMessage] = useState('');

  const load = useCallback(async (signal?: AbortSignal) => {
    try {
      const [snapshot, revisionPage] = await Promise.all([
        client.getCanonical(signal),
        client.listRevisions(signal),
      ]);
      if (signal?.aborted) return;
      setState({ status: 'ready', snapshot, draft: parseDraft(snapshot), error: null });
      setRevisions(revisionPage);
      setRevisionError(null);
    } catch (error) {
      if (signal?.aborted || (error instanceof DOMException && error.name === 'AbortError')) return;
      setState({ status: 'error', snapshot: null, draft: null, error });
    }
  }, [client]);

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, [load]);

  const update = useCallback((change: (draft: CanonicalDraft) => CanonicalDraft) => {
    setState((current) => current.status === 'ready'
      ? { ...current, draft: change(current.draft) }
      : current);
    setMessage('');
    setSaveError(null);
  }, []);

  const save = useCallback(async () => {
    if (state.status !== 'ready') return;
    setSaving(true);
    setSaveError(null);
    setMessage('');
    try {
      const result = await client.replaceCanonical(encodeDraft(state.draft), state.snapshot.id);
      const next = result.revision;
      setState({ status: 'ready', snapshot: next, draft: parseDraft(next), error: null });
      setMessage(result.no_change ? 'Configuration already matched the saved revision.' : `Saved global revision #${next.sequence}.`);
      try {
        setRevisions(await client.listRevisions());
        setRevisionError(null);
      } catch (error) {
        setRevisionError(error);
      }
    } catch (error) {
      setSaveError(error);
    } finally {
      setSaving(false);
    }
  }, [client, state]);

  const reset = useCallback(() => {
    setState((current) => current.status === 'ready'
      ? { ...current, draft: parseDraft(current.snapshot) }
      : current);
    setMessage('');
    setSaveError(null);
  }, []);

  const restore = useCallback(async (reference: string) => {
    if (state.status !== 'ready') return;
    setSaving(true);
    setSaveError(null);
    setMessage('');
    try {
      const result = await client.restoreRevision(reference, state.snapshot.id);
      const next = result.revision;
      setState({ status: 'ready', snapshot: next, draft: parseDraft(next), error: null });
      setMessage(`Restored revision ${reference} as new global revision #${next.sequence}.`);
      setRevisions(await client.listRevisions());
      setRevisionError(null);
    } catch (error) {
      setSaveError(error);
    } finally {
      setSaving(false);
    }
  }, [client, state]);

  return {
    state,
    revisions,
    revisionError,
    saving,
    saveError,
    message,
    load,
    update,
    save,
    reset,
    restore,
  };
}
