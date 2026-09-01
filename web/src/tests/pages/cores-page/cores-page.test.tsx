import type { ReactNode } from 'react';

import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import { act, render, renderHook, screen, waitFor, within } from '@testing-library/react';

import '@/i18n';

import type { ApiClient } from '@/api/api-client';

import { CoresPage } from '@/pages/cores-page/cores-page';
import { ApiClientProvider } from '@/api/api-client-context';
import { ControlPlaneContext } from '@/stores/control-plane.store';
import { useCoreLibraryState } from '@/pages/cores-page/use-core-library-state';
import {
  createMockApiClient,
  testArtifacts,
  testDashboardContext,
  testStartupArtifact,
  testTask,
} from '@/tests/api/mock-api-client';

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((accept) => {
    resolve = accept;
  });
  return { promise, resolve };
}

function renderCores(client: ApiClient) {
  render(
    <MemoryRouter>
      <ApiClientProvider client={client}>
        <ControlPlaneContext value={{
          status: 'ready',
          context: testDashboardContext,
          message: null,
          refresh: vi.fn().mockResolvedValue(undefined),
          setViewVersion: vi.fn(),
          viewVersion: testDashboardContext.view.exactVersion,
        }}>
          <CoresPage />
        </ControlPlaneContext>
      </ApiClientProvider>
    </MemoryRouter>,
  );
}

describe('coresPage artifact inspection', () => {
  it('ignores an older artifact-list response after a newer request completes', async () => {
    const first = deferred<typeof testArtifacts>();
    const secondArtifact = { ...testArtifacts.items[0], id: 'core_2' };
    const second = deferred<typeof testArtifacts>();
    const listCoreArtifacts = vi.fn()
      .mockResolvedValueOnce(testArtifacts)
      .mockImplementationOnce(() => first.promise)
      .mockImplementationOnce(() => second.promise);
    const client = createMockApiClient({ listCoreArtifacts });
    const wrapper = ({ children }: { children: ReactNode }) => (
      <ApiClientProvider client={client}>{children}</ApiClientProvider>
    );
    const { result } = renderHook(() => useCoreLibraryState('1.13.19'), { wrapper });
    await waitFor(() => expect(result.current.artifacts?.items[0].id).toBe('core_1'));

    let firstLoad!: Promise<void>;
    let secondLoad!: Promise<void>;
    act(() => {
      firstLoad = result.current.loadArtifacts();
      secondLoad = result.current.loadArtifacts();
    });
    second.resolve({ ...testArtifacts, items: [secondArtifact] });
    await act(async () => secondLoad);
    expect(result.current.artifacts?.items[0].id).toBe('core_2');

    first.resolve(testArtifacts);
    await act(async () => firstLoad);
    expect(result.current.artifacts?.items[0].id).toBe('core_2');
  });

  it('ignores stale inspection responses after a different artifact is opened', async () => {
    const user = userEvent.setup();
    const secondArtifact = {
      ...testArtifacts.items[0],
      id: 'core_2',
      binary_sha256: 'e'.repeat(64),
    };
    let finishFirstArtifact: ((artifact: typeof testArtifacts.items[number]) => void) | undefined;
    let finishFirstStartup: ((page: { items: Array<typeof testStartupArtifact> }) => void) | undefined;
    const firstArtifact = new Promise<typeof testArtifacts.items[number]>((resolve) => {
      finishFirstArtifact = resolve;
    });
    const firstStartup = new Promise<{ items: Array<typeof testStartupArtifact> }>((resolve) => {
      finishFirstStartup = resolve;
    });
    const client = createMockApiClient({
      listCoreArtifacts: vi.fn().mockResolvedValue({ items: [testArtifacts.items[0], secondArtifact] }),
      getCoreArtifact: vi.fn().mockImplementation((id) => id === 'core_1'
        ? firstArtifact
        : Promise.resolve(secondArtifact)),
      listStartupArtifacts: vi.fn().mockImplementation((filter) => filter.coreArtifactID === 'core_1'
        ? firstStartup
        : Promise.resolve({ items: [] })),
    });

    renderCores(client);
    await user.click(await screen.findByRole('button', {
      name: 'View details for artifact 1.13.19 arm64/plain (core_1)',
    }));
    await user.click(screen.getByRole('button', { name: 'Close' }));
    await user.click(screen.getByRole('button', {
      name: 'View details for artifact 1.13.19 arm64/plain (core_2)',
    }));

    expect(await screen.findByText('e'.repeat(64))).toBeInTheDocument();
    const detail = screen.getByRole('dialog');
    finishFirstArtifact?.(testArtifacts.items[0]);
    finishFirstStartup?.({ items: [testStartupArtifact] });

    await waitFor(() => {
      expect(within(detail).getAllByText('core_2')).toHaveLength(2);
      expect(within(detail).queryByText('d'.repeat(64))).not.toBeInTheDocument();
    });
  });

  it('loads full artifact evidence and queues checks only for pending startup artifacts', async () => {
    const user = userEvent.setup();
    const pendingStartup = {
      ...testStartupArtifact,
      state: 'pending' as const,
      checked_at: undefined,
    };
    const readyStartup = {
      ...testStartupArtifact,
      id: 'startup_ready',
    };
    const checkTask = {
      ...testTask,
      id: 'task_startup_check',
      kind: 'startup-check',
      status: 'succeeded' as const,
    };
    const client = createMockApiClient({
      listStartupArtifacts: vi.fn().mockResolvedValue({
        items: [pendingStartup, readyStartup],
      }),
      checkStartupArtifact: vi.fn().mockResolvedValue(checkTask),
    });
    const artifactLabel = '1.13.19 arm64/plain (core_1)';

    renderCores(client);
    await user.click(await screen.findByRole('button', {
      name: `View details for artifact ${artifactLabel}`,
    }));

    await waitFor(() => {
      expect(client.getCoreArtifact).toHaveBeenCalledWith('core_1', expect.any(AbortSignal));
      expect(client.listStartupArtifacts).toHaveBeenCalledWith(
        { coreArtifactID: 'core_1', limit: 100 },
        expect.any(AbortSignal),
      );
    });
    expect(await screen.findByText('d'.repeat(64))).toBeInTheDocument();
    expect(screen.getByText('{"status":"reported","features":["with_quic"]}')).toBeInTheDocument();
    expect(screen.getByText('/var/lib/sing-box-panel/artifacts/core_1/sing-box')).toBeInTheDocument();

    const readyRow = screen.getByText('startup_ready').closest('li');
    expect(readyRow).not.toBeNull();
    expect(within(readyRow!).queryByRole('button', { name: 'Run startup check' })).not.toBeInTheDocument();
    expect(screen.getAllByRole('button', { name: 'Run startup check' })).toHaveLength(1);

    await user.click(screen.getByRole('button', { name: 'Run startup check' }));
    expect(client.checkStartupArtifact).toHaveBeenCalledWith('startup_1');
    const detail = screen.getByRole('dialog');
    const taskStatus = await within(detail).findByText('Task accepted');
    const taskResult = taskStatus.parentElement;
    expect(taskResult).not.toBeNull();
    expect(within(taskResult!).getByText('task_startup_check')).toBeInTheDocument();
    expect(within(taskResult!).getByRole('link', { name: 'Open tasks' })).toHaveAttribute('href', '/tasks');
    expect(within(taskResult!).queryByText('succeeded')).not.toBeInTheDocument();
  });

  it('blocks new startup checks when the core trust state is restricted', async () => {
    const user = userEvent.setup();
    const restrictedArtifact = {
      ...testArtifacts.items[0],
      verification_state: 'quarantined' as const,
    };
    const client = createMockApiClient({
      listCoreArtifacts: vi.fn().mockResolvedValue({ items: [restrictedArtifact] }),
      getCoreArtifact: vi.fn().mockResolvedValue(restrictedArtifact),
      listStartupArtifacts: vi.fn().mockResolvedValue({
        items: [{ ...testStartupArtifact, state: 'pending', checked_at: undefined }],
      }),
    });
    const artifactLabel = '1.13.19 arm64/plain (core_1)';

    renderCores(client);
    await user.click(await screen.findByRole('button', {
      name: `View details for artifact ${artifactLabel}`,
    }));

    expect(await screen.findByText('Blocked by artifact trust state')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Run startup check' })).not.toBeInTheDocument();
    expect(client.checkStartupArtifact).not.toHaveBeenCalled();
  });
});
