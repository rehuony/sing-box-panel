import type { QueryClient } from '@tanstack/react-query';

import userEvent from '@testing-library/user-event';
import { useQueryClient } from '@tanstack/react-query';
import { act, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import type { FleetStatus, NodeEnrollment } from '@/api/api-client';

import { TestRouter } from '@/tests/test-router';
import { ApiClientProvider } from '@/api/api-client-context';
import { EnrollmentDialog } from '@/components/enrollment-dialog';
import { useEnrollmentDialog } from '@/stores/enrollment-dialog.store';
import { createMockApiClient, testMetrics, testNode } from '@/tests/api/mock-api-client';

const enrollment: NodeEnrollment = {
  code: 'sbp1.code',
  command: 'sing-box-node enroll --code sbp1.code',
  expires_at: new Date(Date.now() + 3_600_000).toISOString(),
  name: 'edge-tokyo',
  node_id: 'node_new',
};

let cache: QueryClient;
function CacheProbe() {
  cache = useQueryClient();
  return null;
}

afterEach(() => act(() => useEnrollmentDialog.getState().reset()));

describe('enrollment dialog', () => {
  it('discards an unused code when closed while waiting, but not once the node enrolled', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient({
      createNode: vi.fn().mockResolvedValue(enrollment),
      getFleetStatus: vi.fn().mockResolvedValue({ items: [] }),
    });
    render(
      <ApiClientProvider client={client}>
        <TestRouter>
          <CacheProbe />
          <EnrollmentDialog />
        </TestRouter>
      </ApiClientProvider>,
    );
    const generate = async () => {
      act(() => useEnrollmentDialog.getState().add());
      await user.type(await screen.findByRole('textbox', { name: 'Name' }), 'edge-tokyo');
      await user.click(screen.getByRole('button', { name: 'Generate enrollment command' }));
      expect(await screen.findByText(enrollment.command, { exact: false })).toBeVisible();
    };

    await generate();
    await user.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(client.cancelNodeEnrollment).toHaveBeenCalledExactlyOnceWith('node_new', false);

    // The node counts as enrolled once the fleet the stream feeds shows it.
    await generate();
    act(() => cache.setQueryData<FleetStatus>(['fleetStatus'], {
      items: [{ metrics: testMetrics, node: { ...testNode, id: 'node_new', name: 'edge-tokyo' } }],
    }));
    expect(await screen.findByText('Enrolled')).toBeVisible();
    await user.click(screen.getByRole('button', { name: 'Done' }));
    expect(client.cancelNodeEnrollment).toHaveBeenCalledOnce();
  });
});
