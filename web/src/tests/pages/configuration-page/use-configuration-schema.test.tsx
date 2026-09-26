import type { PropsWithChildren } from 'react';

import { act, renderHook } from '@testing-library/react';
import { beforeAll, describe, expect, it, vi } from 'vitest';

import type { ConfigurationSchemaContract, CoreArtifact } from '@/api/api-client';

import { deferred } from '@/tests/deferred';
import { ApiClientProvider } from '@/api/api-client-context';
import { reviewedSchemaManifest } from '@/schemas/generated';
import { createMockApiClient, testArtifacts } from '@/tests/api/mock-api-client';
import { useConfigurationSchema } from '@/pages/configuration-page/use-configuration-schema';

const artifact = testArtifacts.items[0];
let contract: ConfigurationSchemaContract;
beforeAll(async () => {
  const loaded = await reviewedSchemaManifest[artifact.exact_version].load();
  contract = { exact_version: artifact.exact_version, schema_sha256: loaded.schemaSHA256, schema: loaded.schema };
});

function mount(getConfigurationSchema = vi.fn().mockResolvedValue(contract), initial: CoreArtifact | null = artifact) {
  const client = createMockApiClient({ getConfigurationSchema });
  const hook = renderHook(({ selected }) => useConfigurationSchema(selected), {
    initialProps: { selected: initial },
    wrapper: ({ children }: PropsWithChildren) => <ApiClientProvider client={client}>{children}</ApiClientProvider>,
  });
  return { ...hook, client };
}

describe('configuration Schema lifecycle', () => {
  it.each([null, { ...artifact, exact_version: '0.0.0' }])('does not request an unavailable version %#', selected => {
    const { result, client } = mount(undefined, selected);
    expect(result.current.status).toBe('unavailable');
    expect(client.getConfigurationSchema).not.toHaveBeenCalled();
  });

  it('ignores an old response after switching artifacts', async () => {
    const old = deferred<ConfigurationSchemaContract>();
    const next = deferred<ConfigurationSchemaContract>();
    const getSchema = vi.fn().mockReturnValueOnce(old.promise).mockReturnValueOnce(next.promise);
    const { result, rerender } = mount(getSchema);
    await act(() => vi.dynamicImportSettled());
    rerender({ selected: { ...artifact, id: 'next' } });
    expect(getSchema.mock.calls[0][1].aborted).toBe(true);
    expect(result.current).toMatchObject({ status: 'loading', artifactID: 'next' });
    await act(async () => next.resolve(contract));
    await act(() => vi.dynamicImportSettled());
    expect(result.current).toMatchObject({ status: 'ready', artifactID: 'next' });
    await act(async () => old.reject(new Error('late failure')));
    expect(result.current).toMatchObject({ status: 'ready', artifactID: 'next' });
  });

  it('exposes a rejected contract as an error', async () => {
    const error = new Error('offline');
    const { result } = mount(vi.fn().mockRejectedValue(error));
    await act(() => vi.dynamicImportSettled());
    expect(result.current).toMatchObject({ status: 'error', error });
  });

  it('aborts an outstanding request on unmount and ignores late completion', async () => {
    const pending = deferred<ConfigurationSchemaContract>();
    const { client, result, unmount } = mount(vi.fn().mockReturnValue(pending.promise));
    const loading = result.current;
    unmount();
    expect(client.getConfigurationSchema.mock.calls[0][1]?.aborted).toBe(true);
    await act(async () => pending.resolve(contract));
    await act(() => vi.dynamicImportSettled());
    expect(result.current).toBe(loading);
  });
});
