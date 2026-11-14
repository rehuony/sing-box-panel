import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import { TooltipProvider } from '@shared/components/ui/tooltip';
import { render, screen, within } from '@testing-library/react';

import '@/i18n';
import { ApiRequestError } from '@/api/api-client';
import { ApiClientProvider } from '@/api/api-client-context';
import { CurrentNodeContext } from '@/stores/current-node.store';
import { createMockApiClient, testNode, testStartupArtifact } from '@/tests/api/mock-api-client';
import { AppliedConfigurationsDialog } from '@/pages/configuration-page/applied-configurations-dialog';

describe('applied configurations', () => {
  it('pages the list, names what a rollback leaves alone and keeps a refusal in the confirmation', async () => {
    const user = userEvent.setup();
    const older = {
      ...testStartupArtifact, checked_at: undefined, created_at: '2026-08-20T07:30:00Z', exact_core_version: '1.12.8', id: 'startup_0',
    };
    const refusal = new ApiRequestError('The core version is no longer installed.', { code: 'core_artifact_not_found', status: 409 });
    const cursor = { created_at: testStartupArtifact.created_at, id: testStartupArtifact.id };
    const client = createMockApiClient({
      activateStartupArtifact: vi.fn().mockRejectedValue(refusal),
      listStartupArtifacts: vi.fn(async before =>
        before ? { items: [older] } : { items: [testStartupArtifact], next: cursor }),
    });
    render(
      <ApiClientProvider client={client}>
        <TooltipProvider delay={0}>
          <CurrentNodeContext value={testNode}><AppliedConfigurationsDialog /></CurrentNodeContext>
        </TooltipProvider>
      </ApiClientProvider>,
    );
    await user.click(screen.getByRole('button', { name: 'Applied configurations' }));
    const list = await screen.findByRole('list', { name: 'Applied configurations' });
    expect(within(list).getAllByRole('listitem')).toHaveLength(1);
    await user.click(screen.getByRole('button', { name: 'Load more' }));
    expect(await within(list).findByText(/1\.12\.8/)).toBeInTheDocument();
    expect(client.listStartupArtifacts).toHaveBeenLastCalledWith(cursor, expect.any(AbortSignal));
    expect(screen.queryByRole('button', { name: 'Load more' })).not.toBeInTheDocument();

    await user.click(within(list).getAllByRole('button', { name: 'Roll back' })[1]!);
    const title = `Roll back ${testNode.name} to this applied configuration?`;
    const confirmation = await screen.findByRole('alertdialog', { name: title });
    expect(confirmation).toHaveTextContent('The core restarts, which interrupts the proxy on');
    expect(confirmation).toHaveTextContent('so the next start or restart loads the saved text again');
    await user.click(within(confirmation).getByRole('button', { name: 'Roll back' }));
    expect(await within(confirmation).findByRole('alert')).toHaveTextContent('The core version is no longer installed.');
    expect(client.activateStartupArtifact).toHaveBeenCalledWith('startup_0', 'limited');
    // The list behind the modal confirmation stays as it was.
    expect(within(list).getAllByRole('listitem', { hidden: true })).toHaveLength(2);
  });
});
