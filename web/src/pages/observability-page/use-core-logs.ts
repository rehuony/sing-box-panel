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
  const offsetRef = useRef({ file: '', value: -1 });
  const file = paused ? pausedFile : selection || files[0]?.name || '';
  const current = file !== '' && file === files[0]?.name;

  useEffect(() => {
    const abort = new AbortController();
    async function refresh() {
      try {
        const result = await client.listCoreLogFiles(abort.signal);
        if (!abort.signal.aborted) {
          setFiles(result.items);
          setListError(null);
        }
      } catch (reason) {
        if (!abort.signal.aborted) setListError(reason);
      } finally {
        if (!abort.signal.aborted) setLoading(false);
      }
    }
    void refresh();
    const timer = setInterval(() => {
      void refresh();
    }, 10_000);
    return () => {
      abort.abort();
      clearInterval(timer);
    };
  }, [client]);

  useEffect(() => {
    if (!file || paused) return;
    const abort = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    async function connect() {
      setConnected(false);
      if (offsetRef.current.file !== file) {
        offsetRef.current = { file, value: -1 };
        setText('');
      }
      try {
        if (!current) {
          const chunk = await client.readCoreLog(file, -1, abort.signal);
          if (!abort.signal.aborted) {
            setText(chunk.text);
            setError(null);
          }
          return;
        }
        for await (const chunk of client.streamCoreLog(
          file,
          offsetRef.current.value,
          abort.signal,
        )) {
          if (abort.signal.aborted) return;
          offsetRef.current.value = chunk.next_offset;
          setText((previous) => appendCoreText(previous, chunk.text));
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
  }, [client, file, current, paused]);

  return {
    files,
    file,
    current,
    text,
    connected: connected && !paused,
    loading,
    error: error ?? listError,
    paused,
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
