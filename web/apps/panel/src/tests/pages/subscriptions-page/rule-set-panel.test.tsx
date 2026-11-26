import userEvent from '@testing-library/user-event';
import { toast } from '@shared/components/ui/toast-manager';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen, waitFor, within } from '@testing-library/react';

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
  it('lists the picked client\'s rule sets and saves a rule added on the prompt line at the version it was read', async () => {
    const user = userEvent.setup();
    const client = renderRuleSets();
    const streaming = testSubscriptionRuleSets[0];
    const editor = await screen.findByRole('region', { name: streaming.name });
    expect(screen.queryByRole('button', { name: /Direct/ })).not.toBeInTheDocument();

    const feedback = vi.spyOn(toast, 'add');
    const entry = within(editor).getByRole('textbox', { name: 'Add rule' });
    await user.type(entry, 'netflix.com{Enter}');
    expect(feedback).toHaveBeenLastCalledWith(
      expect.objectContaining({ title: 'DOMAIN-SUFFIX,netflix.com is already in this rule set', type: 'warning' }),
    );
    await user.clear(entry);
    await user.type(entry, 'disneyplus.com{Enter}');
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

  it('removes a rule from the keyboard and puts it back from the toast', async () => {
    const user = userEvent.setup();
    renderRuleSets();
    const feedback = vi.spyOn(toast, 'add');
    const editor = await screen.findByRole('region', { name: testSubscriptionRuleSets[0].name });
    const rules = within(editor).getByRole('list', { name: 'Rules' });
    // Each line is one button that edits it; its remove action is not a tab stop.
    const [first, second] = rules.querySelectorAll<HTMLElement>('[data-rule-button]');
    first!.focus();
    await user.keyboard('{ArrowDown}');
    expect(second).toHaveFocus();
    await user.keyboard('{Delete}');
    expect(within(rules).queryByText('streaming')).not.toBeInTheDocument();
    const undo = feedback.mock.lastCall?.[0].actionProps?.onClick as (() => void) | undefined;
    act(() => undo?.());
    expect(within(rules).getByText('streaming')).toBeInTheDocument();
  });

  it('keeps text with a line it cannot read, and saving says why instead of saving', async () => {
    const user = userEvent.setup();
    const client = renderRuleSets();
    const feedback = vi.spyOn(toast, 'add');
    const editor = await screen.findByRole('region', { name: testSubscriptionRuleSets[0].name });
    await user.click(within(editor).getByRole('tab', { name: 'Text' }));
    const text = within(editor).getByRole('textbox', { name: 'Rule text, one rule per line' });
    await user.type(text, '{Control>}{End}{/Control}{Enter}GEOIP,CN');
    await user.click(within(editor).getByRole('button', { name: 'Save changes' }));
    expect(feedback).toHaveBeenLastCalledWith(
      expect.objectContaining({ title: 'Line 3: Unsupported match type GEOIP', type: 'error' }),
    );
    expect(client.updateSubscriptionRuleSet).not.toHaveBeenCalled();
    await user.click(within(editor).getByRole('tab', { name: 'Entries' }));
    expect(text).toHaveFocus();
  });
});
