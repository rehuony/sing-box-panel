import type { PropsWithChildren } from 'react';

import { describe, expect, it, vi } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';

import type { ApiClient, CoreLogChunk, CoreLogFile } from '@/api/api-client';

import { ApiClientProvider } from '@/api/api-client-context';
import { createMockApiClient } from '@/tests/api/mock-api-client';
import { useCoreLogs } from '@/pages/observability-page/use-core-logs';

const current = '2026-09-23-000.log';
const archive = '2026-09-22-000.log';
const files = [current, archive].map((name) => ({ name, size: 32, updated_at: '2026-09-23T00:00:00Z', deletable: name === archive }));

function show(client: ApiClient) {
  return renderHook(useCoreLogs, {
    wrapper: ({ children }: PropsWithChildren) => <ApiClientProvider client={client}>{children}</ApiClientProvider>,
  });
}

describe('core log file refresh', () => {
  it('does not overlap slow polls and refreshes immediately when the page becomes visible', async () => {
    vi.useFakeTimers();
    try {
      let finishList = (_page: { items: CoreLogFile[] }) => {};
      const next = { ...files[0], name: '2026-09-24-000.log', size: 0 };
      const client = createMockApiClient({
        listCoreLogFiles: vi.fn()
          .mockImplementationOnce(() => new Promise<{ items: CoreLogFile[] }>((resolve) => {
            finishList = resolve;
          }))
          .mockResolvedValue({ items: [next, ...files] }),
      });
      const { result, unmount } = show(client);
      await act(() => vi.advanceTimersByTimeAsync(20_000));
      expect(client.listCoreLogFiles).toHaveBeenCalledTimes(1);
      await act(async () => finishList({ items: files }));
      expect(result.current.file).toBe(current);
      await act(async () => document.dispatchEvent(new Event('visibilitychange')));
      expect(result.current.file).toBe(next.name);
      expect(client.listCoreLogFiles).toHaveBeenCalledTimes(2);
      unmount();
      document.dispatchEvent(new Event('visibilitychange'));
      expect(client.listCoreLogFiles).toHaveBeenCalledTimes(2);
    } finally {
      vi.useRealTimers();
    }
  });
});

describe('core log clear requests', () => {
  it('replaces buffered output when an active stream observes another reader clearing the file', async () => {
    let release = () => {};
    const client = createMockApiClient({
      listCoreLogFiles: vi.fn(async () => ({ items: files })),
      streamCoreLog: vi.fn(async function* (file, _offset, _generation, signal) {
        yield { file, text: 'INFO old\n', next_offset: 9, size: 9, generation: 'old' };
        await new Promise<void>((resolve) => {
          release = resolve;
        });
        yield { file, text: 'INFO new after external clear\n', next_offset: 30, size: 30, generation: 'new', reset: true };
        if (!signal?.aborted) await new Promise<void>((resolve) => signal?.addEventListener('abort', () => resolve(), { once: true }));
      }),
    });
    const { result } = show(client);
    await waitFor(() => expect(result.current.text).toBe('INFO old\n'));
    await act(async () => release());
    expect(result.current.text).toBe('INFO new after external clear\n');
    expect(client.clearCoreLog).not.toHaveBeenCalled();
  });

  it.each(['resume', 'reconnect'])('sends the generation on %s and replaces output after an external clear', async (mode) => {
    vi.useFakeTimers();
    try {
      const client = createMockApiClient({
        listCoreLogFiles: vi.fn(async () => ({ items: files })),
        streamCoreLog: vi.fn(async function* (file, offset, generation, signal) {
          if (offset === -1) {
            yield { file, text: 'INFO old\n', next_offset: 9, size: 9, generation: 'old' };
            if (mode === 'reconnect') return;
          } else {
            expect(generation).toBe('old');
            yield { file, text: 'INFO new after external clear\n', next_offset: 30, size: 30, generation: 'new', reset: true };
          }
          if (!signal?.aborted) await new Promise<void>((resolve) => signal?.addEventListener('abort', () => resolve(), { once: true }));
        }),
      });
      const { result, unmount } = show(client);
      await act(async () => {});
      expect(result.current.text).toBe('INFO old\n');
      if (mode === 'resume') {
        act(() => result.current.setPaused(true));
        await act(async () => result.current.setPaused(false));
      } else {
        await act(() => vi.advanceTimersByTimeAsync(2000));
      }
      expect(client.streamCoreLog).toHaveBeenLastCalledWith(current, 9, 'old', expect.any(AbortSignal));
      expect(result.current.text).toBe('INFO new after external clear\n');
      unmount();
    } finally {
      vi.useRealTimers();
    }
  });

  it('ignores an outstanding historical read and keeps saved output cleared on reselection', async () => {
    let saved = 'INFO historical output\n';
    let finishRead = (_chunk: CoreLogChunk) => {};
    const client = createMockApiClient({
      listCoreLogFiles: vi.fn(async () => ({ items: files })),
      readCoreLog: vi.fn()
        .mockImplementationOnce(() => new Promise<CoreLogChunk>((resolve) => {
          finishRead = resolve;
        }))
        .mockImplementation(async (name) => ({
          file: name, text: saved, size: saved.length, generation: 'test-generation', next_offset: saved.length,
        })),
      clearCoreLog: vi.fn(async () => {
        saved = '';
      }),
    });
    const { result } = show(client);
    await waitFor(() => expect(result.current.file).toBe(current));
    act(() => result.current.selectFile(archive));
    await waitFor(() => expect(client.readCoreLog).toHaveBeenCalledTimes(1));
    await act(() => result.current.clear(archive));
    expect(vi.mocked(client.readCoreLog).mock.calls[0][3]?.aborted).toBe(true);
    expect(client.readCoreLog).toHaveBeenLastCalledWith(archive, 0, '', expect.any(AbortSignal));
    await act(async () => finishRead({ file: archive, text: 'INFO stale history\n', size: 32, generation: 'test-generation', next_offset: 32 }));
    expect(result.current.text).toBe('');
    act(() => result.current.selectFile(current));
    await waitFor(() => expect(result.current.file).toBe(current));
    act(() => result.current.selectFile(archive));
    await waitFor(() => expect(client.readCoreLog).toHaveBeenLastCalledWith(archive, -1, '', expect.any(AbortSignal)));
    expect(result.current.text).toBe('');
    expect(client.clearCoreLog).toHaveBeenCalledExactlyOnceWith(archive);
  });

  it('ignores list results started before clear and refreshes the selected file size', async () => {
    vi.useFakeTimers();
    try {
      let finishList = (_page: { items: CoreLogFile[] }) => {};
      const client = createMockApiClient({
        listCoreLogFiles: vi.fn()
          .mockResolvedValueOnce({ items: files })
          .mockImplementationOnce(() => new Promise<{ items: CoreLogFile[] }>((resolve) => {
            finishList = resolve;
          }))
          .mockResolvedValue({ items: files.map((file) => ({ ...file, size: file.name === current ? 0 : 32 })) }),
      });
      const { result, unmount } = show(client);
      await act(async () => {});
      await act(() => vi.advanceTimersByTimeAsync(10_000));
      expect(client.listCoreLogFiles).toHaveBeenCalledTimes(2);
      await act(() => result.current.clear(current));
      expect(result.current.files[0].size).toBe(0);
      await act(async () => finishList({ items: files }));
      expect(result.current.files.map((file) => file.size)).toEqual([0, 32]);
      unmount();
    } finally {
      vi.useRealTimers();
    }
  });
});
