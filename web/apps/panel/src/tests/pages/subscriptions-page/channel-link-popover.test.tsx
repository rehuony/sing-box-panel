import { StrictMode } from 'react';
import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import { render, screen, waitFor, within } from '@testing-library/react';

import '@/i18n';
import type { ApiClient } from '@/api/api-client';

import { TestRouter } from '@/tests/test-router';
import { ApiClientProvider } from '@/api/api-client-context';
import { SubscriptionChannelPanel } from '@/pages/subscriptions-page/subscription-channel-panel';
import { createMockApiClient, testSubscriptionChannels, testSubscriptionTokens } from '@/tests/api/mock-api-client';

const key = testSubscriptionTokens[0];
const channel = testSubscriptionChannels[0];

function renderChannels(client = createMockApiClient()) {
  render(
    <StrictMode>
      <ApiClientProvider client={client}><SubscriptionChannelPanel /></ApiClientProvider>
    </StrictMode>,
    { wrapper: TestRouter },
  );
  return client;
}

async function channelRow() {
  return (await screen.findByRole('button', { name: channel.name })).closest('tr')!;
}

describe('channel list', () => {
  it('duplicates the full stored channel from its row menu without altering the source', async () => {
    const user = userEvent.setup();
    const client = renderChannels();
    await user.click(within(await channelRow()).getByRole('button', { name: `More for ${channel.name}` }));
    await user.click(await screen.findByRole('menuitem', { name: 'Duplicate' }));
    await waitFor(() => expect(client.createSubscriptionChannel).toHaveBeenCalledWith({
      name: `${channel.name} copy`, format: channel.format, enabled: channel.enabled, config: channel.config,
    }, expect.any(AbortSignal)));
    expect(client.updateSubscriptionChannel).not.toHaveBeenCalled();
  });

  it('links with any active key from every page, reading only the secret of the key it shows', async () => {
    const user = userEvent.setup();
    const next = { id: 'last-first-page', created_at: key.created_at };
    const second = { ...key, id: 'second-page-key', label: 'Second page' };
    const client = renderChannels(createMockApiClient({
      listSubscriptionTokens: vi.fn<ApiClient['listSubscriptionTokens']>(async filter => filter?.beforeID === next.id
        ? { items: [second], total: 3 }
        : { items: [{ ...key, active: false, label: 'Unavailable' }, key], next, total: 3 }),
    }));
    await user.click(within(await channelRow()).getByRole('button', { name: 'Subscription link' }));
    const picker = await screen.findByRole('combobox', { name: 'Key' });
    const read = client.getSubscriptionTokenSecret;
    await waitFor(() => expect(read).toHaveBeenCalledWith(key.id, expect.any(AbortSignal)));
    await user.click(picker);
    expect(screen.queryByRole('option', { name: /Unavailable/ })).not.toBeInTheDocument();
    await user.click(await screen.findByRole('option', { name: /Second page/ }));
    await waitFor(() => {
      expect(client.getSubscriptionTokenSecret).toHaveBeenLastCalledWith(second.id, expect.any(AbortSignal));
    });
    await user.click(screen.getByRole('button', { name: 'Copy link' }));
    const link = new URL(`sub/sample-subscription-token/${channel.id}`, document.baseURI).href;
    expect(await navigator.clipboard.readText()).toBe(link);
    expect(client.previewSubscriptionChannel).not.toHaveBeenCalled();
  });

  it('makes a key when none is active and links with it at once', async () => {
    const user = userEvent.setup();
    const client = renderChannels(createMockApiClient({
      listSubscriptionTokens: vi.fn().mockResolvedValue({ items: [{ ...key, active: false }], total: 1 }),
    }));
    await user.click(within(await channelRow()).getByRole('button', { name: 'Subscription link' }));
    const panel = await screen.findByRole('dialog');
    await user.click(await within(panel).findByRole('button', { name: 'New key' }));
    await user.click(within(panel).getByRole('button', { name: 'Create key' }));
    expect(within(panel).getByText('Enter a key name')).toBeVisible();
    expect(client.createSubscriptionToken).not.toHaveBeenCalled();
    await user.type(within(panel).getByRole('textbox', { name: 'Name' }), 'laptop{Enter}');
    await waitFor(() => expect(client.createSubscriptionToken).toHaveBeenCalledWith({ label: 'laptop' }));
    // The link of a key made here comes from its creation, not from reading its secret.
    expect(await within(panel).findByTitle(new URL(`sub/one-time-public-token/${channel.id}`, document.baseURI).href)).toBeVisible();
    expect(client.getSubscriptionTokenSecret).not.toHaveBeenCalled();
  });
});
