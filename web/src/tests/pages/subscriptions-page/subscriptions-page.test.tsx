import { describe, expect, it, vi } from 'vitest';
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

  it('keeps a newly created source open when its initial version fails', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient({
      createSubscriptionSourceVersion: vi.fn().mockRejectedValue(new Error('version rejected')),
    });
    render(<ApiClientProvider client={client}><SubscriptionsPage /></ApiClientProvider>);

    const panel = (await screen.findByRole('heading', { name: 'Sources' })).closest('section');
    expect(panel).not.toBeNull();
    await user.click(within(panel!).getByRole('button', { name: 'Attach source' }));
    await user.type(within(panel!).getByLabelText('Name'), 'Imported source');
    fireEvent.change(within(panel!).getByLabelText('New source document'), {
      target: { value: 'socks://proxy.example:1080' },
    });
    const form = within(panel!).getByRole('heading', { name: 'Attach source' }).closest('form');
    expect(form).not.toBeNull();
    await user.click(within(form!).getByRole('button', { name: 'Attach source' }));

    expect(await within(panel!).findByText(/initial version was not saved/i)).toBeInTheDocument();
    expect(within(panel!).getByRole('heading', { name: /Edit Operator additions/i })).toBeInTheDocument();
    expect(within(panel!).getByRole('button', { name: 'Validate and activate version' })).toBeInTheDocument();
    expect(client.createSubscriptionSource).toHaveBeenCalledTimes(1);
    expect(client.createSubscriptionSourceVersion).toHaveBeenCalledTimes(1);
  });
});
