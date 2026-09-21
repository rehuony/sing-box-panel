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
const channel = {
  ...testSubscriptionChannels[0],
  config: { ...testSubscriptionChannels[0].config, export_token_ids: [key.id] },
};

describe('channel copies and shared links', () => {
  it('duplicates the full stored channel without altering the source', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient({ getSubscriptionChannel: vi.fn().mockResolvedValue(channel) });
    render(
      <ApiClientProvider client={client}><SubscriptionChannelPanel /></ApiClientProvider>, { wrapper: TestRouter },
    );
    const row = (await screen.findByText(channel.name)).closest('tr')!;
    expect(within(row).getAllByRole('button').map((button) => button.textContent)).toEqual(['Edit', 'Copy', 'Link', 'Delete channel']);
    await user.click(within(row).getByRole('button', { name: 'Copy' }));
    await waitFor(() => expect(client.createSubscriptionChannel).toHaveBeenCalledWith({
      name: `${channel.name} copy`, format: channel.format, enabled: channel.enabled,
      public_host: channel.public_host, config: channel.config,
    }, expect.any(AbortSignal)));
    expect(client.updateSubscriptionChannel).not.toHaveBeenCalled();
  });
  it('exports from a bound key on demand, including under StrictMode', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient({ getSubscriptionChannel: vi.fn().mockResolvedValue(channel) });
    render(
      <StrictMode>
        <ApiClientProvider client={client}>
          <ChannelLinkDialog channelID={channel.id} tokenIDs={[key.id]} onClose={vi.fn()} />
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
  it.each(['key', 'channel', 'binding'])('rechecks %s before accessing or copying a secret', async (changed) => {
    const user = userEvent.setup();
    const feedback = vi.spyOn(toast, 'add');
    const client = createMockApiClient({
      getSubscriptionChannel: vi.fn().mockResolvedValue({ ...channel,
        enabled: changed !== 'channel', config: changed === 'binding' ? {} : channel.config,
      }),
      getSubscriptionToken: vi.fn().mockResolvedValue({ ...key, active: changed !== 'key' }),
    });
    render(
      <ApiClientProvider client={client}>
        <ChannelLinkDialog channelID={channel.id} tokenIDs={[key.id]} onClose={vi.fn()} />
      </ApiClientProvider>,
    );
    await waitFor(() => expect(screen.getByRole('button', { name: 'Copy' })).toBeEnabled());
    await user.click(screen.getByRole('button', { name: 'Copy' }));
    await waitFor(() => expect(feedback).toHaveBeenCalledWith(expect.objectContaining({ type: 'error' })));
    expect(client.getSubscriptionTokenSecret).not.toHaveBeenCalled();
    feedback.mockRestore();
  });
  it('does not export when no active bound key remains', async () => {
    const client = createMockApiClient({
      listSubscriptionTokens: vi.fn().mockResolvedValue({ items: [{ ...key, active: false }] }),
    });
    render(
      <ApiClientProvider client={client}>
        <ChannelLinkDialog channelID={channel.id} tokenIDs={[key.id]} onClose={vi.fn()} />
      </ApiClientProvider>,
    );
    expect(await screen.findByText('Bind an active key in channel settings and save first.')).toBeVisible();
    expect(screen.getByRole('button', { name: 'Copy' })).toBeDisabled();
    expect(client.getSubscriptionTokenSecret).not.toHaveBeenCalled();
  });
});
