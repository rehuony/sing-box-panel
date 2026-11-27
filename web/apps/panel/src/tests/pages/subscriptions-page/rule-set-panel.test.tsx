import userEvent from '@testing-library/user-event';
import { toast } from '@shared/components/ui/toast-manager';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';

import '@/i18n';
import { TestRouter } from '@/tests/test-router';
import { ApiRequestError } from '@/api/api-client';
import { ApiClientProvider } from '@/api/api-client-context';
import { SubscriptionsPage } from '@/pages/subscriptions-page/subscriptions-page';
import { createMockApiClient, testSubscriptionRuleSets } from '@/tests/api/mock-api-client';

/** The rail's rule set names, top to bottom. */
const railNames = () => screen.getAllByRole('button', { description: /F2/ }).map(item => item.querySelector('.font-medium')?.textContent);

beforeEach(() => window.history.replaceState(null, '', '/subscriptions'));
afterEach(() => vi.restoreAllMocks());

function renderRuleSets(client = createMockApiClient(), entry = '/subscriptions#subscription-rules') {
  render(
    <TestRouter initialEntries={[entry]}>
      <ApiClientProvider client={client}><SubscriptionsPage /></ApiClientProvider>
    </TestRouter>,
  );
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
      { name: streaming.name, icon: streaming.icon, rules: [...streaming.rules, { kind: 'domain_suffix', value: 'disneyplus.com' }] },
      streaming.updated_at,
    ));

    // A saved set has nothing left to save.
    await waitFor(() => expect(within(screen.getByRole('region', { name: streaming.name })).getByRole('button', { name: 'Save changes' })).toBeDisabled());
    // The picker above the rail switches the list to another client's sets.
    await user.click(screen.getByRole('combobox', { name: 'Client' }));
    await user.click(await screen.findByRole('option', { name: /sing-box/ }));
    expect(await screen.findByRole('region', { name: testSubscriptionRuleSets[1].name })).toBeVisible();
  });

  it('opens on the client a link to the tab names', async () => {
    renderRuleSets(createMockApiClient(), '/subscriptions#subscription-rules/sing-box');
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

  it('creates a rule set with the icon picked in its name field', async () => {
    const user = userEvent.setup();
    const client = renderRuleSets();
    await screen.findByRole('region', { name: testSubscriptionRuleSets[0].name });
    await user.click(screen.getByRole('button', { name: 'New rule set' }));
    const dialog = await screen.findByRole('dialog', { name: 'New rule set' });
    // The icon menu opens on its search, whose first match Enter picks.
    await user.click(within(dialog).getByRole('button', { name: 'Icon: Rules' }));
    expect(await screen.findByRole('textbox', { name: 'Search icons' })).toHaveFocus();
    await user.keyboard('gam{Enter}');
    await user.type(within(dialog).getByRole('textbox', { name: 'Name' }), 'Games{Enter}');

    // The dialog closes as it is submitted and the set joins the end of the rail without reading the list again.
    expect(dialog).not.toBeInTheDocument();
    expect(railNames()).toEqual(['Streaming', 'Games']);
    expect(client.createSubscriptionRuleSet).toHaveBeenCalledExactlyOnceWith({ client: 'mihomo', icon: 'gamepad', name: 'Games', rules: [] });
    expect(await screen.findByRole('region', { name: 'Games' })).toBeVisible();
    expect(client.listSubscriptionRuleSets).toHaveBeenCalledOnce();
  });

  it('takes a set the server refuses back out of the rail and says why', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient();
    client.createSubscriptionRuleSet.mockRejectedValue(new ApiRequestError('Subscription limit exceeded', { status: 422, code: 'subscription_limit_exceeded' }));
    const feedback = vi.spyOn(toast, 'add');
    renderRuleSets(client);
    await screen.findByRole('region', { name: testSubscriptionRuleSets[0].name });
    await user.click(screen.getByRole('button', { name: 'New rule set' }));
    await user.type(within(await screen.findByRole('dialog')).getByRole('textbox', { name: 'Name' }), 'Games{Enter}');

    await waitFor(() => expect(railNames()).toEqual(['Streaming']));
    expect(feedback).toHaveBeenLastCalledWith(expect.objectContaining({ title: 'The rule set “Games” was not created', type: 'error' }));
    expect(screen.getByRole('region', { name: testSubscriptionRuleSets[0].name })).toBeVisible();
  });

  it('moves a rule set with the keyboard and saves the client\'s order', async () => {
    const [streaming] = testSubscriptionRuleSets;
    const news = { ...streaming, id: 'rule_set_news', icon: 'newspaper', name: 'News', rules: [] };
    const client = createMockApiClient();
    client.listSubscriptionRuleSets.mockResolvedValue({ items: [streaming, news] });
    const originalRect = HTMLElement.prototype.getBoundingClientRect;
    // Items stack 52px apart; the copy that follows the pointer starts over the first.
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
      if (this.matches('li') && this.querySelector(':scope > [aria-roledescription="Sortable rule set"]')) {
        const index = Array.from(this.parentElement!.children).indexOf(this);
        return DOMRect.fromRect({ x: 0, y: index * 52, width: 256, height: 52 });
      }
      if (this.classList.contains('cursor-grabbing')) return DOMRect.fromRect({ x: 0, y: 0, width: 256, height: 52 });
      return originalRect.call(this);
    });
    renderRuleSets(client);
    await screen.findByRole('region', { name: streaming.name });
    expect(railNames()).toEqual(['Streaming', 'News']);

    const item = screen.getAllByRole('button', { description: /F2/ })[0]!;
    item.focus();
    fireEvent.keyDown(item, { key: 'F2', code: 'F2' });
    await userEvent.setup().keyboard('[ArrowDown]');
    await waitFor(() => expect(screen.getByText('Position 2 of 2.')).toBeInTheDocument());
    fireEvent.keyDown(item, { key: 'F2', code: 'F2' });

    expect(railNames()).toEqual(['News', 'Streaming']);
    await waitFor(() => expect(client.setSubscriptionRuleSetOrder).toHaveBeenCalledWith('mihomo', ['rule_set_news', 'rule_set_streaming']));
    expect(client.listSubscriptionRuleSets).toHaveBeenCalledOnce();
    // Dropping does not open the set it moved over.
    expect(screen.getByRole('region', { name: streaming.name })).toBeVisible();
  });
});
