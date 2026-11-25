import userEvent from '@testing-library/user-event';
import { setAppLanguage } from '@shared/i18n/language';
import { toast } from '@shared/components/ui/toast-manager';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { TooltipProvider } from '@shared/components/ui/tooltip';
import { render, screen, waitFor, within } from '@testing-library/react';

import { ThemeProvider } from '@/theme';
import { TestRouter } from '@/tests/test-router';
import { ApiClientProvider } from '@/api/api-client-context';
import { StatusLine, useShellScope } from '@/components/status-line';
import { AuthSessionProvider } from '@/stores/auth-session-provider';
import { createMockApiClient, testMetrics, testNode } from '@/tests/api/mock-api-client';

function Line() {
  return <StatusLine shell={useShellScope()} />;
}

function renderLine(path: string, overrides: Parameters<typeof createMockApiClient>[0] = {}) {
  const other = { ...testNode, id: 'node_other', name: 'other' };
  const items = [{ metrics: testMetrics, node: testNode }, { metrics: testMetrics, node: other }];
  const client = createMockApiClient({ getFleetStatus: vi.fn().mockResolvedValue({ items }), ...overrides });
  render(
    <ApiClientProvider client={client}>
      <ThemeProvider>
        <TooltipProvider delay={0}>
          <TestRouter initialEntries={[path]}>
            <AuthSessionProvider><Line /></AuthSessionProvider>
          </TestRouter>
        </TooltipProvider>
      </ThemeProvider>
    </ApiClientProvider>,
  );
  return client;
}

describe('status line', () => {
  beforeEach(async () => {
    await setAppLanguage('en');
  });

  it('leads from a node window to the fleet and to the neighbouring nodes in the same window', async () => {
    renderLine(`/nodes/${testNode.id}/configuration`);
    expect(await screen.findByRole('link', { name: 'Next node: other' })).toHaveAttribute('href', '/nodes/node_other/configuration');
    expect(screen.getByRole('link', { name: 'Previous node: other' })).toHaveAttribute('href', '/nodes/node_other/configuration');
    expect(screen.getByRole('link', { name: 'All nodes' })).toHaveAttribute('href', '/nodes');
    expect(screen.queryByText('fleet')).not.toBeInTheDocument();
  });

  it('names the fleet in the panel windows', async () => {
    renderLine('/subscriptions');
    expect(await screen.findByText('fleet')).toHaveAttribute('href', '/nodes');
    expect(screen.queryByRole('link', { name: /^Next node/ })).not.toBeInTheDocument();
  });

  it('restarts the panel after confirming, showing it in the readout\'s slot until the panel reports a later start', async () => {
    const user = userEvent.setup();
    const notify = vi.spyOn(toast, 'add');
    const client = renderLine('/subscriptions', {
      getSystemStatus: vi.fn().mockResolvedValue({ panel_version: 'v1.0.0', platform: { arch: 'amd64', os: 'linux' }, started_at: '2026-09-19T00:00:09Z' }),
      restartPanel: vi.fn().mockResolvedValue({ started_at: '2026-09-19T00:00:00Z' }),
    });
    await user.click(await screen.findByRole('button', { name: 'Restart panel' }));
    const dialog = await screen.findByRole('alertdialog', { name: 'Restart panel' });
    expect(dialog).toHaveTextContent('Nodes and their cores keep running');
    await user.click(within(dialog).getByRole('button', { name: 'Restart panel' }));
    expect(client.restartPanel).toHaveBeenCalledTimes(1);
    await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('Restarting panel…'));
    await waitFor(() => expect(notify).toHaveBeenCalledWith(expect.objectContaining({ title: 'Panel restarted', type: 'success' })), { timeout: 3_000 });
    expect(screen.getByRole('status')).toBeEmptyDOMElement();
  });
});
