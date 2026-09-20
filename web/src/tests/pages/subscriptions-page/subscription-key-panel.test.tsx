import userEvent from '@testing-library/user-event';
import { beforeEach, expect, it, vi } from 'vitest';
import { act, render, screen, waitFor, within } from '@testing-library/react';

import '@/i18n';
import { toast } from '@/components/ui/toast-manager';
import { ApiClientProvider } from '@/api/api-client-context';
import { SubscriptionsPage } from '@/pages/subscriptions-page/subscriptions-page';
import { createMockApiClient, testSubscriptionTokens } from '@/tests/api/mock-api-client';
import { SubscriptionTokenPanel } from '@/pages/subscriptions-page/subscription-token-panel';

const feedback = vi.spyOn(toast, 'add');
beforeEach(() => {
  feedback.mockClear();
  window.history.replaceState(null, '', '/subscriptions');
});

it('shows the three accepted tabs without fetching users', () => {
  const client = createMockApiClient();
  render(<ApiClientProvider client={client}><SubscriptionsPage /></ApiClientProvider>);
  expect(screen.getAllByRole('tab').map(tab => tab.textContent)).toEqual(['Sources', 'Key management', 'Channels']);
  expect(client.listSubscriptionUsers).not.toHaveBeenCalled();
});

it('creates a key without a user, with an independent shared quota, then clears its secret', async () => {
  const user = userEvent.setup();
  const client = createMockApiClient();
  render(<ApiClientProvider client={client}><SubscriptionTokenPanel /></ApiClientProvider>);
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
  expect(within(issued).queryByLabelText('Delivery channel')).not.toBeInTheDocument();
  expect(within(issued).queryByRole('button', { name: 'Copy subscription URL' })).not.toBeInTheDocument();
  expect(client.listSubscriptionChannels).not.toHaveBeenCalled();
  await user.click(within(issued).getByRole('button', { name: 'Done' }));
  await waitFor(() => expect(screen.queryByText('one-time-public-token')).not.toBeInTheDocument());
});

it('retains entered values after rejection without a false creation result', async () => {
  const user = userEvent.setup();
  const client = createMockApiClient({ createSubscriptionToken: vi.fn().mockRejectedValue(new Error('write failed')) });
  render(<ApiClientProvider client={client}><SubscriptionTokenPanel /></ApiClientProvider>);
  await user.click(screen.getByRole('button', { name: 'Create key' }));
  const form = screen.getByRole('dialog', { name: 'Create key' });
  await user.type(within(form).getByLabelText('Name'), 'Retained');
  await user.click(within(form).getByRole('button', { name: 'Create key' }));
  await waitFor(() => expect(feedback).toHaveBeenCalledWith(expect.objectContaining({ type: 'error' })));
  expect(within(form).getByLabelText('Name')).toHaveValue('Retained');
  expect(screen.queryByRole('dialog', { name: 'Key created' })).not.toBeInTheDocument();
});

it('shows the key name and quota without a legacy description, inspect or revoke actions', async () => {
  const key = { ...testSubscriptionTokens[0], active: false, download_limit: 2, body_response_count: 2 };
  const client = createMockApiClient({
    listSubscriptionTokens: vi.fn().mockResolvedValue({ items: [key] }),
    getSubscriptionToken: vi.fn().mockResolvedValue(key),
  });
  render(<ApiClientProvider client={client}><SubscriptionTokenPanel /></ApiClientProvider>);
  expect(await screen.findByText('Quota reached')).toBeVisible();
  expect(screen.getByText('2 / 2')).toBeVisible();
  expect(screen.getByRole('cell', { name: key.label })).toHaveTextContent(key.label);
  expect(screen.queryByText('Original user grants retained')).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Inspect' })).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: /revoke/i })).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Rotate' })).toBeEnabled();
  expect(screen.getByRole('button', { name: 'Delete' })).toBeEnabled();
  expect(client.getSubscriptionToken).not.toHaveBeenCalled();
});

it('disables and enables the selected key directly from its row', async () => {
  const user = userEvent.setup();
  let key = testSubscriptionTokens[0];
  const client = createMockApiClient({
    listSubscriptionTokens: vi.fn().mockImplementation(async () => ({ items: [key] })),
    setSubscriptionTokenEnabled: vi.fn().mockImplementation(async (_id, enabled) => {
      key = { ...key, enabled, active: enabled };
      return key;
    }),
  });
  render(<ApiClientProvider client={client}><SubscriptionTokenPanel /></ApiClientProvider>);
  await user.click(await screen.findByRole('button', { name: 'Disable' }));
  expect(client.setSubscriptionTokenEnabled).toHaveBeenCalledWith(key.id, false);
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  const enable = await screen.findByRole('button', { name: 'Enable' });
  await waitFor(() => expect(enable).toBeEnabled());
  await user.click(enable);
  expect(client.setSubscriptionTokenEnabled).toHaveBeenLastCalledWith(key.id, true);
});

it.each([true, false])('rotates an enabled=%s key after confirmation and shows the new secret once', async (enabled) => {
  const user = userEvent.setup();
  const key = { ...testSubscriptionTokens[0], enabled, active: enabled };
  const client = createMockApiClient({
    listSubscriptionTokens: vi.fn().mockResolvedValue({ items: [key] }),
  });
  render(<ApiClientProvider client={client}><SubscriptionTokenPanel /></ApiClientProvider>);
  await user.click(await screen.findByRole('button', { name: 'Rotate' }));
  const confirmation = screen.getByRole('dialog', { name: 'Rotate' });
  expect(confirmation).toHaveTextContent(testSubscriptionTokens[0].label);
  expect(client.rotateSubscriptionToken).not.toHaveBeenCalled();
  await user.click(within(confirmation).getByRole('button', { name: 'Rotate' }));
  expect(client.rotateSubscriptionToken).toHaveBeenCalledWith(testSubscriptionTokens[0].id);
  const issued = await screen.findByRole('dialog', { name: 'Key created' });
  const { token } = await vi.mocked(client.rotateSubscriptionToken).mock.results[0].value;
  expect(within(issued).getByText(token)).toBeVisible();
  await user.click(within(issued).getByRole('button', { name: 'Done' }));
  await waitFor(() => expect(screen.queryByText(token)).not.toBeInTheDocument());
});

it('deletes the selected revoked key, with cancellation and retry after failure', async () => {
  const user = userEvent.setup();
  const key = { ...testSubscriptionTokens[0], id: 'unused', label: 'Unused', revoked_at: '2026-09-01T00:00:00Z', active: false };
  const client = createMockApiClient({
    listSubscriptionTokens: vi.fn().mockResolvedValue({ items: [...testSubscriptionTokens, key] }),
    deleteSubscriptionToken: vi.fn().mockRejectedValueOnce(new Error('delete failed')).mockResolvedValueOnce(undefined),
  });
  render(<ApiClientProvider client={client}><SubscriptionTokenPanel /></ApiClientProvider>);
  const row = (await screen.findByText('Unused')).closest('tr')!;
  expect(within(row).getByRole('button', { name: 'Disable' })).toBeDisabled();
  expect(within(row).getByRole('button', { name: 'Rotate' })).toBeDisabled();
  await user.click(within(row).getByRole('button', { name: 'Delete' }));
  await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Cancel' }));
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
  expect(client.deleteSubscriptionToken).not.toHaveBeenCalled();
  await user.click(within(row).getByRole('button', { name: 'Delete' }));
  const confirmation = screen.getByRole('dialog', { name: 'Delete' });
  expect(confirmation).toHaveTextContent('Unused');
  await user.click(within(confirmation).getByRole('button', { name: 'Delete' }));
  await waitFor(() => expect(feedback).toHaveBeenCalledWith(expect.objectContaining({ title: 'delete failed', type: 'error' })));
  expect(confirmation).toBeVisible();
  await user.click(within(confirmation).getByRole('button', { name: 'Delete' }));
  expect(client.deleteSubscriptionToken).toHaveBeenNthCalledWith(1, key.id);
  expect(client.deleteSubscriptionToken).toHaveBeenNthCalledWith(2, key.id);
  expect(client.revokeSubscriptionToken).not.toHaveBeenCalled();
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
});

it('does not report a stale copy after the one-time secret is closed', async () => {
  const user = userEvent.setup();
  let resolve!: () => void;
  vi.spyOn(navigator.clipboard, 'writeText').mockImplementation(() => new Promise<void>(accept => {
    resolve = accept;
  }));
  const client = createMockApiClient();
  render(<ApiClientProvider client={client}><SubscriptionTokenPanel /></ApiClientProvider>);
  await user.click(screen.getByRole('button', { name: 'Create key' }));
  const form = screen.getByRole('dialog', { name: 'Create key' });
  await user.type(within(form).getByLabelText('Name'), 'Copy');
  await user.click(within(form).getByRole('button', { name: 'Create key' }));
  const issued = await screen.findByRole('dialog', { name: 'Key created' });
  await user.click(within(issued).getByRole('button', { name: 'Copy token' }));
  await user.click(within(issued).getByRole('button', { name: 'Done' }));
  await act(async () => resolve());
  expect(feedback).not.toHaveBeenCalledWith(expect.objectContaining({ type: 'success' }));
});
