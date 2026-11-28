import { logLine } from '@shared/components/terminal';
import { toast } from '@shared/components/ui/toast-manager';
import { afterEach, describe, expect, it, vi } from 'vitest';

import type { ApiClient } from '@/api/api-client';
import type { NodeStatus } from '@/utils/fleet-summary';
import type { BatchReport } from '@/stores/command-session.store';

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
  return useCommandSession.getState().entries.flatMap(entry => entry.kind === 'line' ? [entry.text] : []);
}

function report(): BatchReport | undefined {
  return useCommandSession.getState().entries.find((entry): entry is BatchReport => entry.kind === 'batch');
}

afterEach(() => {
  useCommandSession.getState().reset();
  vi.restoreAllMocks();
  vi.useRealTimers();
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

  it('reports every node of a batch larger than the terminal keeps lines for in one report', async () => {
    const client = createMockApiClient();
    await runBatch({ cache, action: 'restart', client, nodes: fleet(300), t });
    expect(useCommandSession.getState().entries).toHaveLength(1);
    expect(report()?.outcomes.filter(outcome => outcome.ok)).toHaveLength(300);
    expect(report()?.summary).toEqual({ failed: 0, skipped: 0, succeeded: 300, total: 300 });
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

  it('follows the command again after its stream breaks and continues its results', async () => {
    vi.useFakeTimers();
    const client = createMockApiClient({
      runFleetCommand: vi.fn<ApiClient['runFleetCommand']>(async function* () {
        yield { command: { id: 'command-1' } };
        yield { result: { node_id: 'node_0', ok: true } };
        throw new TypeError('network error');
      }),
      followFleetCommand: vi.fn<ApiClient['followFleetCommand']>(async function* (commandID) {
        yield { command: { id: commandID } };
        yield { result: { node_id: 'node_1', ok: true } };
        yield { done: { failed: 0, skipped: 0, succeeded: 2, total: 2 } };
      }),
    });
    const batch = runBatch({ cache, action: 'update', client, nodes: fleet(2), t });
    await vi.advanceTimersByTimeAsync(0);
    expect(report()).toMatchObject({ lost: true, outcomes: [{ nodeId: 'node_0', ok: true }] });
    await vi.runAllTimersAsync();
    await batch;
    expect(client.followFleetCommand).toHaveBeenCalledWith('command-1', 1, expect.any(AbortSignal));
    expect(report()).toMatchObject({
      failure: null,
      lost: false,
      outcomes: [{ nodeId: 'node_0', ok: true }, { nodeId: 'node_1', ok: true }],
      summary: { failed: 0, skipped: 0, succeeded: 2, total: 2 },
    });
  });

  it('says so when a batch stops before its summary', async () => {
    const add = vi.spyOn(toast, 'add');
    const client = createMockApiClient({
      runFleetCommand: vi.fn<ApiClient['runFleetCommand']>(async function* () {
        yield { result: { node_id: 'node_0', ok: true } };
      }),
    });
    await runBatch({ cache, action: 'restart', client, nodes: fleet(2), t });
    expect(report()).toMatchObject({ failure: 'the batch stopped reporting before its summary; check each node\'s state', summary: null });
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
