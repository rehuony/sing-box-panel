import { useEffect, useRef, useState } from 'react';

import type { CoreLogFile } from '@/api/api-client';

import { useApiClient } from '@/api/api-client-context';

import { appendCoreText } from './core-log-lines';

export function useCoreLogs() {
  const client = useApiClient();
  const [files, setFiles] = useState<CoreLogFile[]>([]);
  const [selection, setSelection] = useState('');
  const [paused, setPaused] = useState(false);
  const [pausedFile, setPausedFile] = useState('');
  const [text, setText] = useState('');
  const [connected, setConnected] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [listError, setListError] = useState<unknown>(null);
  const [loading, setLoading] = useState(true);
  const [filesVersion, setFilesVersion] = useState(0);
  const [clearing, setClearing] = useState(false);
  const [readVersion, setReadVersion] = useState(0);
  const readerRef = useRef<AbortController | null>(null);
  const listVersionRef = useRef(0);
  const offsetRef = useRef({ file: '', value: -1, generation: '' });
  const file = paused ? pausedFile : selection || files[0]?.name || '';
  const current = file !== '' && file === files[0]?.name;

  useEffect(() => {
    const abort = new AbortController();
    let refreshing = false;
    async function refresh() {
      // A slow poll must not overlap the next one and overwrite a newer list.
      if (refreshing) return;
      refreshing = true;
      const version = listVersionRef.current;
      try {
        const result = await client.listCoreLogFiles(abort.signal);
        if (!abort.signal.aborted && version === listVersionRef.current) {
          setFiles(result.items);
          setListError(null);
        }
      } catch (reason) {
        if (!abort.signal.aborted && version === listVersionRef.current) setListError(reason);
      } finally {
        refreshing = false;
        if (!abort.signal.aborted && version === listVersionRef.current) setLoading(false);
      }
    }
    function refreshWhenVisible() {
      if (document.visibilityState === 'visible') void refresh();
    }
    void refresh();
    document.addEventListener('visibilitychange', refreshWhenVisible);
    const timer = setInterval(() => {
      void refresh();
    }, 10_000);
    return () => {
      abort.abort();
      clearInterval(timer);
      document.removeEventListener('visibilitychange', refreshWhenVisible);
    };
  }, [client, filesVersion]);

  useEffect(() => {
    if (paused || clearing) return;
    const abort = new AbortController();
    readerRef.current = abort;
    let timer: ReturnType<typeof setTimeout>;
    async function connect() {
      if (abort.signal.aborted) return;
      setConnected(false);
      if (offsetRef.current.file !== file) {
        offsetRef.current = { file, value: -1, generation: '' };
        setText('');
        setError(null);
      }
      if (!file) return;
      try {
        if (!current) {
          const chunk = await client.readCoreLog(
            file, offsetRef.current.value, offsetRef.current.generation, abort.signal,
          );
          if (!abort.signal.aborted) {
            offsetRef.current = { file, value: chunk.next_offset, generation: chunk.generation };
            setText((previous) => appendCoreText(chunk.reset ? '' : previous, chunk.text));
            setError(null);
          }
          return;
        }
        for await (const chunk of client.streamCoreLog(
          file,
          offsetRef.current.value,
          offsetRef.current.generation,
          abort.signal,
        )) {
          if (abort.signal.aborted) return;
          offsetRef.current = { file, value: chunk.next_offset, generation: chunk.generation };
          setText((previous) => appendCoreText(chunk.reset ? '' : previous, chunk.text));
          setConnected(true);
          setError(null);
        }
      } catch (reason) {
        if (!abort.signal.aborted) setError(reason);
      } finally {
        if (!abort.signal.aborted && current) {
          setConnected(false);
          timer = setTimeout(() => {
            void connect();
          }, 2000);
        }
      }
    }
    void connect();
    return () => {
      abort.abort();
      clearTimeout(timer);
    };
  }, [client, file, current, paused, clearing, readVersion]);

  return {
    files,
    file,
    current,
    text,
    connected: connected && !paused && !clearing,
    loading,
    error: error ?? listError,
    paused,
    deletable: files.find((item) => item.name === file)?.deletable === true,
    clearing,
    clear: async (name: string) => {
      // Abort synchronously: already-buffered chunks must not replay after clear.
      readerRef.current?.abort();
      setClearing(true);
      listVersionRef.current++;
      try {
        await client.clearCoreLog(name);
        if (offsetRef.current.file === name) {
          offsetRef.current = { file: name, value: 0, generation: '' };
          setText('');
          setError(null);
        }
        listVersionRef.current++;
        setFiles((previous) => previous.map((item) => item.name === name ? { ...item, size: 0 } : item));
        setFilesVersion((version) => version + 1);
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
      listVersionRef.current++;
      setFilesVersion((version) => version + 1);
      setFiles((previous) => previous.filter((item) => item.name !== name));
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
