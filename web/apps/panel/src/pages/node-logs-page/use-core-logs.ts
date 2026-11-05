import { useEffect, useRef, useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';

import { queries } from '@/api/queries';
import { followStream } from '@/api/stream-connection';
import { useApiClient } from '@/api/api-client-context';
import { usePageVisible } from '@/hooks/use-page-visible';

import { appendCoreText } from './core-log-lines';

export function useCoreLogs(active = true) {
  const client = useApiClient();
  const cache = useQueryClient();
  const visible = usePageVisible();
  const enabled = active && visible;
  const filesQuery = useQuery({ ...queries.coreLogFiles(client), enabled, refetchInterval: enabled ? 10_000 : false });
  const files = filesQuery.data?.items ?? [];
  const refetchFiles = filesQuery.refetch;
  useEffect(() => {
    if (enabled) void refetchFiles({ cancelRefetch: false });
  }, [enabled, refetchFiles]);
  const loading = filesQuery.isPending;
  const listError = filesQuery.error;
  const [selection, setSelection] = useState('');
  const [paused, setPaused] = useState(false);
  const [pausedFile, setPausedFile] = useState('');
  const [text, setText] = useState('');
  const [connected, setConnected] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [clearing, setClearing] = useState(false);
  const [readVersion, setReadVersion] = useState(0);
  const stopReaderRef = useRef<(() => void) | null>(null);
  const offsetRef = useRef({ file: '', value: -1, generation: '' });
  const file = paused ? pausedFile : selection || files[0]?.name || '';
  const current = file !== '' && file === files[0]?.name;

  useEffect(() => {
    if (paused || clearing || !enabled) return;
    if (offsetRef.current.file !== file) {
      offsetRef.current = { file, value: -1, generation: '' };
      setText('');
      setError(null);
    }
    if (!file) return;
    const receive = (chunk: import('@/api/api-client').CoreLogChunk) => {
      offsetRef.current = { file, value: chunk.next_offset, generation: chunk.generation };
      setText(previous => appendCoreText(chunk.reset ? '' : previous, chunk.text));
      setError(null);
      setConnected(current);
    };
    if (!current) {
      const abort = new AbortController();
      stopReaderRef.current = () => abort.abort();
      void client.readCoreLog(file, offsetRef.current.value, offsetRef.current.generation, abort.signal)
        .then(chunk => {
          if (!abort.signal.aborted) receive(chunk);
        })
        .catch(reason => {
          if (!abort.signal.aborted) setError(reason);
        });
      return () => abort.abort();
    }
    const stop = followStream(
      signal => client.streamCoreLog(file, offsetRef.current.value, offsetRef.current.generation, signal),
      receive, reason => setError(reason), () => setConnected(false),
    );
    stopReaderRef.current = stop;
    return stop;
  }, [client, file, current, paused, clearing, readVersion, enabled]);

  return {
    files,
    file,
    current,
    text,
    connected: connected && !paused && !clearing && enabled,
    loading,
    error: error ?? listError,
    paused,
    deletable: files.find((item) => item.name === file)?.deletable === true,
    clearing,
    clear: async (name: string) => {
      // Abort synchronously: already-buffered chunks must not replay after clear.
      stopReaderRef.current?.();
      setClearing(true);
      await cache.cancelQueries({ queryKey: ['coreLogFiles'] });
      try {
        await client.clearCoreLog(name);
        if (offsetRef.current.file === name) {
          offsetRef.current = { file: name, value: 0, generation: '' };
          setText('');
          setError(null);
        }
        await cache.cancelQueries({ queryKey: ['coreLogFiles'] });
        cache.setQueryData(queries.coreLogFiles(client).queryKey, previous => previous && ({
          ...previous, items: previous.items.map(item => item.name === name ? { ...item, size: 0 } : item),
        }));
        await cache.invalidateQueries({ queryKey: ['coreLogFiles'] });
      } finally {
        setClearing(false);
        // Restart even if the mutation settled before React rendered its state.
        // On failure the old text and cursor are retained.
        setReadVersion((version) => version + 1);
      }
    },
    deleteFile: async (name: string) => {
      await client.deleteCoreLogFile(name);
      // Ignore list requests started before deletion, then fetch a fresh list.
      await cache.cancelQueries({ queryKey: ['coreLogFiles'] });
      await cache.invalidateQueries({ queryKey: ['coreLogFiles'] });
      cache.setQueryData(queries.coreLogFiles(client).queryKey, previous => previous && ({
        ...previous, items: previous.items.filter(item => item.name !== name),
      }));
      setSelection('');
      setPaused(false);
    },
    setPaused: (value: boolean) => {
      if (value) setPausedFile(file);
      setPaused(value);
    },
    selectFile: (name: string) => {
      setPaused(false);
      setSelection(name === files[0]?.name ? '' : name);
    },
  };
}
