import { useEffect, useRef, useState } from 'react';

import type { FilesystemEntry, FilesystemMode, FilesystemPage, FilesystemQuery } from '@/api/api-client';

import { useApiClient } from '@/api/api-client-context';

export function validOutputName(value: string): boolean {
  return value !== '' && value !== '.' && value !== '..' && !value.includes('/') && !value.includes('\0');
}

export function useServerPathPicker(
  initialValue: string, mode: FilesystemMode, open: boolean, onSelect: (path: string) => void,
) {
  const api = useApiClient();
  const [query, setQuery] = useState<FilesystemQuery>({ path: initialValue || undefined, limit: 10, offset: 0 });
  const [result, setResult] = useState<{ query: FilesystemQuery; data?: FilesystemPage; error?: unknown }>();
  const [search, setSearch] = useState('');
  const [filename, setFilename] = useState(initialValue.split('/').at(-1) ?? '');
  const [selection, setSelection] = useState<{ query: FilesystemQuery; entry: FilesystemEntry }>();
  const [selectionError, setSelectionError] = useState<unknown>();
  const [checking, setChecking] = useState(false);
  const checkControllerRef = useRef<AbortController | null>(null);
  const initializedRef = useRef(false);
  const filenameEditedRef = useRef(false);

  useEffect(() => {
    if (!open) return;
    const controller = new AbortController();
    api.listFilesystemEntries(query, controller.signal).then(data => {
      if (controller.signal.aborted) return;
      if (!initializedRef.current) {
        initializedRef.current = true;
        if (!filenameEditedRef.current && data.path === data.requested_path) setFilename('');
      }
      setSelectionError(undefined);
      setResult({ query, data });
    }).catch((error: unknown) => {
      if (!controller.signal.aborted) setResult({ query, error });
    });
    return () => controller.abort();
  }, [api, open, query]);

  useEffect(() => () => checkControllerRef.current?.abort(), [open, mode]);

  useEffect(() => {
    if (!open || checking) return;
    const timeout = window.setTimeout(() => {
      setQuery(current => current.search === (search || undefined)
        ? current
        : {
            ...current,
            path: result?.query === current ? result.data?.path ?? current.path : current.path,
            search: search || undefined, offset: 0,
          });
    }, 200);
    return () => window.clearTimeout(timeout);
  }, [open, search, result, checking]);

  const pending = result?.query !== query;
  const data = result?.data;
  const outputPath = mode === 'output-file' && data && validOutputName(filename)
    ? `${data.path.replace(/\/+$/, '')}/${filename}`
    : '';
  // A changed directory, filter, page or refresh invalidates the visible selection.
  const selectedEntry = selection?.query === query ? selection.entry : undefined;
  const selectedPath = selectedEntry
    ? selectedEntry.kind === (mode === 'output-file' ? 'file' : mode) ? selectedEntry.path : ''
    : mode === 'directory' ? data?.path ?? '' : outputPath;

  function select(entry: FilesystemEntry) {
    setSelection({ query, entry });
    setSelectionError(undefined);
    if (mode === 'output-file' && entry.kind === 'file') {
      filenameEditedRef.current = true;
      setFilename(entry.name);
    }
  }

  function editFilename(value: string) {
    filenameEditedRef.current = true;
    setFilename(value);
    setSelection(undefined);
    setSelectionError(undefined);
  }

  function navigate(path: string) {
    setSelectionError(undefined);
    setSearch('');
    setQuery(current => ({ ...current, path, offset: 0, search: undefined }));
  }

  async function confirm(path: string) {
    if (!path || pending || checking || !open || result?.error
      || (checkControllerRef.current && !checkControllerRef.current.signal.aborted)) {
      return;
    }
    const controller = new AbortController();
    checkControllerRef.current = controller;
    setChecking(true);
    setSelectionError(undefined);
    try {
      const resolved = await api.resolveFilesystemPath({ path, mode }, controller.signal);
      if (!controller.signal.aborted) onSelect(resolved.path);
    } catch (error) {
      if (!controller.signal.aborted) setSelectionError(error);
    } finally {
      if (checkControllerRef.current === controller) checkControllerRef.current = null;
      if (!controller.signal.aborted) setChecking(false);
    }
  }

  return {
    query, setQuery, data, pending, search, setSearch,
    filename, editFilename, checking, selectedEntry, selectedPath, select,
    error: result?.error ?? selectionError, navigate, confirm,
  };
}
