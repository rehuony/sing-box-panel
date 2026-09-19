import { describe, expect, it } from 'vitest';
import userEvent from '@testing-library/user-event';
import { render, screen, within } from '@testing-library/react';

import '@/i18n';
import { ApiClientProvider } from '@/api/api-client-context';
import { createMockApiClient } from '@/tests/api/mock-api-client';
import { SubscriptionsPage } from '@/pages/subscriptions-page/subscriptions-page';
import { buildPublicSubscriptionURL } from '@/pages/subscriptions-page/public-subscription-url';

describe('subscriptionsPage', () => {
  it('builds the public subscription path under the configured document base', () => {
    expect(buildPublicSubscriptionURL(
      'secret/token',
      'channel main',
      'https://panel.example/panel/',
    )).toBe('https://panel.example/panel/sub/secret%2Ftoken/channel%20main');
  });

  it('keeps the current route, query and router history state for section links', async () => {
    const user = userEvent.setup();
    const originalURL = window.location.href;
    const originalState = window.history.state;
    const routerState = { idx: 4, key: 'router-key', usr: { from: 'tests' } };

    try {
      window.history.replaceState(routerState, '', '/panel/subscriptions/?view=active');
      const client = createMockApiClient();
      render(<ApiClientProvider client={client}><SubscriptionsPage /></ApiClientProvider>);

      const channelsTab = screen.getByRole('tab', { name: 'Channels' });
      await user.click(channelsTab);

      expect(window.location.pathname).toBe('/panel/subscriptions/');
      expect(window.location.search).toBe('?view=active');
      expect(window.location.hash).toBe('#subscription-channels');
      expect(window.history.state).toEqual(routerState);
    } finally {
      window.history.replaceState(originalState, '', originalURL);
    }
  });

  it('creates a channel through the strict management contract', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient();
    render(<ApiClientProvider client={client}><SubscriptionsPage /></ApiClientProvider>);

    await user.click(screen.getByRole('tab', { name: 'Channels' }));
    await user.click(screen.getByRole('button', { name: 'Add channel' }));
    const dialog = await screen.findByRole('dialog', { name: 'Add channel' });
    await user.type(within(dialog).getByLabelText('Channel name'), 'Mobile clients');
    await user.selectOptions(within(dialog).getByLabelText('Output client'), 'mihomo');
    await user.click(within(dialog).getByRole('button', { name: 'Add channel' }));
    expect(client.createSubscriptionChannel).toHaveBeenCalledWith({
      name: 'Mobile clients', format: 'mihomo', public_host: '', enabled: true,
      config: { policy: { selection: { ids: [], excluded_ids: [], new_node_policy: 'exclude' },
        organizer: { prefix: '', exclude_names: [], sort: 'none', deduplicate: false, incompatible: 'skip' },
        groups: [], default_exit: { kind: 'direct' } } },
    }, expect.any(AbortSignal));
  });
});
