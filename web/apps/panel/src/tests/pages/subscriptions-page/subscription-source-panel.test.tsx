import userEvent from '@testing-library/user-event';
import { toast } from '@shared/components/ui/toast-manager';
import { render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import '@/i18n';
import { TestRouter } from '@/tests/test-router';
import { ApiRequestError } from '@/api/api-client';
import { ApiClientProvider } from '@/api/api-client-context';
import { SubscriptionsPage } from '@/pages/subscriptions-page/subscriptions-page';
import { createMockApiClient, testSubscriptionSources } from '@/tests/api/mock-api-client';

const content = 'vless://id@hk.example:443#HK%2001\ntrojan://secret@us.example:443#US%2001';

beforeEach(() => window.history.replaceState(null, '', '/subscriptions#subscription-nodes'));
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
    render(<TestRouter initialEntries={['/subscriptions#subscription-nodes']}><ApiClientProvider client={client}><SubscriptionsPage /></ApiClientProvider></TestRouter>);
    expect(await screen.findByText(testSubscriptionSources[0].name)).toBeVisible();
    await user.click(screen.getAllByRole('button', { name: 'Add source' })[0]);
    const dialog = await screen.findByRole('dialog', { name: 'Add source' });
    // The source field takes the focus, and content pasted into it makes a file source.
    await waitFor(() => expect(within(dialog).getByRole('textbox', { name: 'Source' })).toHaveFocus());
    await user.paste(content);
    await user.type(within(dialog).getByLabelText('Name'), 'Provider');
    await user.type(within(dialog).getByRole('textbox', { name: 'Include' }), 'HK{Enter}');
    const filter = { include: ['HK'], exclude: [] };
    await waitFor(() => expect(client.previewSubscriptionSource).toHaveBeenLastCalledWith(
      { content, filter, format: 'auto' }, expect.any(AbortSignal),
    ), { timeout: 2_000 });
    expect(await within(dialog).findByText('1 of 2 nodes kept')).toBeVisible();
    // Share links are named by the protocols they hold.
    expect(within(dialog).getByText('vless, trojan')).toBeVisible();
    await user.click(within(dialog).getByRole('button', { name: 'View' }));
    const nodes = await screen.findByRole('list', { name: 'Nodes' });
    expect(within(nodes).getByText('US 01').closest('li')).toHaveAttribute('data-kept', 'false');
    // Escape closes the node list and leaves the dialog open.
    await user.keyboard('{Escape}');
    await waitFor(() => expect(screen.queryByRole('list', { name: 'Nodes' })).not.toBeInTheDocument());
    expect(dialog).toBeInTheDocument();
    const feedback = vi.spyOn(toast, 'add');
    await user.click(within(dialog).getByRole('button', { name: 'Save source' }));
    // A refused save is reported in a toast, the dialog kept open with what was given.
    await waitFor(() => expect(feedback).toHaveBeenLastCalledWith(expect.objectContaining({
      title: 'The source was not saved', description: 'line 1 does not convert', type: 'error',
    })));
    expect(dialog).toBeInTheDocument();
    await user.click(within(dialog).getByRole('button', { name: 'Save source' }));
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(client.createSubscriptionSource).toHaveBeenLastCalledWith({ content, filter, format: 'auto', kind: 'file', name: 'Provider' });
  });

  it('saves a file source\'s settings and new content together at the version it was read', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient();
    render(<TestRouter initialEntries={['/subscriptions#subscription-nodes']}><ApiClientProvider client={client}><SubscriptionsPage /></ApiClientProvider></TestRouter>);
    const source = testSubscriptionSources[0];
    await screen.findByText(source.name);
    await user.click(screen.getByRole('button', { name: 'Edit' }));
    const dialog = await screen.findByRole('dialog', { name: source.name });
    await user.clear(within(dialog).getByLabelText('Name'));
    await user.type(within(dialog).getByLabelText('Name'), 'Renamed');
    await user.click(within(dialog).getByLabelText('Content'));
    await user.paste(content);
    // The pasted text can be read back before it is saved.
    await user.click(within(dialog).getByRole('button', { name: 'View the content' }));
    expect(await screen.findByText('trojan://secret@us.example:443#US%2001')).toBeVisible();
    await user.keyboard('{Escape}');
    // New content is read before it is saved.
    expect(await within(dialog).findByText('0 of 0 nodes kept', {}, { timeout: 2_000 })).toBeVisible();
    await user.click(within(dialog).getByRole('button', { name: 'Save source' }));
    await waitFor(() => expect(client.updateSubscriptionSource).toHaveBeenCalledWith(
      source.id, { content, filter: source.filter, format: 'auto', name: 'Renamed' }, source.updated_at,
    ));
  });

  it('names a new source after its link, says why the link cannot be fetched and saves once a retry reads it', async () => {
    const user = userEvent.setup();
    const url = 'https://sub.example.com/api?token=demo';
    const client = createMockApiClient({
      previewSubscriptionSource: vi.fn()
        .mockRejectedValueOnce(new ApiRequestError('The source could not be fetched (http_status_404).', {
          code: 'subscription_source_fetch_failed', fields: { reason: 'http_status_404' }, status: 502,
        }))
        .mockResolvedValue({ format: 'share-links', items: [{ location: 'line 1', name: 'HK 01', protocol: 'vless', kept: true }] }),
    });
    render(<TestRouter initialEntries={['/subscriptions#subscription-nodes']}><ApiClientProvider client={client}><SubscriptionsPage /></ApiClientProvider></TestRouter>);
    await screen.findByText(testSubscriptionSources[0].name);
    await user.click(screen.getAllByRole('button', { name: 'Add source' })[0]);
    const dialog = await screen.findByRole('dialog', { name: 'Add source' });
    await waitFor(() => expect(within(dialog).getByRole('textbox', { name: 'Source' })).toHaveFocus());
    await user.paste(url);
    expect(within(dialog).getByLabelText('Name')).toHaveAttribute('placeholder', 'example.com');
    expect(await within(dialog).findByText('The link answered HTTP 404', {}, { timeout: 2_000 })).toBeVisible();
    // What cannot be read is not sent to be saved.
    const feedback = vi.spyOn(toast, 'add');
    await user.click(within(dialog).getByRole('button', { name: 'Save source' }));
    expect(feedback).toHaveBeenLastCalledWith(expect.objectContaining({ title: 'The source cannot be read; fix what the nodes line says first', type: 'error' }));
    expect(client.createSubscriptionSource).not.toHaveBeenCalled();
    await user.click(within(dialog).getByRole('button', { name: 'Retry' }));
    expect(await within(dialog).findByText('1 of 1 nodes kept', {}, { timeout: 2_000 })).toBeVisible();
    await user.click(within(dialog).getByRole('button', { name: 'Save source' }));
    await waitFor(() => expect(client.createSubscriptionSource).toHaveBeenCalledWith({
      filter: { exclude: [], include: [] }, format: 'auto', kind: 'url', name: 'example.com', refresh_minutes: 360, url,
    }));
  });

  it('offers the format list when the content\'s format is not recognized', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient({
      previewSubscriptionSource: vi.fn().mockResolvedValue({ items: [], error: { code: 'unrecognized_format' } }),
    });
    render(<TestRouter initialEntries={['/subscriptions#subscription-nodes']}><ApiClientProvider client={client}><SubscriptionsPage /></ApiClientProvider></TestRouter>);
    await screen.findByText(testSubscriptionSources[0].name);
    await user.click(screen.getAllByRole('button', { name: 'Add source' })[0]);
    const dialog = await screen.findByRole('dialog', { name: 'Add source' });
    await waitFor(() => expect(within(dialog).getByRole('textbox', { name: 'Source' })).toHaveFocus());
    await user.paste('not a subscription');
    expect(await within(dialog).findByText('The content’s format is not recognized', {}, { timeout: 2_000 })).toBeVisible();
    await user.click(within(dialog).getByRole('button', { name: 'Choose a format' }));
    expect(await screen.findByRole('option', { name: 'sing-box' })).toBeVisible();
  });

  it('splits typed rules into pills and marks the rule the server refuses', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient({
      previewSubscriptionSource: vi.fn().mockRejectedValue(new ApiRequestError('filter.include[1]: invalid_regular_expression', {
        code: 'subscription_source_invalid', fields: { 'filter.include[1]': 'invalid_regular_expression' }, status: 422,
      })),
    });
    render(<TestRouter initialEntries={['/subscriptions#subscription-nodes']}><ApiClientProvider client={client}><SubscriptionsPage /></ApiClientProvider></TestRouter>);
    await screen.findByText(testSubscriptionSources[0].name);
    await user.click(screen.getByRole('button', { name: 'Edit' }));
    const dialog = await screen.findByRole('dialog', { name: testSubscriptionSources[0].name });
    const include = within(dialog).getByRole('textbox', { name: 'Include' });
    // A pattern keeps its spaces; a space elsewhere ends a rule.
    await user.type(include, 'HK /HK 0[[/ ');
    await waitFor(() => expect(client.previewSubscriptionSource).toHaveBeenLastCalledWith(
      { filter: { include: ['HK', '/HK 0[/'], exclude: [] }, format: 'auto', source_id: testSubscriptionSources[0].id }, expect.any(AbortSignal),
    ), { timeout: 2_000 });
    expect(await within(dialog).findByText('/HK 0[/ in Include is not a valid regular expression')).toBeVisible();
    expect(include).toHaveAttribute('aria-invalid', 'true');
    await user.click(within(dialog).getByRole('button', { name: 'Remove rule /HK 0[/' }));
    expect(include).not.toHaveAttribute('aria-invalid');
  });

  it('deletes a source from its dialog after a confirmation that says its nodes leave the pool and channels', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient();
    render(<TestRouter initialEntries={['/subscriptions#subscription-nodes']}><ApiClientProvider client={client}><SubscriptionsPage /></ApiClientProvider></TestRouter>);
    await screen.findByText(testSubscriptionSources[0].name);
    await user.click(screen.getByRole('button', { name: 'Edit' }));
    await user.click(within(await screen.findByRole('dialog', { name: testSubscriptionSources[0].name })).getByRole('button', { name: 'Delete source' }));
    const dialog = await screen.findByRole('dialog', { name: 'Delete source' });
    expect(dialog).toHaveTextContent('Its 2 nodes leave the pool and every channel.');
    await user.click(within(dialog).getByRole('button', { name: 'Delete' }));
    await waitFor(() => expect(client.deleteSubscriptionSource).toHaveBeenCalledWith(
      testSubscriptionSources[0].id, testSubscriptionSources[0].updated_at,
    ));
  });
});
