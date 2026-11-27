import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';

import '@/i18n';
import type { ChannelRuleGroup, SubscriptionNode } from '@/api/api-client';

import { newRuleGroup } from '@/pages/subscriptions-page/channel-policy';
import { ChannelNodePicker } from '@/pages/subscriptions-page/channel-node-picker';

function poolNode(id: string, name: string): SubscriptionNode {
  return {
    id, display_name: name, origin_name: name, protocol: 'vless', server: `${id}.example`, port: 443,
    origin: { kind: 'inbound', node_id: 'node_edge', node_name: 'edge', inbound_tag: `${id}-in` },
    updated_at: '2026-11-19T08:00:00Z',
  };
}
// The pool's saved order, which is not alphabetical.
const pool = [poolNode('seattle', 'Seattle'), poolNode('tokyo', 'Tokyo')];

function mount(group: ChannelRuleGroup = newRuleGroup(), format: 'mihomo' | 'sing-box' = 'mihomo', groups = [group]) {
  const onAdd = vi.fn();
  const onClose = vi.fn();
  render(
    <ChannelNodePicker nodes={pool} group={group} groups={groups} format={format} onAdd={onAdd} onClose={onClose} />,
  );
  return { onAdd, onClose, dialog: screen.getByRole('dialog', { name: 'Add nodes' }) };
}
const cardNames = () => within(screen.getByRole('dialog')).getAllByRole('checkbox').map(card => card.getAttribute('aria-label'));
afterEach(() => vi.restoreAllMocks());

describe('channel node picker', () => {
  it('lists the built-ins, the other groups, then pool nodes in pool order, and adds the chosen members in that order', async () => {
    const user = userEvent.setup();
    const current = { ...newRuleGroup([{ kind: 'node', id: 'tokyo' }]), id: 'current', name: 'Current' };
    const groups = [
      current,
      { ...newRuleGroup(), id: 'auto', name: 'Auto', type: 'url-test' as const },
      { ...newRuleGroup([{ kind: 'group', id: 'current' }]), id: 'cycle', name: 'Cycle' },
    ];
    const { dialog, onAdd } = mount(current, 'sing-box', groups);
    // sing-box has no REJECT outbound, so the picker leaves it out.
    expect(cardNames()).toEqual(['DIRECT', 'Auto', 'Cycle', 'Seattle', 'Tokyo']);
    expect(within(dialog).getByRole('checkbox', { name: 'Seattle' })).toHaveTextContent('edge · seattle-in');
    // Already a member or making a cycle: listed but not chosen.
    for (const name of ['Tokyo', 'Cycle']) {
      expect(within(dialog).getByRole('checkbox', { name })).toHaveAttribute('aria-disabled', 'true');
    }
    for (const name of ['Auto', 'DIRECT', 'Seattle']) await user.click(within(dialog).getByRole('checkbox', { name }));
    await user.click(within(dialog).getByRole('button', { name: 'Add 3 nodes' }));
    expect(onAdd).toHaveBeenCalledWith([
      { kind: 'builtin', id: 'direct' }, { kind: 'group', id: 'auto' }, { kind: 'node', id: 'seattle' },
    ]);
  });

  it('cancels without applying selections', async () => {
    const user = userEvent.setup();
    const { dialog, onClose, onAdd } = mount();
    await user.click(within(dialog).getByRole('button', { name: 'Select all' }));
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }));
    expect(onClose).toHaveBeenCalledOnce();
    expect(onAdd).not.toHaveBeenCalled();
  });

  it('reorders cards with the keyboard before adding, without closing the dialog on a cancelled drag', async () => {
    const user = userEvent.setup();
    const { dialog, onAdd, onClose } = mount();
    const originalRect = HTMLElement.prototype.getBoundingClientRect;
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
      if (this.matches('[data-slot="channel-candidates"] > [role="checkbox"]')) {
        const index = Array.from(this.parentElement!.children).indexOf(this);
        return DOMRect.fromRect({ x: (index % 2) * 300, y: Math.floor(index / 2) * 200, width: 288, height: 184 });
      }
      // The copy that follows the pointer starts over Seattle, the third card.
      if (this.matches('.cursor-grabbing[aria-hidden="true"]')) return DOMRect.fromRect({ x: 0, y: 200, width: 288, height: 184 });
      return originalRect.call(this);
    });
    const seattle = within(dialog).getByRole('checkbox', { name: 'Seattle' });
    await user.click(seattle);
    fireEvent.keyDown(seattle, { key: 'F2', code: 'F2' });
    await waitFor(() => expect(seattle).toHaveAttribute('data-dragging', 'true'));
    await user.keyboard('[ArrowRight][Escape]');
    expect(onClose).not.toHaveBeenCalled();
    expect(cardNames()[2]).toBe('Seattle');
    fireEvent.keyDown(seattle, { key: 'F2', code: 'F2' });
    await waitFor(() => expect(seattle).toHaveAttribute('data-dragging', 'true'));
    await user.keyboard('[ArrowRight]');
    await waitFor(() => expect(screen.getByText('Position 4 of 4.')).toBeInTheDocument());
    fireEvent.keyDown(seattle, { key: 'F2', code: 'F2' });
    await waitFor(() => expect(cardNames().slice(2)).toEqual(['Tokyo', 'Seattle']));
    await user.click(within(dialog).getByRole('checkbox', { name: 'Tokyo' }));
    await user.click(within(dialog).getByRole('button', { name: 'Add 2 nodes' }));
    expect(onAdd).toHaveBeenCalledWith([{ kind: 'node', id: 'tokyo' }, { kind: 'node', id: 'seattle' }]);
  });
});
