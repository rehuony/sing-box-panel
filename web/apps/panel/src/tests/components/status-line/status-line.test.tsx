import { setAppLanguage } from '@shared/i18n/language';
import { render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { TooltipProvider } from '@shared/components/ui/tooltip';

import { ThemeProvider } from '@/theme';
import { TestRouter } from '@/tests/test-router';
import { ApiClientProvider } from '@/api/api-client-context';
import { StatusLine, useShellScope } from '@/components/status-line';
import { AuthSessionProvider } from '@/stores/auth-session-provider';
import { createMockApiClient, testMetrics, testNode } from '@/tests/api/mock-api-client';

function Line() {
  return <StatusLine shell={useShellScope()} />;
}

function renderLine(path: string) {
  const other = { ...testNode, id: 'node_other', name: 'other' };
  const items = [{ metrics: testMetrics, node: testNode }, { metrics: testMetrics, node: other }];
  const client = createMockApiClient({ getFleetStatus: vi.fn().mockResolvedValue({ items }) });
  return render(
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
});
