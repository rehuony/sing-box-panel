import userEvent from '@testing-library/user-event';
import { render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import '@/i18n';
import { TestRouter } from '@/tests/test-router';
import { ApiRequestError } from '@/api/api-client';
import { ApiClientProvider } from '@/api/api-client-context';
import { SubscriptionsPage } from '@/pages/subscriptions-page/subscriptions-page';
import { createMockApiClient, testSubscriptionSources } from '@/tests/api/mock-api-client';

const content = 'vless://id@hk.example:443#HK%2001\ntrojan://secret@us.example:443#US%2001';

beforeEach(() => window.history.replaceState(null, '', '/subscriptions'));
afterEach(() => vi.restoreAllMocks());

describe('subscription sources', () => {
  it('previews pasted content through the filter and shows the server\'s refusal in the dialog', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient({
      previewSubscriptionSource: vi.fn().mockResolvedValue({
        format: 'share-links',
        items: [
          { location: 'line 1', name: 'HK 01', protocol: 'vless', kept: true },
          { location: 'line 2', name: 'US 01', protocol: 'trojan', kept: false },
        ],
      }),
      createSubscriptionSource: vi.fn()
        .mockRejectedValueOnce(new ApiRequestError('line 1 does not convert', { status: 422, code: 'node_not_convertible' }))
        .mockResolvedValue(testSubscriptionSources[0]),
    });
    render(<TestRouter initialEntries={['/subscriptions#subscription-sources']}><ApiClientProvider client={client}><SubscriptionsPage /></ApiClientProvider></TestRouter>);
    expect(await screen.findByRole('cell', { name: testSubscriptionSources[0].name })).toBeVisible();
    await user.click(screen.getByRole('button', { name: 'Add source' }));
    const dialog = await screen.findByRole('dialog', { name: 'Add source' });
    await user.type(within(dialog).getByLabelText('Name'), 'Provider');
    await user.click(within(dialog).getByRole('combobox', { name: 'Kind' }));
    await user.click(await screen.findByRole('option', { name: 'File' }));
    await user.click(within(dialog).getByLabelText('Content'));
    await user.paste(content);
    await user.type(within(dialog).getByRole('textbox', { name: 'Include' }), 'HK');
    const filter = { include: ['HK'], exclude: [] };
    await waitFor(() => expect(client.previewSubscriptionSource).toHaveBeenLastCalledWith(
      { content, filter, format: 'auto' }, expect.any(AbortSignal),
    ), { timeout: 2_000 });
    const preview = within(dialog).getByRole('region', { name: 'Preview' });
    expect(await within(preview).findByText('1 of 2 nodes kept')).toBeVisible();
    expect(within(preview).getByText('US 01').closest('li')).toHaveAttribute('data-kept', 'false');
    await user.click(within(dialog).getByRole('button', { name: 'Save source' }));
    expect(await within(dialog).findByText('line 1 does not convert')).toBeVisible();
    await user.click(within(dialog).getByRole('button', { name: 'Save source' }));
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(client.createSubscriptionSource).toHaveBeenLastCalledWith({ content, filter, format: 'auto', kind: 'file', name: 'Provider' });
  });

  it('deletes a source after a confirmation that says its nodes leave the pool and channels', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient();
    render(<TestRouter initialEntries={['/subscriptions#subscription-sources']}><ApiClientProvider client={client}><SubscriptionsPage /></ApiClientProvider></TestRouter>);
    const row = (await screen.findByRole('cell', { name: testSubscriptionSources[0].name })).closest('tr')!;
    await user.click(within(row).getByRole('button', { name: 'Delete' }));
    const dialog = await screen.findByRole('dialog', { name: 'Delete source' });
    expect(dialog).toHaveTextContent('Its 2 nodes leave the pool and every channel.');
    await user.click(within(dialog).getByRole('button', { name: 'Delete' }));
    await waitFor(() => expect(client.deleteSubscriptionSource).toHaveBeenCalledWith(
      testSubscriptionSources[0].id, testSubscriptionSources[0].updated_at,
    ));
  });
});
