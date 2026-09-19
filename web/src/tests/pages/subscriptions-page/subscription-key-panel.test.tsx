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

it('distinguishes exhausted quota from expiry and retains legacy access in details', async () => {
  const user = userEvent.setup();
  const key = { ...testSubscriptionTokens[0], active: false, download_limit: 2, body_response_count: 2 };
  const client = createMockApiClient({
    listSubscriptionTokens: vi.fn().mockResolvedValue({ items: [key] }),
    getSubscriptionToken: vi.fn().mockResolvedValue(key),
  });
  render(<ApiClientProvider client={client}><SubscriptionTokenPanel /></ApiClientProvider>);
  expect(await screen.findByText('Quota reached')).toBeVisible();
  expect(screen.getByText('2 / 2')).toBeVisible();
  await user.click(screen.getByRole('button', { name: 'Inspect' }));
  expect(await screen.findByText('Original user grants retained')).toBeVisible();
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
