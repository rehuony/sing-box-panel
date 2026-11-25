import type { PropsWithChildren } from 'react';

import { describe, expect, it, vi } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';

import type { ConfigurationSchemaContract, CoreArtifact } from '@/api/api-client';

import { deferred } from '@/tests/deferred';
import { ApiRequestError } from '@/api/api-client';
import { ApiClientProvider } from '@/api/api-client-context';
import { createMockApiClient, testArtifacts } from '@/tests/api/mock-api-client';
import { configurationSchemaContract } from '@/tests/fixtures/configuration-schemas';
import { useConfigurationSchema } from '@/pages/configuration-page/use-configuration-schema';

const artifact = testArtifacts.items[0];
const contract = configurationSchemaContract(artifact.exact_version);

function mount(getConfigurationSchema = vi.fn().mockResolvedValue(contract), initial: CoreArtifact | null = artifact) {
  const client = createMockApiClient({ getConfigurationSchema });
  const hook = renderHook(({ selected }) => useConfigurationSchema(selected), {
    initialProps: { selected: initial },
    wrapper: ({ children }: PropsWithChildren) => <ApiClientProvider client={client}>{children}</ApiClientProvider>,
  });
  return { ...hook, client };
}

describe('configuration Schema lifecycle', () => {
  it('requests nothing without a selected core', () => {
    const { result, client } = mount(undefined, null);
    expect(result.current.status).toBe('unavailable');
    expect(client.getConfigurationSchema).not.toHaveBeenCalled();
  });

  it('uses the panel schema of any version on a supported line', async () => {
    const later = { ...artifact, id: 'core_later', exact_version: '1.14.4' };
    const { result } = mount(vi.fn().mockResolvedValue(configurationSchemaContract('1.14.4')), later);
    await waitFor(() => expect(result.current).toMatchObject({ status: 'ready', artifactID: 'core_later' }));
  });

  it('reports a version the panel has no schema for as unavailable', async () => {
    const refusal = new ApiRequestError('No schema', { status: 409, code: 'configuration_schema_unavailable' });
    const { result } = mount(vi.fn().mockRejectedValue(refusal));
    await waitFor(() => expect(result.current).toMatchObject({ status: 'unavailable', artifactID: artifact.id }));
  });

  it.each([
    new ApiRequestError('Node upgrade required', { status: 409, code: 'node_upgrade_required' }),
    new ApiRequestError('Node offline', { status: 503, code: 'node_offline' }),
  ])('exposes any other failure as an error: %s', async error => {
    const { result } = mount(vi.fn().mockRejectedValue(error));
    await waitFor(() => expect(result.current).toMatchObject({ status: 'error', error }));
  });

  it.each([
    { ...contract, exact_version: '1.13.20' },
    { ...contract, schema: [] as unknown as ConfigurationSchemaContract['schema'] },
  ])('rejects a contract for another version or without an object schema %#', async received => {
    const { result } = mount(vi.fn().mockResolvedValue(received));
    await waitFor(() => expect(result.current.status).toBe('error'));
  });

  it('ignores an old response after switching artifacts', async () => {
    const old = deferred<ConfigurationSchemaContract>();
    const next = deferred<ConfigurationSchemaContract>();
    const getSchema = vi.fn().mockReturnValueOnce(old.promise).mockReturnValueOnce(next.promise);
    const { result, rerender } = mount(getSchema);
    rerender({ selected: { ...artifact, id: 'next' } });
    expect(getSchema.mock.calls[0][1].aborted).toBe(true);
    expect(result.current).toMatchObject({ status: 'loading', artifactID: 'next' });
    await act(async () => next.resolve(contract));
    await waitFor(() => expect(result.current).toMatchObject({ status: 'ready', artifactID: 'next' }));
    await act(async () => old.reject(new Error('late failure')));
    await waitFor(() => expect(result.current).toMatchObject({ status: 'ready', artifactID: 'next' }));
  });

  it('aborts an outstanding request on unmount and ignores late completion', async () => {
    const pending = deferred<ConfigurationSchemaContract>();
    const { client, result, unmount } = mount(vi.fn().mockReturnValue(pending.promise));
    const loading = result.current;
    unmount();
    expect(client.getConfigurationSchema.mock.calls[0][1]?.aborted).toBe(true);
    await act(async () => pending.resolve(contract));
    expect(result.current).toBe(loading);
  });
});
