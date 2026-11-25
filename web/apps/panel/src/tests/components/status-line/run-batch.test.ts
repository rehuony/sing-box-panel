import { logLine } from '@shared/components/terminal';
import { toast } from '@shared/components/ui/toast-manager';
import { afterEach, describe, expect, it, vi } from 'vitest';

import type { ApiClient } from '@/api/api-client';
import type { NodeStatus } from '@/utils/fleet-summary';

import i18n from '@/i18n';
import { deferred } from '@/tests/deferred';
import { createQueryClient } from '@/api/query-client';
import { runBatch } from '@/components/status-line/run-batch';
import { useCommandSession } from '@/stores/command-session.store';
import { createMockApiClient, testMetrics, testNode } from '@/tests/api/mock-api-client';

const t = i18n.getFixedT('en');
const cache = createQueryClient();

function fleet(count: number): NodeStatus[] {
  return Array.from({ length: count }, (_, index) => ({ metrics: testMetrics, node: { ...testNode, id: `node_${index}`, name: `node-${index}` } }));
}

function texts() {
  return useCommandSession.getState().entries.map(entry => entry.text);
}

afterEach(() => {
  useCommandSession.getState().reset();
  vi.restoreAllMocks();
});

describe('runBatch', () => {
  it('refuses a second batch while one runs', async () => {
    const release = deferred<void>();
    const client = createMockApiClient({
      runFleetCommand: vi.fn<ApiClient['runFleetCommand']>(async function* () {
        await release.promise;
        yield { done: { failed: 0, skipped: 0, succeeded: 1, total: 1 } };
      }),
    });
    const first = runBatch({ cache, action: 'restart', client, nodes: fleet(1), t });
    await runBatch({ cache, action: 'stop', client, nodes: fleet(1), t });
    expect(client.runFleetCommand).toHaveBeenCalledTimes(1);
    expect(texts()).toContain('a batch is still running; wait for it to finish');
    expect(useCommandSession.getState().running).toBe(true);
    release.resolve();
    await first;
    expect(useCommandSession.getState().running).toBe(false);
  });

  it('keeps every node\'s line of a batch larger than the terminal keeps', async () => {
    const client = createMockApiClient();
    await runBatch({ cache, action: 'restart', client, nodes: fleet(300), t });
    const lines = texts();
    expect(lines[0]).toBe('Restart on 300 nodes…');
    expect(lines.filter(text => text === 'done')).toHaveLength(300);
    expect(lines.at(-1)).toBe('300 succeeded · 0 failed · 0 skipped');
  });

  it('leaves a later command\'s result alone and reports in a toast when its own was replaced', async () => {
    const add = vi.spyOn(toast, 'add');
    const release = deferred<void>();
    const client = createMockApiClient({
      runFleetCommand: vi.fn<ApiClient['runFleetCommand']>(async function* () {
        await release.promise;
        yield { result: { node_id: 'node_0', ok: true } };
        yield { done: { failed: 0, skipped: 0, succeeded: 1, total: 1 } };
      }),
    });
    useCommandSession.getState().setOpen(true);
    const batch = runBatch({ cache, action: 'restart', client, nodes: fleet(1), t });
    const session = useCommandSession.getState();
    session.begin(logLine('muted', 'later'));
    release.resolve();
    await batch;
    expect(texts()).toEqual(['later']);
    expect(add).toHaveBeenCalledWith(expect.objectContaining({ description: '1 succeeded · 0 failed · 0 skipped', type: 'success' }));
  });

  it('says so when a batch stops before its summary', async () => {
    const add = vi.spyOn(toast, 'add');
    const client = createMockApiClient({
      runFleetCommand: vi.fn<ApiClient['runFleetCommand']>(async function* () {
        yield { result: { node_id: 'node_0', ok: true } };
      }),
    });
    await runBatch({ cache, action: 'restart', client, nodes: fleet(2), t });
    expect(texts().at(-1)).toBe('the batch stopped reporting before its summary; check each node\'s state');
    expect(add).toHaveBeenCalledWith(expect.objectContaining({ title: 'Restart did not report its outcome', type: 'error' }));
  });

  it('prints nothing into the session after the one it started in', async () => {
    const release = deferred<void>();
    const client = createMockApiClient({
      runFleetCommand: vi.fn<ApiClient['runFleetCommand']>(async function* () {
        await release.promise;
        yield { result: { node_id: 'node_0', ok: true } };
        yield { done: { failed: 0, skipped: 0, succeeded: 1, total: 1 } };
      }),
    });
    const batch = runBatch({ cache, action: 'restart', client, nodes: fleet(1), t });
    useCommandSession.getState().reset();
    release.resolve();
    await batch;
    expect(useCommandSession.getState().entries).toEqual([]);
    expect(useCommandSession.getState().running).toBe(false);
  });
});
