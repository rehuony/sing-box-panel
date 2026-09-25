import type { PropsWithChildren } from 'react';

import { describe, expect, it, vi } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';

import type { ApiClient, RuntimeStatus } from '@/api/api-client';

import { ApiClientProvider } from '@/api/api-client-context';
import { useVersionLibrary } from '@/pages/cores-page/use-version-library';
import { useSharedTelemetry } from '@/components/app-shell/telemetry-context';
import { TelemetryProvider } from '@/components/app-shell/telemetry-provider';
import { createMockApiClient, testArtifacts, testRuntimeStatus } from '@/tests/api/mock-api-client';

const stopped: RuntimeStatus = {
  ...testRuntimeStatus,
  observation_state: 'stopped',
  desired_running: false,
  running: undefined,
};

function mount(client: ApiClient) {
  return renderHook(() => ({ library: useVersionLibrary(), telemetry: useSharedTelemetry() }), {
    wrapper: ({ children }: PropsWithChildren) => (
      <ApiClientProvider client={client}>
        <TelemetryProvider>{children}</TelemetryProvider>
      </ApiClientProvider>
    ),
  });
}

describe('version library runtime ordering', () => {
  it('does not replay the initial runtime snapshot after a slow artifact list finishes', async () => {
    let finish!: (value: typeof testArtifacts) => void;
    const client = createMockApiClient({
      listCoreArtifacts: vi.fn(() => new Promise<typeof testArtifacts>(resolve => {
        finish = resolve;
      })),
    });
    const { result } = mount(client);
    await waitFor(() => expect(client.listCoreArtifacts).toHaveBeenCalled());
    act(() => result.current.telemetry.acceptRuntimeStatus(stopped));
    await act(async () => finish(testArtifacts));
    await waitFor(() => expect(result.current.library.installedLoading).toBe(false));
    expect(result.current.library.runtime).toEqual(stopped);
    expect(result.current.telemetry.runtimeStatus).toEqual(stopped);
    expect(client.getRuntimeStatus).toHaveBeenCalledOnce();
  });

  it.each(['entry', 'refresh'] as const)('discards a pending %s read after newer runtime evidence', async phase => {
    let finish!: (value: RuntimeStatus) => void;
    const pending = () => new Promise<RuntimeStatus>(resolve => {
      finish = resolve;
    });
    const client = createMockApiClient();
    if (phase === 'entry') client.getRuntimeStatus.mockImplementationOnce(pending);
    const { result } = mount(client);
    await waitFor(() => expect(client.getRuntimeStatus).toHaveBeenCalled());
    if (phase === 'refresh') {
      await waitFor(() => expect(result.current.library.installedLoading).toBe(false));
      client.getRuntimeStatus.mockImplementationOnce(pending);
      act(() => {
        void result.current.library.refreshInstalled();
      });
    }
    await act(async () => {
      result.current.telemetry.acceptRuntimeStatus(stopped);
      finish(testRuntimeStatus);
    });
    await waitFor(() => expect(result.current.library.installedLoading).toBe(false));
    expect(result.current.telemetry.runtimeStatus).toEqual(stopped);
    expect(result.current.library.runtime).toEqual(stopped);
  });
});
