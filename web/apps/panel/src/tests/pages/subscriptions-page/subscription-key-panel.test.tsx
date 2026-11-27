import userEvent from '@testing-library/user-event';
import { beforeEach, expect, it, vi } from 'vitest';
import { toast } from '@shared/components/ui/toast-manager';
import { act, render, screen, waitFor, within } from '@testing-library/react';

import '@/i18n';
import { TestRouter } from '@/tests/test-router';
import { ApiClientProvider } from '@/api/api-client-context';
import { SubscriptionsPage } from '@/pages/subscriptions-page/subscriptions-page';
import { createMockApiClient, testSubscriptionTokens } from '@/tests/api/mock-api-client';
import { useSubscriptionTokenPage } from '@/pages/subscriptions-page/use-subscription-token-page';
import { SubscriptionTokenPanel as TokenPanel } from '@/pages/subscriptions-page/subscription-token-panel';

function SubscriptionTokenPanel() {
  const list = useSubscriptionTokenPage(true);
  return <TokenPanel list={list} />;
}

const feedback = vi.spyOn(toast, 'add');

it('jumps between key pages and updates the total after changing page size', async () => {
  const user = userEvent.setup();
  const keys = Array.from({ length: 12 }, (_, index) => ({ ...testSubscriptionTokens[0], id: `key-${index}`, label: `Key ${index + 1}` }));
  const client = createMockApiClient({ listSubscriptionTokens: vi.fn(async (filter) => ({
    items: keys.slice(filter?.offset ?? 0, (filter?.offset ?? 0) + (filter?.limit ?? 10)), total: keys.length,
  })) });
  render(<TestRouter><ApiClientProvider client={client}><SubscriptionTokenPanel /></ApiClientProvider></TestRouter>);
  await screen.findByText('Key 1');
  const input = screen.getByRole('spinbutton', { name: 'Current page' });
  expect(input).toHaveAccessibleDescription('2 pages in total');
  await user.clear(input);
  await user.type(input, '2{Enter}');
  expect(await screen.findByText('Key 11')).toBeVisible();
  expect(screen.queryByText('Key 1')).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Next page' })).toBeDisabled();
  await user.click(screen.getByRole('combobox', { name: 'Items per page' }));
  await user.click(await screen.findByRole('option', { name: '5 per page' }));
  await screen.findByText('Key 1');
  expect(input).toHaveValue(1);
  expect(input).toHaveAccessibleDescription('3 pages in total');
  await user.clear(input);
  await user.type(input, '3{Enter}');
  await screen.findByText('Key 11');
  expect(client.listSubscriptionTokens).toHaveBeenLastCalledWith({ limit: 5, offset: 10 }, expect.any(AbortSignal));
});

beforeEach(() => {
  feedback.mockClear();
  window.history.replaceState(null, '', '/subscriptions');
});

it('opens on rule sets with keys last, without reading keys', () => {
  const client = createMockApiClient();
  render(<TestRouter><ApiClientProvider client={client}><SubscriptionsPage /></ApiClientProvider></TestRouter>);
  expect(screen.getAllByRole('tab').map(tab => tab.textContent)).toEqual(['Rule sets', 'Nodes', 'Channels', 'Key management']);
  expect(screen.getByRole('tab', { name: 'Rule sets' })).toHaveAttribute('aria-selected', 'true');
  expect(client.listSubscriptionTokens).not.toHaveBeenCalled();
});

it('reuses pending and completed list requests across tab switches', async () => {
  const user = userEvent.setup();
  let resolve!: (value: { items: typeof testSubscriptionTokens; total: number }) => void;
  const pending = new Promise<{ items: typeof testSubscriptionTokens; total: number }>(accept => {
    resolve = accept;
  });
  const client = createMockApiClient({ listSubscriptionTokens: vi.fn(() => pending) });
  const view = render(
    <TestRouter><ApiClientProvider client={client}><SubscriptionsPage /></ApiClientProvider></TestRouter>,
  );
  await user.click(screen.getByRole('tab', { name: 'Key management' }));
  const signal = vi.mocked(client.listSubscriptionTokens).mock.calls[0][1]!;
  expect(screen.getByRole('status', { name: 'Loading' })).toBeVisible();
  await user.click(screen.getByRole('tab', { name: 'Rule sets' }));
  expect(signal.aborted).toBe(false);
  await user.click(screen.getByRole('tab', { name: 'Key management' }));
  expect(client.listSubscriptionTokens).toHaveBeenCalledTimes(1);
  await act(async () => resolve({ items: testSubscriptionTokens, total: testSubscriptionTokens.length }));
  await screen.findByText(testSubscriptionTokens[0].label);
  for (let i = 0; i < 3; i++) {
    await user.click(screen.getByRole('tab', { name: 'Rule sets' }));
    await user.click(screen.getByRole('tab', { name: 'Key management' }));
    expect(screen.getByText(testSubscriptionTokens[0].label)).toBeVisible();
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
  }
  expect(client.listSubscriptionTokens).toHaveBeenCalledTimes(1);
  view.unmount();
  expect(client.listSubscriptionTokens).toHaveBeenCalledTimes(1);
});

it('retries a failed list when returning and refreshes after a mutation', async () => {
  const user = userEvent.setup();
  const client = createMockApiClient();
  vi.mocked(client.listSubscriptionTokens).mockRejectedValueOnce(new Error('list failed'));
  render(<TestRouter><ApiClientProvider client={client}><SubscriptionsPage /></ApiClientProvider></TestRouter>);
  await user.click(screen.getByRole('tab', { name: 'Key management' }));
  await waitFor(() => expect(feedback).toHaveBeenCalledWith(expect.objectContaining({ description: 'list failed' })));
  await user.click(screen.getByRole('tab', { name: 'Rule sets' }));
  await user.click(screen.getByRole('tab', { name: 'Key management' }));
  await screen.findByText(testSubscriptionTokens[0].label);
  expect(client.listSubscriptionTokens).toHaveBeenCalledTimes(2);
  const toggle = screen.getByRole('button', { name: 'Disable' });
  await user.click(toggle);
  await waitFor(() => expect(client.listSubscriptionTokens).toHaveBeenCalledTimes(3));
  await user.click(screen.getByRole('tab', { name: 'Rule sets' }));
  await user.click(screen.getByRole('tab', { name: 'Key management' }));
  expect(client.listSubscriptionTokens).toHaveBeenCalledTimes(3);
});

it('creates a key without a user, with an independent shared quota, then clears its secret', async () => {
  const user = userEvent.setup();
  const client = createMockApiClient();
  render(<TestRouter><ApiClientProvider client={client}><SubscriptionTokenPanel /></ApiClientProvider></TestRouter>);
  await user.click(screen.getByRole('button', { name: 'Create key' }));
  const form = screen.getByRole('dialog', { name: 'Create key' });
  expect(within(form).queryByLabelText('User')).not.toBeInTheDocument();
  await user.type(within(form).getByLabelText('Name'), 'Travel');
  await user.type(within(form).getByLabelText('Download limit'), '50');
  await user.click(within(form).getByRole('button', { name: 'Create key' }));
  await waitFor(() => expect(client.createSubscriptionToken).toHaveBeenCalledWith({
    label: 'Travel', expiresAt: undefined, downloadLimit: 50,
  }));
  const issued = await screen.findByRole('dialog', { name: 'Key created' });
  expect(within(issued).getByText('one-time-public-token')).toBeVisible();
  // The new key's name and secret, with nothing else to read.
  expect(within(issued).queryByText('Download limit')).not.toBeInTheDocument();
  expect(within(issued).queryByLabelText('Delivery channel')).not.toBeInTheDocument();
  expect(within(issued).queryByRole('button', { name: 'Copy subscription URL' })).not.toBeInTheDocument();
  expect(client.listSubscriptionChannels).not.toHaveBeenCalled();
  await user.click(within(issued).getByRole('button', { name: 'Done' }));
  await waitFor(() => expect(screen.queryByText('one-time-public-token')).not.toBeInTheDocument());
});

it('retains entered values after rejection without a false creation result', async () => {
  const user = userEvent.setup();
  const client = createMockApiClient({ createSubscriptionToken: vi.fn().mockRejectedValue(new Error('write failed')) });
  render(<TestRouter><ApiClientProvider client={client}><SubscriptionTokenPanel /></ApiClientProvider></TestRouter>);
  await user.click(screen.getByRole('button', { name: 'Create key' }));
  const form = screen.getByRole('dialog', { name: 'Create key' });
  await user.type(within(form).getByLabelText('Name'), 'Retained');
  await user.click(within(form).getByRole('button', { name: 'Create key' }));
  expect(await within(form).findByRole('status')).toHaveTextContent('write failed');
  expect(within(form).getByLabelText('Name')).toHaveValue('Retained');
  expect(screen.queryByRole('dialog', { name: 'Key created' })).not.toBeInTheDocument();
});

it('expires a key after a preset or at a time chosen on its calendar', async () => {
  const user = userEvent.setup();
  const client = createMockApiClient();
  render(<TestRouter><ApiClientProvider client={client}><SubscriptionTokenPanel /></ApiClientProvider></TestRouter>);
  await user.click(screen.getByRole('button', { name: 'Create key' }));
  let form = screen.getByRole('dialog', { name: 'Create key' });
  await user.type(within(form).getByLabelText('Name'), 'Month');
  await user.click(within(form).getByRole('combobox', { name: 'Expires' }));
  await user.click(await screen.findByRole('option', { name: '30 days' }));
  const before = Date.now();
  await user.click(within(form).getByRole('button', { name: 'Create key' }));
  await waitFor(() => expect(client.createSubscriptionToken).toHaveBeenCalledTimes(1));
  const preset = new Date(vi.mocked(client.createSubscriptionToken).mock.calls[0][0].expiresAt!).getTime();
  expect(preset - before).toBeGreaterThanOrEqual(30 * 86_400_000 - 1_000);
  expect(preset - before).toBeLessThan(30 * 86_400_000 + 5_000);
  await user.click(within(await screen.findByRole('dialog', { name: 'Key created' })).getByRole('button', { name: 'Done' }));

  await user.click(screen.getByRole('button', { name: 'Create key' }));
  form = screen.getByRole('dialog', { name: 'Create key' });
  await user.type(within(form).getByLabelText('Name'), 'Chosen');
  await user.click(within(form).getByRole('combobox', { name: 'Expires' }));
  await user.click(await screen.findByRole('option', { name: 'Choose a time…' }));
  const calendar = await screen.findByRole('dialog', { name: 'Choose a date and time' });
  const tomorrow = new Date();
  tomorrow.setDate(tomorrow.getDate() + 1);
  if (tomorrow.getDate() === 1) await user.click(within(calendar).getByRole('button', { name: 'Next month' }));
  await user.click(within(calendar).getByRole('button', { name: tomorrow.toLocaleDateString('en', { dateStyle: 'full' }) }));
  await user.click(within(calendar).getByRole('button', { name: 'Done' }));
  await user.click(within(form).getByRole('button', { name: 'Create key' }));
  const chosen = new Date(tomorrow.getFullYear(), tomorrow.getMonth(), tomorrow.getDate(), 23, 59);
  await waitFor(() => expect(client.createSubscriptionToken).toHaveBeenLastCalledWith({
    label: 'Chosen', expiresAt: chosen.toISOString(), downloadLimit: undefined,
  }));
});

it('shows the key name and quota without a legacy description, inspect or revoke actions', async () => {
  const key = { ...testSubscriptionTokens[0], active: false, download_limit: 2, body_response_count: 2 };
  const client = createMockApiClient({
    listSubscriptionTokens: vi.fn().mockResolvedValue({ items: [key], total: 1 }),
    getSubscriptionToken: vi.fn().mockResolvedValue(key),
  });
  render(<TestRouter><ApiClientProvider client={client}><SubscriptionTokenPanel /></ApiClientProvider></TestRouter>);
  expect(await screen.findByText('Quota reached')).toBeVisible();
  expect(screen.getByText('2 / 2')).toBeVisible();
  expect(screen.getByRole('cell', { name: key.label })).toHaveTextContent(key.label);
  expect(screen.queryByText('Original user grants retained')).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Inspect' })).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: /revoke/i })).not.toBeInTheDocument();
  // Rotating and deleting wait in the row's menu.
  expect(screen.queryByRole('button', { name: 'Rotate' })).not.toBeInTheDocument();
  await userEvent.setup().click(screen.getByRole('button', { name: `More for ${key.label}` }));
  expect(await screen.findByRole('menuitem', { name: 'Rotate' })).toBeVisible();
  expect(screen.getByRole('menuitem', { name: 'Delete' })).toBeVisible();
  expect(client.getSubscriptionToken).not.toHaveBeenCalled();
});

it('disables and enables the selected key directly from its row', async () => {
  const user = userEvent.setup();
  let key = testSubscriptionTokens[0];
  const client = createMockApiClient({
    listSubscriptionTokens: vi.fn().mockImplementation(async () => ({ items: [key], total: 1 })),
    setSubscriptionTokenEnabled: vi.fn().mockImplementation(async (_id, enabled) => {
      key = { ...key, enabled, active: enabled };
      return key;
    }),
  });
  render(<TestRouter><ApiClientProvider client={client}><SubscriptionTokenPanel /></ApiClientProvider></TestRouter>);
  const label = key.label;
  await user.click(await screen.findByRole('button', { name: 'Disable' }));
  expect(client.setSubscriptionTokenEnabled).toHaveBeenCalledWith(key.id, false);
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  // Each toast names what happened to which key.
  await waitFor(() => expect(feedback).toHaveBeenLastCalledWith({ title: `Key “${label}” disabled`, type: 'success' }));
  const enable = await screen.findByRole('button', { name: 'Enable' });
  await waitFor(() => expect(enable).toBeEnabled());
  await user.click(enable);
  expect(client.setSubscriptionTokenEnabled).toHaveBeenLastCalledWith(key.id, true);
  await waitFor(() => expect(feedback).toHaveBeenLastCalledWith({ title: `Key “${label}” enabled`, type: 'success' }));
});

it.each([true, false])('rotates an enabled=%s key after confirmation and shows the new secret once', async (enabled) => {
  const user = userEvent.setup();
  const key = { ...testSubscriptionTokens[0], enabled, active: enabled };
  const client = createMockApiClient({
    listSubscriptionTokens: vi.fn().mockResolvedValue({ items: [key], total: 1 }),
  });
  render(<TestRouter><ApiClientProvider client={client}><SubscriptionTokenPanel /></ApiClientProvider></TestRouter>);
  await user.click(await screen.findByRole('button', { name: `More for ${key.label}` }));
  await user.click(await screen.findByRole('menuitem', { name: 'Rotate' }));
  const confirmation = screen.getByRole('dialog', { name: 'Rotate key' });
  expect(confirmation).toHaveTextContent(testSubscriptionTokens[0].label);
  expect(client.rotateSubscriptionToken).not.toHaveBeenCalled();
  await user.click(within(confirmation).getByRole('button', { name: 'Rotate' }));
  expect(client.rotateSubscriptionToken).toHaveBeenCalledWith(testSubscriptionTokens[0].id);
  const issued = await screen.findByRole('dialog', { name: 'Key rotated' });
  const { token } = await vi.mocked(client.rotateSubscriptionToken).mock.results[0].value;
  expect(within(issued).getByText(token)).toBeVisible();
  await user.click(within(issued).getByRole('button', { name: 'Done' }));
  await waitFor(() => expect(screen.queryByText(token)).not.toBeInTheDocument());
});

it('deletes the selected revoked key, with cancellation and retry after failure', async () => {
  const user = userEvent.setup();
  const key = { ...testSubscriptionTokens[0], id: 'unused', label: 'Unused', revoked_at: '2026-09-01T00:00:00Z', active: false };
  const client = createMockApiClient({
    listSubscriptionTokens: vi.fn().mockResolvedValue({
      items: [...testSubscriptionTokens, key], total: testSubscriptionTokens.length + 1,
    }),
    deleteSubscriptionToken: vi.fn().mockRejectedValueOnce(new Error('delete failed')).mockResolvedValueOnce(undefined),
  });
  render(<TestRouter><ApiClientProvider client={client}><SubscriptionTokenPanel /></ApiClientProvider></TestRouter>);
  const row = (await screen.findByRole('button', { name: 'Unused' })).closest('tr')!;
  // A revoked key can only be deleted.
  expect(within(row).queryByRole('button', { name: 'Disable' })).not.toBeInTheDocument();
  const more = within(row).getByRole('button', { name: 'More for Unused' });
  await user.click(more);
  expect(screen.queryByRole('menuitem', { name: 'Rotate' })).not.toBeInTheDocument();
  await user.click(await screen.findByRole('menuitem', { name: 'Delete' }));
  await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Cancel' }));
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
  expect(client.deleteSubscriptionToken).not.toHaveBeenCalled();
  await user.click(more);
  await user.click(await screen.findByRole('menuitem', { name: 'Delete' }));
  const confirmation = screen.getByRole('dialog', { name: 'Delete key' });
  expect(confirmation).toHaveTextContent('Unused');
  await user.click(within(confirmation).getByRole('button', { name: 'Delete' }));
  await waitFor(() => expect(feedback).toHaveBeenCalledWith({ title: 'The key was not deleted', description: 'delete failed', type: 'error' }));
  expect(confirmation).toBeVisible();
  await user.click(within(confirmation).getByRole('button', { name: 'Delete' }));
  expect(client.deleteSubscriptionToken).toHaveBeenNthCalledWith(1, key.id);
  expect(client.deleteSubscriptionToken).toHaveBeenNthCalledWith(2, key.id);
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
  expect(feedback).toHaveBeenLastCalledWith({ title: 'Key “Unused” deleted', type: 'success' });
});

it('does not report a stale copy after the one-time secret is closed', async () => {
  const user = userEvent.setup();
  let resolve!: () => void;
  vi.spyOn(navigator.clipboard, 'writeText').mockImplementation(() => new Promise<void>(accept => {
    resolve = accept;
  }));
  const client = createMockApiClient();
  render(<TestRouter><ApiClientProvider client={client}><SubscriptionTokenPanel /></ApiClientProvider></TestRouter>);
  await user.click(screen.getByRole('button', { name: 'Create key' }));
  const form = screen.getByRole('dialog', { name: 'Create key' });
  await user.type(within(form).getByLabelText('Name'), 'Copy');
  await user.click(within(form).getByRole('button', { name: 'Create key' }));
  const issued = await screen.findByRole('dialog', { name: 'Key created' });
  await user.click(within(issued).getByRole('button', { name: 'Copy' }));
  await user.click(within(issued).getByRole('button', { name: 'Done' }));
  await act(async () => resolve());
  expect(feedback).not.toHaveBeenCalledWith(expect.objectContaining({ type: 'success' }));
});

it('reveals stored key contents only on request and clears them when closed', async () => {
  const user = userEvent.setup();
  const client = createMockApiClient();
  render(<TestRouter><ApiClientProvider client={client}><SubscriptionTokenPanel /></ApiClientProvider></TestRouter>);
  // The row's title opens the key's details.
  const view = await screen.findByRole('button', { name: testSubscriptionTokens[0].label });
  await waitFor(() => expect(view).toBeEnabled());
  expect(client.getSubscriptionTokenSecret).not.toHaveBeenCalled();
  await user.click(view);
  // Titled by the key's name and state, with no Done to acknowledge.
  const dialog = await screen.findByRole('dialog', { name: new RegExp(`^${testSubscriptionTokens[0].label}`) });
  expect(client.getSubscriptionTokenSecret).toHaveBeenCalledWith(testSubscriptionTokens[0].id);
  expect(within(dialog).getByText('sample-subscription-token')).toBeVisible();
  expect(within(dialog).getByText('Created')).toBeVisible();
  expect(within(dialog).queryByRole('button', { name: 'Done' })).not.toBeInTheDocument();
  await user.click(within(dialog).getByRole('button', { name: 'Close' }));
  expect(screen.queryByText('sample-subscription-token')).not.toBeInTheDocument();
});
