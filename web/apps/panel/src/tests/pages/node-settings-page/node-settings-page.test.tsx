import { describe, expect, it } from 'vitest';
import userEvent from '@testing-library/user-event';
import { TooltipProvider } from '@shared/components/ui/tooltip';
import { render, screen, within } from '@testing-library/react';

import '@/i18n';
import { TestRouter } from '@/tests/test-router';
import { ApiClientProvider } from '@/api/api-client-context';
import { CurrentNodeContext } from '@/stores/current-node.store';
import { createMockApiClient, testNode } from '@/tests/api/mock-api-client';
import { NodeSettingsPage } from '@/pages/node-settings-page/node-settings-page';

describe('node settings', () => {
  it('counts unsaved rows and keeps the update and re-enrollment beside what they change', async () => {
    const user = userEvent.setup();
    render(
      <TestRouter>
        <ApiClientProvider client={createMockApiClient()}>
          <TooltipProvider>
            <CurrentNodeContext value={{ ...testNode, update_available: true }}>
              <NodeSettingsPage />
            </CurrentNodeContext>
          </TooltipProvider>
        </ApiClientProvider>
      </TestRouter>,
    );
    await user.type(await screen.findByRole('textbox', { name: 'Published host' }), 'edge.example.com');
    expect(screen.getByText('1 unsaved change')).toBeVisible();

    const profile = screen.getByRole('region', { name: 'Node profile' });
    expect(profile).toHaveTextContent('linux/arm64');
    expect(within(profile).getByRole('button', { name: 'Update' })).toBeVisible();
    await user.click(within(profile).getByRole('button', { name: 'Re-enroll' }));
    expect(await screen.findByRole('alertdialog', { name: `Re-enroll ${testNode.name}?` })).toBeVisible();
  });
});
