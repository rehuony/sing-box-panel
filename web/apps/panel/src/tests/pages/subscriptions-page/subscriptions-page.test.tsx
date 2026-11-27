import { useLocation } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import { render, screen, waitFor, within } from '@testing-library/react';

import { TestRouter } from '@/tests/test-router';
import '@/i18n';
import { ApiClientProvider } from '@/api/api-client-context';
import { SubscriptionsPage } from '@/pages/subscriptions-page/subscriptions-page';
import { createMockApiClient, testSubscriptionChannels } from '@/tests/api/mock-api-client';
import { emptyChannelConfig, newRuleGroup } from '@/pages/subscriptions-page/channel-policy';
import { buildPublicSubscriptionURL } from '@/pages/subscriptions-page/public-subscription-url';

function LocationProbe() {
  const location = useLocation();
  return <output data-testid='location'>{JSON.stringify(location)}</output>;
}

describe('subscriptionsPage', () => {
  it('opens a channel on its exit rules and starts groups on candidates again after reopening', async () => {
    const user = userEvent.setup();
    const group = { ...newRuleGroup(), name: 'Proxy' };
    const channel = { ...testSubscriptionChannels[0], config: { ...emptyChannelConfig(), groups: [group] } };
    const client = createMockApiClient({ getSubscriptionChannel: vi.fn().mockResolvedValue(channel) });
    render(<TestRouter initialEntries={['/subscriptions#subscription-channels']}><ApiClientProvider client={client}><SubscriptionsPage /></ApiClientProvider></TestRouter>);
    await user.click(await screen.findByRole('button', { name: channel.name }));
    expect(await screen.findByRole('region', { name: 'Exit rules' })).toBeVisible();
    await user.click(screen.getByRole('button', { name: /^Proxy/ }));
    await user.click(await screen.findByRole('tab', { name: 'Exit rules' }));
    await user.click(screen.getByRole('button', { name: 'Back' }));
    await user.click(await screen.findByRole('button', { name: channel.name }));
    expect(await screen.findByRole('region', { name: 'Exit rules' })).toBeVisible();
    await user.click(screen.getByRole('button', { name: /^Proxy/ }));
    expect(await screen.findByRole('tab', { name: 'Candidate nodes' })).toHaveAttribute('aria-selected', 'true');
    expect(client.updateSubscriptionChannel).not.toHaveBeenCalled();
  });

  it('names the open channel in the address, so a reload opens it again and back returns to the list', async () => {
    const user = userEvent.setup();
    const [channel] = testSubscriptionChannels;
    const hash = () => JSON.parse(screen.getByTestId('location').textContent!).hash;
    const client = createMockApiClient();
    const { unmount } = render(
      <TestRouter initialEntries={['/subscriptions#subscription-channels']}>
        <ApiClientProvider client={client}>
          <SubscriptionsPage />
          <LocationProbe />
        </ApiClientProvider>
      </TestRouter>,
    );
    await user.click(await screen.findByRole('button', { name: channel.name }));
    expect(await screen.findByRole('button', { name: 'Back' })).toBeVisible();
    expect(hash()).toBe(`#subscription-channels/${channel.id}`);
    await user.click(screen.getByRole('button', { name: 'Back' }));
    expect(await screen.findByRole('searchbox', { name: 'Search channels' })).toBeVisible();
    expect(hash()).toBe('#subscription-channels');
    unmount();

    // A reload starts on the channel's address.
    render(
      <TestRouter initialEntries={[`/subscriptions#subscription-channels/${channel.id}`]}>
        <ApiClientProvider client={client}>
          <SubscriptionsPage />
          <LocationProbe />
        </ApiClientProvider>
      </TestRouter>,
    );
    await user.click(await screen.findByRole('button', { name: 'Back' }));
    expect(await screen.findByRole('searchbox', { name: 'Search channels' })).toBeVisible();
    expect(hash()).toBe('#subscription-channels');
    expect(client.getSubscriptionChannel).toHaveBeenLastCalledWith(channel.id, expect.any(AbortSignal));
  });

  it('keeps edits on canceled navigation and returns to the channel list after discarding', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient();
    render(<TestRouter><ApiClientProvider client={client}><SubscriptionsPage /></ApiClientProvider></TestRouter>);
    await user.click(screen.getByRole('tab', { name: 'Channels' }));
    await user.click(await screen.findByRole('button', { name: testSubscriptionChannels[0].name }));
    await user.click(await screen.findByRole('button', { name: 'Add strategy group' }));
    expect(screen.queryByRole('tab', { name: 'Node selection' })).not.toBeInTheDocument();
    expect(screen.queryByRole('tab', { name: 'Strategy groups' })).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /^Configure Strategy group/ }));
    await user.type(screen.getByLabelText('Group name'), ' edited');
    await user.click(screen.getByRole('button', { name: 'Apply' }));
    const back = screen.getByRole('button', { name: 'Back' });
    expect(back.closest('[data-slot="title-strip"]')).not.toBeNull();
    await user.click(screen.getByRole('tab', { name: 'Rule sets' }));
    const confirmation = screen.getByRole('alertdialog', { name: 'Discard unsaved changes?' });
    expect(within(confirmation).queryByRole('button', { name: 'Close' })).not.toBeInTheDocument();
    await waitFor(() => expect(within(confirmation).getByRole('button', { name: 'Cancel' })).toHaveFocus());
    await user.click(within(confirmation).getByRole('button', { name: 'Cancel' }));
    expect(screen.getByRole('tab', { name: 'Channels' })).toHaveAttribute('aria-selected', 'true');
    await user.click(screen.getByRole('button', { name: /^Configure Strategy group/ }));
    expect(screen.getByLabelText('Group name')).toHaveValue('Strategy group 1 edited');
    await user.click(screen.getByRole('button', { name: 'Cancel' }));
    await user.click(screen.getByRole('tab', { name: 'Rule sets' }));
    await user.click(screen.getByRole('button', { name: 'Discard changes' }));
    expect(screen.getByRole('tab', { name: 'Rule sets' })).toHaveAttribute('aria-selected', 'true');
    await user.click(screen.getByRole('tab', { name: 'Channels' }));
    expect(await screen.findByRole('searchbox', { name: 'Search channels' })).toBeVisible();
    expect(screen.queryByRole('button', { name: 'Back' })).not.toBeInTheDocument();
    expect(client.updateSubscriptionChannel).not.toHaveBeenCalled();
  });

  it('resets sections and shows only the active actions', async () => {
    const user = userEvent.setup();
    const originalURL = window.location.href;
    try {
      window.history.replaceState(window.history.state, '', '/subscriptions#subscription-nodes');
      render(
        <TestRouter initialEntries={['/subscriptions#subscription-nodes']}>
          <ApiClientProvider client={createMockApiClient()}><SubscriptionsPage /></ApiClientProvider>
        </TestRouter>,
      );
      await user.type(screen.getByRole('searchbox', { name: 'Search nodes' }), 'node filter');
      await user.click(screen.getByRole('tab', { name: 'Channels' }));
      expect(screen.queryByRole('searchbox', { name: 'Search nodes' })).not.toBeInTheDocument();
      await user.type(screen.getByRole('searchbox', { name: 'Search channels' }), 'channel filter');
      await user.click(screen.getByRole('tab', { name: 'Key management' }));
      expect(screen.queryByRole('textbox')).not.toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Create key' })).toBeVisible();
      expect(screen.queryByRole('button', { name: 'Add channel' })).not.toBeInTheDocument();
      await user.click(screen.getByRole('tab', { name: 'Nodes' }));
      expect(screen.getByRole('searchbox', { name: 'Search nodes' })).toHaveValue('');
      expect(screen.queryByRole('button', { name: 'Create key' })).not.toBeInTheDocument();
      await user.click(screen.getByRole('tab', { name: 'Channels' }));
      expect(screen.getByRole('searchbox', { name: 'Search channels' })).toHaveValue('');
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
    await user.click(within(dialog).getByRole('combobox', { name: 'Client' }));
    await user.click(await screen.findByRole('option', { name: 'mihomo' }));
    await user.click(within(dialog).getByRole('button', { name: 'Add channel' }));
    expect(client.createSubscriptionChannel).toHaveBeenCalledWith({
      name: 'Mobile clients', format: 'mihomo', enabled: true,
      config: { groups: [], routes: [], default_exit: { kind: 'direct' } },
    }, expect.any(AbortSignal));
    expect(await screen.findByRole('button', { name: 'Add strategy group' })).toBeVisible();
    expect(screen.queryByRole('tab', { name: 'Node selection' })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: /^Exit rules/ })).toHaveTextContent('Final exit DIRECT');
    await user.click(screen.getByRole('button', { name: 'Add strategy group' }));
    expect(screen.getByRole('region', { name: 'Edit strategy group' })).toBeVisible();
    expect(screen.getByRole('button', { name: /^Strategy group 1/ })).toHaveAttribute('aria-current', 'true');
  });
});
