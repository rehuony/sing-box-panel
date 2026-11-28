import type { QueryClient } from '@tanstack/react-query';

import { describe, expect, it } from 'vitest';
import { useQueryClient } from '@tanstack/react-query';
import { act, render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';

import type { FleetNode, FleetStatus } from '@/api/api-client';

import '@/i18n';
import { NodeWorkspace } from '@/components/node-workspace';
import { ApiClientProvider } from '@/api/api-client-context';
import { useCurrentNode } from '@/stores/current-node.store';
import { createMockApiClient, testFleetStatus, testMetrics, testNode } from '@/tests/api/mock-api-client';

describe('node workspace', () => {
  it('re-renders its view when the node changes, not when it only reports again', async () => {
    let cache: QueryClient | undefined;
    let renders = 0;
    function View() {
      cache = useQueryClient();
      renders += 1;
      return <output>{useCurrentNode().name}</output>;
    }
    // The cache tells its readers on the next task.
    const report = (node: FleetNode) => act(async () => {
      cache!.setQueryData<FleetStatus>(['fleetStatus'], { items: [{ metrics: testMetrics, node }] });
      await new Promise(resolve => setTimeout(resolve, 0));
    });
    render(
      <ApiClientProvider client={createMockApiClient({ getFleetStatus: async () => testFleetStatus })}>
        <MemoryRouter initialEntries={[`/nodes/${testNode.id}`]}>
          <Routes>
            <Route element={<NodeWorkspace />} path='/nodes/:nodeId'>
              <Route element={<View />} index />
            </Route>
          </Routes>
        </MemoryRouter>
      </ApiClientProvider>,
    );
    await screen.findByText('edge', { selector: 'output' });
    const settled = renders;

    const seen = '2026-08-26T07:12:10Z';
    await report({ ...testNode, last_seen_at: seen });
    expect(renders).toBe(settled);

    await report({ ...testNode, last_seen_at: seen, name: 'edge-renamed' });
    expect(screen.getByText('edge-renamed', { selector: 'output' })).toBeInTheDocument();
  });
});
