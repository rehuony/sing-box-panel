import type { PropsWithChildren } from 'react';

import { describe, expect, it, vi } from 'vitest';
import { act, renderHook } from '@testing-library/react';

import type { ApiClient } from '@/api/api-client';

import '@/i18n';
import { deferred } from '@/tests/deferred';
import { TestRouter } from '@/tests/test-router';
import { toast } from '@/components/ui/toast-manager';
import { ApiClientProvider } from '@/api/api-client-context';
import { createMockApiClient, testSubscriptionSources } from '@/tests/api/mock-api-client';
import { useSubscriptionSources } from '@/pages/subscriptions-page/use-subscription-sources';

async function mount(overrides: Partial<ApiClient> = {}) {
  const client = createMockApiClient(overrides);
  const hook = renderHook(useSubscriptionSources, {
    wrapper: ({ children }: PropsWithChildren) => (
      <TestRouter><ApiClientProvider client={client}>{children}</ApiClientProvider></TestRouter>
    ),
  });
  await act(async () => {});
  return { ...hook, client };
}

describe('subscription sources', () => {
  it('loads nodes concurrently with sources and ignores a superseded read', async () => {
    const old = deferred<Awaited<ReturnType<ApiClient['listSubscriptionSources']>>>();
    const { result, client } = await mount({
      listSubscriptionSources: vi.fn().mockReturnValueOnce(old.promise).mockResolvedValue({ items: [] }),
    });
    expect(client.getSubscriptionNodeCatalog).toHaveBeenCalledOnce();
    await act(async () => result.current.reload());
    await act(async () => old.resolve({
      items: testSubscriptionSources.map(source => ({ ...source, has_version: false })),
    }));
    expect(result.current.sources).toEqual([]);
  });
  it('polls on virtual time and aborts reads on unmount', async () => {
    vi.useFakeTimers();
    const { client, unmount } = await mount();
    await act(() => vi.advanceTimersByTimeAsync(15_000));
    expect(client.listSubscriptionSources).toHaveBeenCalledTimes(2);
    unmount();
    expect(client.listSubscriptionSources.mock.calls[0][1]?.aborted).toBe(true);
    await vi.advanceTimersByTimeAsync(30_000);
    expect(client.listSubscriptionSources).toHaveBeenCalledTimes(2);
  });
  it.each([true, false])('uses fresh source metadata and preserves enablement %s on a CAS save', async enabled => {
    const source = { ...testSubscriptionSources[0], enabled, updated_at: 'fresh-revision' };
    const { result, client } = await mount({ getSubscriptionSource: vi.fn().mockResolvedValue(source) });
    act(() => result.current.openSource(source.id));
    await act(() => result.current.openSettings());
    act(() => result.current.setForm({ ...result.current.form!, name: ' Renamed ' }));
    await act(() => result.current.saveSource());
    expect(client.updateSubscriptionSource).toHaveBeenCalledWith(source.id,
      expect.objectContaining({ name: 'Renamed', enabled, config: expect.objectContaining(source.config) }),
      'fresh-revision', expect.any(AbortSignal));
  });
  it('creates remote sources with minute-based intervals and retains failed drafts', async () => {
    const { result, client } = await mount({ createSubscriptionSource: vi.fn().mockRejectedValue(new Error('offline')) });
    act(() => {
      result.current.setCreating(true);
      result.current.setForm({ ...result.current.newSource(), name: 'Source', url: 'https://example.com/sub', interval: '30' });
    });
    await act(() => result.current.saveSource());
    expect(client.createSubscriptionSource).toHaveBeenCalledWith({ name: 'Source', enabled: true, source_kind: 'remote', config: { url: 'https://example.com/sub', format: 'auto', refresh_interval_minutes: 30 } }, expect.any(AbortSignal));
    expect(result.current.creating).toBe(true);
    expect(result.current.form?.name).toBe('Source');
    expect(result.current.formError).toBe('offline');
  });
  it('keeps local sources in the manual collection without changing node identity', async () => {
    const local = { ...testSubscriptionSources[0], source_kind: 'local' as const };
    const { result } = await mount({ listSubscriptionSources: vi.fn().mockResolvedValue({ items: [local] }) });
    expect(result.current.displayedSources.map(source => source.id)).toEqual(['manual']);
    const node = { ...result.current.nodes[0], source_id: local.id, origin: 'source' as const };
    expect(result.current.inManualCollection(node)).toBe(true);
    expect(result.current.inManualCollection({ ...node, source_id: 'remote' })).toBe(false);
  });
  it('uses only valid recorded starts and retains them when later history reads fail', async () => {
    const started = '2026-09-20T01:00:00Z';
    const { result, client } = await mount({ getRuntimeHistory: vi.fn().mockResolvedValueOnce({ items: [{ process_started_at: started }] }).mockRejectedValue(new Error('offline')) });
    expect(result.current.displayedSources[0].updated_at).toBe(started);
    await act(async () => result.current.reload());
    expect(result.current.displayedSources[0].updated_at).toBe(started);
    expect(client.getRuntimeHistory).toHaveBeenCalledTimes(2);
  });
  it.each([{ items: [] }, { items: [{ process_started_at: 'invalid' }] }])('does not invent a manual update time from %j', async ({ items }) => {
    const { result } = await mount({ getRuntimeHistory: vi.fn().mockResolvedValue({ items }) });
    expect(result.current.displayedSources[0].updated_at).toBe('');
  });
  it('refreshes persisted data after all refresh outcomes without changing node visibility', async () => {
    const notice = vi.spyOn(toast, 'add');
    const { result, client } = await mount({ refreshSubscriptionSource: vi.fn().mockResolvedValueOnce({}).mockRejectedValueOnce(new Error('offline')) });
    const initialNodes = result.current.nodes;
    await act(() => result.current.refresh(['first', 'second']));
    expect(client.refreshSubscriptionSource).toHaveBeenCalledTimes(2);
    expect(client.invalidateReadCache).toHaveBeenCalledOnce();
    expect(client.getSubscriptionNodeCatalog).toHaveBeenCalledTimes(2);
    expect(result.current.nodes).toEqual(initialNodes);
    expect(result.current.busy).toBe(false);
    expect(notice).toHaveBeenCalledWith(expect.objectContaining({ type: 'error' }));
    notice.mockRestore();
  });
});
