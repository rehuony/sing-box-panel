import { describe, expect, it } from 'vitest';
import userEvent from '@testing-library/user-event';
import { fireEvent, render, screen, within } from '@testing-library/react';

import { ApiClientProvider } from '@/api/api-client-context';
import { SubscriptionsPage } from '@/pages/subscriptions-page';
import { createMockApiClient } from '@/tests/api/mock-api-client';

describe('subscriptionsPage', () => {
  it('creates a channel through the strict management contract', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient();
    render(<ApiClientProvider client={client}><SubscriptionsPage /></ApiClientProvider>);

    const panel = (await screen.findByRole('heading', { name: 'Channels' })).closest('section');
    expect(panel).not.toBeNull();
    await user.click(within(panel!).getByRole('button', { name: 'New channel' }));
    await user.type(within(panel!).getByLabelText('Name'), 'Mobile clients');
    await user.selectOptions(within(panel!).getByLabelText('Format'), 'mihomo');
    await user.type(within(panel!).getByLabelText('Excluded tags'), 'private, private, lab');
    await user.click(within(panel!).getByRole('button', { name: 'Save channel' }));

    expect(client.createSubscriptionChannel).toHaveBeenCalledWith({
      name: 'Mobile clients',
      format: 'mihomo',
      config: { exclude_tags: ['private', 'lab'], exclude_types: [] },
      enabled: true,
    });
  });

  it('updates a source snapshot separately from metadata', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient();
    render(<ApiClientProvider client={client}><SubscriptionsPage /></ApiClientProvider>);

    const panel = (await screen.findByRole('heading', { name: 'Sources' })).closest('section');
    expect(panel).not.toBeNull();
    await user.click(within(panel!).getByRole('button', { name: 'Edit' }));
    const snapshot = within(panel!).getByLabelText('Candidate snapshot JSON');
    fireEvent.change(snapshot, { target: { value: '{"outbounds":[{"tag":"extra"}]}' } });
    await user.click(within(panel!).getByRole('button', { name: 'Save snapshot candidate' }));

    expect(client.updateSubscriptionSourceSnapshot).toHaveBeenCalledWith(
      'source_local',
      { outbounds: [{ tag: 'extra' }] },
      '2026-08-26T07:06:00Z',
    );
    expect(client.updateSubscriptionSource).not.toHaveBeenCalled();
  });
});
