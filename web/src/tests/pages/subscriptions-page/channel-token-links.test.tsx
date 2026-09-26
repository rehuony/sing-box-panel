import { StrictMode } from 'react';
import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import { render, screen, waitFor, within } from '@testing-library/react';

import '@/i18n';
import { TestRouter } from '@/tests/test-router';
import { toast } from '@/components/ui/toast-manager';
import { ApiClientProvider } from '@/api/api-client-context';
import { ChannelLinkDialog } from '@/pages/subscriptions-page/channel-token-links';
import { SubscriptionChannelPanel } from '@/pages/subscriptions-page/subscription-channel-panel';
import { createMockApiClient, testSubscriptionChannels, testSubscriptionTokens } from '@/tests/api/mock-api-client';

const key = testSubscriptionTokens[0];
const channel = testSubscriptionChannels[0];

describe('channel copies and shared links', () => {
  it('duplicates the full stored channel without altering the source', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient({ getSubscriptionChannel: vi.fn().mockResolvedValue(channel) });
    render(
      <ApiClientProvider client={client}><SubscriptionChannelPanel /></ApiClientProvider>, { wrapper: TestRouter },
    );
    const row = (await screen.findByText(channel.name)).closest('tr')!;
    await user.click(within(row).getByRole('button', { name: 'Copy' }));
    await waitFor(() => expect(client.createSubscriptionChannel).toHaveBeenCalledWith({
      name: `${channel.name} copy`, format: channel.format, enabled: channel.enabled,
      public_host: channel.public_host, config: channel.config,
    }, expect.any(AbortSignal)));
    expect(client.updateSubscriptionChannel).not.toHaveBeenCalled();
  });
  it('exports from any active key on demand, including under StrictMode', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient({ getSubscriptionChannel: vi.fn().mockResolvedValue(channel) });
    render(
      <StrictMode>
        <ApiClientProvider client={client}>
          <ChannelLinkDialog channelID={channel.id} onClose={vi.fn()} />
        </ApiClientProvider>
      </StrictMode>,
    );
    await waitFor(() => expect(screen.getByRole('button', { name: 'Copy' })).toBeEnabled());
    expect(client.getSubscriptionTokenSecret).not.toHaveBeenCalled();
    await user.click(screen.getByRole('button', { name: 'Copy' }));
    await waitFor(() => {
      expect(client.getSubscriptionTokenSecret).toHaveBeenCalledWith(key.id, expect.any(AbortSignal));
    });
    const expectedURL = new URL(`sub/sample-subscription-token/${channel.id}`, document.baseURI).toString();
    expect(await navigator.clipboard.readText()).toBe(expectedURL);
    expect(client.previewSubscriptionChannel).not.toHaveBeenCalled();
  });
  it.each(['key', 'channel'])('rechecks %s before accessing or copying a secret', async (changed) => {
    const user = userEvent.setup();
    const feedback = vi.spyOn(toast, 'add');
    const client = createMockApiClient({
      getSubscriptionChannel: vi.fn().mockResolvedValue({ ...channel,
        enabled: changed !== 'channel',
      }),
      getSubscriptionToken: vi.fn().mockResolvedValue({ ...key, active: changed !== 'key' }),
    });
    render(
      <ApiClientProvider client={client}>
        <ChannelLinkDialog channelID={channel.id} onClose={vi.fn()} />
      </ApiClientProvider>,
    );
    await waitFor(() => expect(screen.getByRole('button', { name: 'Copy' })).toBeEnabled());
    await user.click(screen.getByRole('button', { name: 'Copy' }));
    await waitFor(() => expect(feedback).toHaveBeenCalledWith(expect.objectContaining({ type: 'error' })));
    expect(client.getSubscriptionTokenSecret).not.toHaveBeenCalled();
    feedback.mockRestore();
  });
  it('does not export when no active key remains', async () => {
    const client = createMockApiClient({
      listSubscriptionTokens: vi.fn().mockResolvedValue({ items: [{ ...key, active: false }], total: 1 }),
    });
    render(
      <ApiClientProvider client={client}>
        <ChannelLinkDialog channelID={channel.id} onClose={vi.fn()} />
      </ApiClientProvider>,
    );
    expect(await screen.findByText('No active keys. Create or enable a key in Key management.')).toBeVisible();
    expect(screen.getByRole('button', { name: 'Copy' })).toBeDisabled();
    expect(client.getSubscriptionTokenSecret).not.toHaveBeenCalled();
  });

  it('offers active keys from every page without reading their secrets', async () => {
    const user = userEvent.setup();
    const next = { id: 'last-first-page', created_at: key.created_at };
    const second = { ...key, id: 'second-page-key', label: 'Second page' };
    const client = createMockApiClient({
      listSubscriptionTokens: vi.fn()
        .mockResolvedValueOnce({ items: [{ ...key, active: false, label: 'Unavailable' }, key], next, total: 3 })
        .mockResolvedValueOnce({ items: [second], total: 3 }),
      getSubscriptionToken: vi.fn().mockResolvedValue(second),
    });
    render(
      <ApiClientProvider client={client}>
        <ChannelLinkDialog channelID={channel.id} onClose={vi.fn()} />
      </ApiClientProvider>,
    );
    const selector = screen.getByRole('combobox', { name: 'Subscription key' });
    await waitFor(() => expect(selector).toBeEnabled());
    expect(client.listSubscriptionTokens).toHaveBeenLastCalledWith(
      { limit: 100, beforeID: next.id, beforeTime: next.created_at }, expect.any(AbortSignal),
    );
    await user.click(selector);
    expect(await screen.findByRole('option', { name: key.label })).toBeVisible();
    expect(screen.queryByRole('option', { name: 'Unavailable' })).not.toBeInTheDocument();
    await user.click(screen.getByRole('option', { name: 'Second page' }));
    expect(client.getSubscriptionTokenSecret).not.toHaveBeenCalled();
    await user.click(screen.getByRole('button', { name: 'Copy' }));
    await waitFor(() => {
      expect(client.getSubscriptionTokenSecret).toHaveBeenCalledWith(second.id, expect.any(AbortSignal));
    });
  });
});
