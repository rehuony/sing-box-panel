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
    await user.type(within(panel!).getByLabelText('Public host'), 'proxy.example');
    await user.selectOptions(within(panel!).getByLabelText('Format'), 'mihomo');
    await user.type(within(panel!).getByLabelText('Excluded tags'), 'private, private, lab');
    await user.click(within(panel!).getByRole('button', { name: 'Save channel' }));

    expect(client.createSubscriptionChannel).toHaveBeenCalledWith({
      name: 'Mobile clients',
      format: 'mihomo',
      public_host: 'proxy.example',
      config: { exclude_tags: ['private', 'lab'], exclude_types: [] },
      enabled: true,
    });
  });

  it('creates a validated source version separately from metadata', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient();
    render(<ApiClientProvider client={client}><SubscriptionsPage /></ApiClientProvider>);

    const panel = (await screen.findByRole('heading', { name: 'Sources' })).closest('section');
    expect(panel).not.toBeNull();
    await user.click(within(panel!).getByRole('button', { name: 'Edit' }));
    const snapshot = within(panel!).getByLabelText('New source document');
    fireEvent.change(snapshot, { target: { value: '{"outbounds":[{"tag":"extra"}]}' } });
    await user.click(within(panel!).getByRole('button', { name: 'Validate and activate version' }));

    expect(client.createSubscriptionSourceVersion).toHaveBeenCalledWith(
      'source_local',
      'auto',
      '{"outbounds":[{"tag":"extra"}]}',
      '2026-08-26T07:06:00Z',
    );
    expect(client.updateSubscriptionSource).not.toHaveBeenCalled();
  });
});
