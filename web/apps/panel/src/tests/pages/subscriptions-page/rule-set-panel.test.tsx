import userEvent from '@testing-library/user-event';
import { render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import '@/i18n';
import { TestRouter } from '@/tests/test-router';
import { ApiClientProvider } from '@/api/api-client-context';
import { SubscriptionsPage } from '@/pages/subscriptions-page/subscriptions-page';
import { createMockApiClient, testSubscriptionRuleSets } from '@/tests/api/mock-api-client';

beforeEach(() => window.history.replaceState(null, '', '/subscriptions'));
afterEach(() => vi.restoreAllMocks());

function renderRuleSets(client = createMockApiClient()) {
  render(<TestRouter initialEntries={['/subscriptions#subscription-rules']}><ApiClientProvider client={client}><SubscriptionsPage /></ApiClientProvider></TestRouter>);
  return client;
}

describe('rule sets', () => {
  it('lists the picked client\'s rule sets and saves an added rule at the version it was read', async () => {
    const user = userEvent.setup();
    const client = renderRuleSets();
    const streaming = testSubscriptionRuleSets[0];
    const editor = await screen.findByRole('region', { name: streaming.name });
    expect(screen.queryByRole('button', { name: /Direct/ })).not.toBeInTheDocument();

    await user.click(within(editor).getByRole('button', { name: 'Add rule' }));
    const dialog = await screen.findByRole('dialog', { name: 'Add rule' });
    await user.type(within(dialog).getByLabelText('Match'), 'disneyplus.com');
    await user.click(within(dialog).getByRole('button', { name: 'Apply' }));
    // The server lists the set as saved from now on.
    const saved = { ...streaming, rules: [...streaming.rules, { kind: 'domain_suffix' as const, value: 'disneyplus.com' }], updated_at: '2026-08-26T08:00:00.000000001Z' };
    client.listSubscriptionRuleSets.mockResolvedValue({ items: [saved, testSubscriptionRuleSets[1]] });
    await user.click(within(editor).getByRole('button', { name: 'Save changes' }));

    await waitFor(() => expect(client.updateSubscriptionRuleSet).toHaveBeenCalledWith(
      streaming.id,
      { name: streaming.name, rules: [...streaming.rules, { kind: 'domain_suffix', value: 'disneyplus.com' }] },
      streaming.updated_at,
    ));

    // A saved set has nothing left to save.
    await waitFor(() => expect(within(screen.getByRole('region', { name: streaming.name })).getByRole('button', { name: 'Save changes' })).toBeDisabled());
    // The picker above the rail switches the list to another client's sets.
    await user.click(screen.getByRole('combobox', { name: 'Client' }));
    await user.click(await screen.findByRole('option', { name: /sing-box/ }));
    expect(await screen.findByRole('region', { name: testSubscriptionRuleSets[1].name })).toBeVisible();
  });

  it('keeps text with a line it cannot read and blocks saving until it is fixed', async () => {
    const user = userEvent.setup();
    renderRuleSets();
    const editor = await screen.findByRole('region', { name: testSubscriptionRuleSets[0].name });
    await user.click(within(editor).getByRole('tab', { name: 'Text' }));
    const text = within(editor).getByRole('textbox', { name: 'Rule text, one rule per line' });
    await user.type(text, '{Control>}{End}{/Control}{Enter}GEOIP,CN');
    expect(within(editor).getByRole('status')).toHaveTextContent('Line 3: Unsupported match type GEOIP');
    expect(within(editor).getByRole('button', { name: 'Save changes' })).toBeDisabled();
    await user.click(within(editor).getByRole('tab', { name: 'Entries' }));
    expect(text).toBeInTheDocument();
  });
});
