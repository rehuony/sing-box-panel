import { describe, expect, it } from 'vitest';
import { useLocation } from 'react-router-dom';
import userEvent from '@testing-library/user-event';
import { render, screen, waitFor, within } from '@testing-library/react';

import { TestRouter } from '@/tests/test-router';
import '@/i18n';
import { ApiClientProvider } from '@/api/api-client-context';
import { SubscriptionsPage } from '@/pages/subscriptions-page/subscriptions-page';
import { createMockApiClient, testSubscriptionChannels } from '@/tests/api/mock-api-client';
import { buildPublicSubscriptionURL } from '@/pages/subscriptions-page/public-subscription-url';

function LocationProbe() {
  const location = useLocation();
  return <output data-testid='location'>{JSON.stringify(location)}</output>;
}

describe('subscriptionsPage', () => {
  it('keeps edits on canceled navigation and returns to the channel list after discarding', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient();
    render(<TestRouter><ApiClientProvider client={client}><SubscriptionsPage /></ApiClientProvider></TestRouter>);
    await user.click(screen.getByRole('tab', { name: 'Channels' }));
    const name = await screen.findByRole('cell', { name: testSubscriptionChannels[0].name });
    expect(within(name).queryByRole('button')).not.toBeInTheDocument();
    expect(within(name).queryByRole('link')).not.toBeInTheDocument();
    await user.click(name);
    expect(client.getSubscriptionChannel).not.toHaveBeenCalled();
    expect(screen.getByRole('textbox', { name: 'Search channels' })).toBeVisible();
    await user.click(within(name.closest('tr')!).getByRole('button', { name: 'Edit' }));
    await user.click(await screen.findByRole('button', { name: 'Add strategy group' }));
    expect(screen.queryByRole('tab', { name: 'Node selection' })).not.toBeInTheDocument();
    expect(screen.queryByRole('tab', { name: 'Strategy groups' })).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /^Configure Strategy group/ }));
    await user.type(screen.getByLabelText('Group name'), ' edited');
    await user.click(screen.getByRole('button', { name: 'Done' }));
    const back = screen.getByRole('button', { name: 'Back' });
    expect(back.closest('.workspace-toolbar')).not.toBeNull();
    await user.click(screen.getByRole('tab', { name: 'Sources' }));
    const confirmation = screen.getByRole('alertdialog', { name: 'Discard unsaved changes?' });
    expect(within(confirmation).queryByRole('button', { name: 'Close' })).not.toBeInTheDocument();
    await waitFor(() => expect(within(confirmation).getByRole('button', { name: 'Keep editing' })).toHaveFocus());
    await user.click(within(confirmation).getByRole('button', { name: 'Keep editing' }));
    expect(screen.getByRole('tab', { name: 'Channels' })).toHaveAttribute('aria-selected', 'true');
    await user.click(screen.getByRole('button', { name: /^Configure Strategy group/ }));
    expect(screen.getByLabelText('Group name')).toHaveValue('Strategy group 1 edited');
    await user.click(screen.getByRole('button', { name: 'Cancel' }));
    await user.click(screen.getByRole('tab', { name: 'Sources' }));
    await user.click(screen.getByRole('button', { name: 'Discard changes' }));
    expect(screen.getByRole('tab', { name: 'Sources' })).toHaveAttribute('aria-selected', 'true');
    await user.click(screen.getByRole('tab', { name: 'Channels' }));
    expect(await screen.findByRole('textbox', { name: 'Search channels' })).toBeVisible();
    expect(screen.queryByRole('button', { name: 'Back' })).not.toBeInTheDocument();
    expect(client.updateSubscriptionChannel).not.toHaveBeenCalled();
  });

  it('resets sections and shows only the active actions', async () => {
    const user = userEvent.setup();
    const originalURL = window.location.href;
    try {
      window.history.replaceState(window.history.state, '', '/subscriptions');
      render(
        <TestRouter>
          <ApiClientProvider client={createMockApiClient()}><SubscriptionsPage /></ApiClientProvider>
        </TestRouter>,
      );
      await user.type(screen.getByRole('textbox', { name: 'Search sources or nodes' }), 'source filter');
      await user.click(screen.getByRole('tab', { name: 'Channels' }));
      expect(screen.queryByRole('textbox', { name: 'Search sources or nodes' })).not.toBeInTheDocument();
      await user.type(screen.getByRole('textbox', { name: 'Search channels' }), 'channel filter');
      await user.click(screen.getByRole('tab', { name: 'Key management' }));
      expect(screen.queryByRole('textbox')).not.toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Create key' })).toBeVisible();
      expect(screen.queryByRole('button', { name: 'Add channel' })).not.toBeInTheDocument();
      await user.click(screen.getByRole('tab', { name: 'Sources' }));
      expect(screen.getByRole('textbox', { name: 'Search sources or nodes' })).toHaveValue('');
      expect(screen.queryByRole('button', { name: 'Create key' })).not.toBeInTheDocument();
      await user.click(screen.getByRole('tab', { name: 'Channels' }));
      expect(screen.getByRole('textbox', { name: 'Search channels' })).toHaveValue('');
    } finally {
      window.history.replaceState(window.history.state, '', originalURL);
    }
  });

  it('builds the public subscription path under the configured document base', () => {
    expect(buildPublicSubscriptionURL(
      'secret/token',
      'channel main',
      'https://panel.example/panel/',
    )).toBe('https://panel.example/panel/sub/secret%2Ftoken/channel%20main');
  });

  it('leaves an unedited legacy channel without treating its available upgrade as unsaved edits', async () => {
    const user = userEvent.setup();
    render(<TestRouter initialEntries={['/subscriptions#subscription-channels']}><ApiClientProvider client={createMockApiClient()}><SubscriptionsPage /></ApiClientProvider></TestRouter>);
    const row = (await screen.findByRole('cell', { name: testSubscriptionChannels[0].name })).closest('tr')!;
    await user.click(within(row).getByRole('button', { name: 'Edit' }));
    expect(await screen.findByRole('button', { name: 'Save changes' })).toBeEnabled();
    await user.click(screen.getByRole('tab', { name: 'Sources' }));
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();
    await user.click(screen.getByRole('tab', { name: 'Channels' }));
    expect(await screen.findByRole('textbox', { name: 'Search channels' })).toBeVisible();
    expect(screen.queryByRole('button', { name: 'Back' })).not.toBeInTheDocument();
  });

  it('keeps the current route, query and user state for section navigation under a base path', async () => {
    const user = userEvent.setup();
    render(
      <TestRouter basename='/panel' initialEntries={[{ pathname: '/panel/subscriptions/', search: '?view=active', state: { from: 'tests' } }]}>
        <ApiClientProvider client={createMockApiClient()}>
          <SubscriptionsPage />
          <LocationProbe />
        </ApiClientProvider>
      </TestRouter>,
    );
    await user.click(screen.getByRole('tab', { name: 'Channels' }));
    expect(JSON.parse(screen.getByTestId('location').textContent!)).toMatchObject({
      pathname: '/subscriptions/', search: '?view=active', hash: '#subscription-channels', state: { from: 'tests' },
    });
  });

  it('creates a channel and opens the strategy-group workspace directly', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient();
    render(<TestRouter><ApiClientProvider client={client}><SubscriptionsPage /></ApiClientProvider></TestRouter>);

    await user.click(screen.getByRole('tab', { name: 'Channels' }));
    await user.click(screen.getByRole('button', { name: 'Add channel' }));
    const dialog = await screen.findByRole('dialog', { name: 'Add channel' });
    await user.type(within(dialog).getByLabelText('Channel name'), 'Mobile clients');
    await user.click(within(dialog).getByRole('combobox', { name: 'Output client' }));
    await user.click(await screen.findByRole('option', { name: 'Mihomo' }));
    await user.click(within(dialog).getByRole('button', { name: 'Add channel' }));
    expect(client.createSubscriptionChannel).toHaveBeenCalledWith({
      name: 'Mobile clients', format: 'mihomo', public_host: '', enabled: true,
      config: { policy: { selection: { ids: [], excluded_ids: [], new_node_policy: 'exclude' },
        groups: [], default_exit: { kind: 'direct' } } },
    }, expect.any(AbortSignal));
    expect(await screen.findByRole('button', { name: 'Add strategy group' })).toBeVisible();
    expect(screen.queryByRole('tab', { name: 'Node selection' })).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Add strategy group' }));
    expect(screen.getByRole('region', { name: 'Edit strategy group' })).toBeVisible();
    expect(screen.getByRole('button', { name: 'Set as final exit' })).toHaveAttribute('aria-pressed', 'false');
  });
});
