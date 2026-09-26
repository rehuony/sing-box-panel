import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';

import '@/i18n';
import type { SubscriptionNodeSummary } from '@/api/api-client';

import { newRuleGroup } from '@/pages/subscriptions-page/channel-policy';
import { ChannelNodePicker } from '@/pages/subscriptions-page/channel-node-picker';

const tokyo: SubscriptionNodeSummary = {
  id: 'tokyo', key: 'manual:tokyo', name: 'Tokyo', tag: 'Tokyo', source_id: 'manual', source_name: 'Manual nodes',
  origin: 'manual', type: 'socks', server: 'tokyo.example', port: 1080, tls: false, reality: false,
  available: true, hidden: false, revision: 1, visibility_revision: 0,
};
function mount(group = newRuleGroup([]), format: 'mihomo' | 'sing-box' = 'mihomo') {
  const onAdd = vi.fn();
  const onClose = vi.fn();
  const nodes = [tokyo, { ...tokyo, id: 'seattle', name: 'Seattle' }];
  render(<ChannelNodePicker nodes={nodes} group={group} format={format} onAdd={onAdd} onClose={onClose} />);
  return { onAdd, onClose, dialog: screen.getByRole('dialog', { name: 'Add nodes' }) };
}
const cardNames = () => within(screen.getByRole('dialog')).getAllByRole('checkbox').map(card => card.getAttribute('aria-label'));
afterEach(() => vi.restoreAllMocks());

describe('channel node picker', () => {
  it('selects whole cards with mouse/keyboard, conditionally shows clear/count, and reuses source metadata', async () => {
    const user = userEvent.setup();
    const { dialog, onAdd } = mount();
    expect(within(dialog).queryByRole('button', { name: 'Close' })).not.toBeInTheDocument();
    expect(within(dialog).queryByRole('button', { name: 'Clear selection' })).not.toBeInTheDocument();
    expect(within(dialog).getByRole('button', { name: 'Add 0 nodes' })).toBeDisabled();
    const card = within(dialog).getByRole('checkbox', { name: 'Tokyo Manual nodes' });
    expect(card.querySelector('input')).toBeNull();
    expect(within(card).getByText('Imported node')).toBeInTheDocument();
    expect(within(card).getByText('tokyo.example:1080')).toBeInTheDocument();
    await user.click(within(card).getByText('Tokyo'));
    expect(card).toBeChecked();
    expect(card).toHaveAttribute('data-selected', 'true');
    expect(within(dialog).getByRole('status', { name: '1 selected' })).toHaveTextContent('1');
    expect(within(dialog).getAllByRole('button').map((button) => button.getAttribute('aria-label') ?? button.textContent))
      .toEqual(['Clear selection', 'Select all', 'Cancel', 'Add 1 nodes']);
    await user.keyboard(' ');
    expect(card).not.toBeChecked();
    await user.keyboard('{Enter}');
    expect(card).toBeChecked();
    await user.click(within(dialog).getByRole('button', { name: 'Clear selection' }));
    expect(card).not.toBeChecked();
    expect(within(dialog).queryByRole('status', { name: '1 selected' })).not.toBeInTheDocument();
    expect(within(dialog).getAllByRole('checkbox')).toHaveLength(4);
    expect(onAdd).not.toHaveBeenCalled();
  });
  it('selects filtered cards without losing selections and excludes already-added or unsupported cards', async () => {
    const user = userEvent.setup();
    const { dialog, onAdd } = mount({ ...newRuleGroup(['tokyo']), builtin_nodes: ['direct'] }, 'sing-box');
    await user.click(within(dialog).getByRole('checkbox', { name: 'REJECT' }));
    await user.click(within(dialog).getByRole('checkbox', { name: 'Tokyo Manual nodes' }));
    expect(within(dialog).getByRole('button', { name: 'Add 0 nodes' })).toBeDisabled();
    await user.type(within(dialog).getByRole('textbox'), 'Seattle');
    await user.click(within(dialog).getByRole('button', { name: 'Select all' }));
    await user.clear(within(dialog).getByRole('textbox'));
    expect(within(dialog).getByRole('checkbox', { name: 'Seattle Manual nodes' })).toBeChecked();
    expect(within(dialog).getByRole('checkbox', { name: 'REJECT' })).not.toBeChecked();
    await user.click(within(dialog).getByRole('button', { name: 'Add 1 nodes' }));
    expect(onAdd).toHaveBeenCalledWith(['seattle'], [], ['node:seattle']);
  });
  it('cancels without applying selections', async () => {
    const user = userEvent.setup();
    const { dialog, onClose, onAdd } = mount();
    await user.click(within(dialog).getByRole('button', { name: 'Select all' }));
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }));
    expect(onClose).toHaveBeenCalledOnce();
    expect(onAdd).not.toHaveBeenCalled();
  });
  it('uses DND-KIT to reorder without selecting and cancels drags without closing the dialog', async () => {
    const user = userEvent.setup();
    const { dialog, onAdd, onClose } = mount();
    const originalRect = HTMLElement.prototype.getBoundingClientRect;
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
      if (this.matches('.channel-candidates > [role="checkbox"]')) {
        const index = Array.from(this.parentElement!.children).indexOf(this);
        return DOMRect.fromRect({ x: (index % 2) * 300, y: Math.floor(index / 2) * 200, width: 288, height: 184 });
      }
      return originalRect.call(this);
    });
    const direct = within(dialog).getByRole('checkbox', { name: 'DIRECT' });
    await user.click(direct);
    fireEvent.keyDown(direct, { key: 'F2', code: 'F2' });
    await waitFor(() => expect(direct).toHaveAttribute('data-dragging', 'true'));
    await user.keyboard('[ArrowRight][Escape]');
    expect(onClose).not.toHaveBeenCalled();
    expect(cardNames()[0]).toBe('DIRECT');
    await waitFor(() => expect(direct).toHaveFocus());
    fireEvent.keyDown(direct, { key: 'F2', code: 'F2' });
    await waitFor(() => expect(direct).toHaveAttribute('data-dragging', 'true'));
    await user.keyboard('[ArrowRight]');
    await waitFor(() => expect(screen.getByText('Position 2 of 4.')).toBeInTheDocument());
    fireEvent.keyDown(direct, { key: 'F2', code: 'F2' });
    await waitFor(() => expect(cardNames().slice(0, 2)).toEqual(['REJECT', 'DIRECT']));
    expect(direct).toBeChecked();
    expect(within(dialog).getByRole('checkbox', { name: 'REJECT' })).not.toBeChecked();
    await user.click(within(dialog).getByRole('checkbox', { name: 'Tokyo Manual nodes' }));
    await user.click(within(dialog).getByRole('button', { name: 'Add 2 nodes' }));
    expect(onAdd).toHaveBeenCalledWith(['tokyo'], ['direct'], ['builtin:direct', 'node:tokyo']);
  });
});
