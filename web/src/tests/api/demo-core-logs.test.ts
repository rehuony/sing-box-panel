import { describe, expect, it, vi } from 'vitest';

import { createDemoCoreLogs } from '@/api/demo/demo-core-logs';

describe('demo core log clearing', () => {
  it('resets a live reader and stale resume after another reader clears the capture', async () => {
    vi.useFakeTimers();
    const abort = new AbortController();
    try {
      const client = createDemoCoreLogs();
      const { items: [file] } = await client.listCoreLogFiles();
      const stream = client.streamCoreLog(file.name, -1, undefined, abort.signal);
      const first = (await stream.next()).value;
      expect(first).toBeDefined();
      if (!first) return;
      const pending = stream.next();
      await client.clearCoreLog(file.name);
      await vi.advanceTimersByTimeAsync(2500);
      const next = (await pending).value;
      expect(next).toMatchObject({ reset: true });
      expect(next?.text).toContain('outbound connection');
      expect(next?.text).not.toContain('sing-box started');
      expect(await client.readCoreLog(file.name, first.next_offset, first.generation)).toEqual(next);
      abort.abort();
      await stream.return();
    } finally {
      abort.abort();
      vi.useRealTimers();
    }
  });

  it('clears only the selected saved capture and keeps it available for rereads', async () => {
    const client = createDemoCoreLogs();
    const { items } = await client.listCoreLogFiles();
    const [current, archive] = items;
    const old = await client.readCoreLog(archive.name, -1);
    await client.clearCoreLog(current.name);
    expect(await client.readCoreLog(current.name, -1)).toMatchObject({ text: '', next_offset: 0, size: 0 });
    expect(await client.readCoreLog(archive.name, -1)).toEqual(old);
    const stream = client.streamCoreLog(current.name, 0, undefined);
    expect((await stream.next()).value).toMatchObject({ text: '', next_offset: 0 });
    await stream.return();
    await client.clearCoreLog(archive.name);
    expect(await client.readCoreLog(archive.name, -1)).toMatchObject({ text: '', next_offset: 0 });
    expect((await client.listCoreLogFiles()).items).toHaveLength(2);
    await expect(client.clearCoreLog('missing.log')).rejects.toThrow('Log file not found');
  });
});
