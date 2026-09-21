import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';

import '@/i18n';
import type { SubscriptionNodeSummary } from '@/api/api-client';

import { toast } from '@/components/ui/toast-manager';
import { SubscriptionNodeGrid } from '@/pages/subscriptions-page/subscription-node-grid';
import { readSourceNodeOrder } from '@/pages/subscriptions-page/subscription-node-order';

function node(id: string, name = id): SubscriptionNodeSummary {
  return {
    id, name, key: id, tag: id, source_id: 'source', source_name: 'Remote', origin: 'source',
    type: 'socks', server: 'proxy.example', port: 1080, hidden: false, available: true,
    tls: false, reality: false, revision: 1, visibility_revision: 0,
  };
}
const order = () => screen.getAllByRole('article').map((card) => card.getAttribute('aria-label'));

beforeEach(() => {
  window.localStorage.clear();
  const originalRect = HTMLElement.prototype.getBoundingClientRect;
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
    if (this.matches('.subscription-node-grid > article')) {
      const index = Array.from(this.parentElement!.children).indexOf(this);
      return DOMRect.fromRect({ x: (index % 2) * 300, y: Math.floor(index / 2) * 200, width: 288, height: 184 });
    }
    return originalRect.call(this);
  });
});
afterEach(() => {
  vi.restoreAllMocks();
  window.localStorage.clear();
});

async function moveRight(name: string, count: number) {
  const user = userEvent.setup();
  const card = screen.getByRole('article', { name });
  await user.click(card);
  fireEvent.keyDown(card, { key: 'F2', code: 'F2' });
  await waitFor(() => expect(card).toHaveAttribute('data-dragging', 'true'));
  await user.keyboard('[ArrowRight]');
  await waitFor(() => expect(screen.getByText(`Position 2 of ${count}.`)).toBeInTheDocument());
  fireEvent.keyDown(card, { key: 'F2', code: 'F2' });
  await waitFor(() => expect(card).not.toHaveAttribute('data-dragging'));
}

describe('source node sorting', () => {
  it('persists per source, survives catalog polling/remounts, and appends new nodes', async () => {
    const onOpen = vi.fn();
    const props = { sourceID: 'source', nodes: [node('A'), node('B')], search: '', onOpen };
    const view = render(<SubscriptionNodeGrid {...props} />);
    await moveRight('A', 2);
    expect(order()).toEqual(['B', 'A']);
    expect(readSourceNodeOrder('source')).toEqual(['B', 'A']);
    expect(readSourceNodeOrder('manual')).toEqual([]);
    expect(onOpen).not.toHaveBeenCalled();
    view.rerender(<SubscriptionNodeGrid {...props} nodes={[node('A'), node('B'), node('C')]} />);
    expect(order()).toEqual(['B', 'A', 'C']);
    view.unmount();
    const restored = render(<SubscriptionNodeGrid {...props} nodes={[node('A'), node('C')]} />);
    expect(order()).toEqual(['A', 'C']);
    restored.unmount();
    render(<SubscriptionNodeGrid {...props} />);
    expect(order()).toEqual(['B', 'A']);
  });

  it('only reorders filtered slots and preserves the positions of other nodes', async () => {
    const props = { sourceID: 'source', nodes: [node('A', 'Edge A'), node('other'), node('B', 'Edge B')], search: 'Edge', onOpen: vi.fn() };
    const view = render(<SubscriptionNodeGrid {...props} />);
    await moveRight('Edge A', 2);
    expect(order()).toEqual(['Edge B', 'Edge A']);
    view.rerender(<SubscriptionNodeGrid {...props} search='' />);
    expect(order()).toEqual(['Edge B', 'other', 'Edge A']);
  });

  it('only changes the current page and cancels keyboard drags without saving', async () => {
    const user = userEvent.setup();
    const nodes = Array.from({ length: 12 }, (_, index) => node(String(index + 1)));
    render(<SubscriptionNodeGrid sourceID='source' nodes={nodes} search='' onOpen={vi.fn()} />);
    await user.click(screen.getByRole('button', { name: 'Next page' }));
    expect(order()).toEqual(['11', '12']);
    const card = screen.getByRole('article', { name: '11' });
    await user.click(card);
    fireEvent.keyDown(card, { key: 'F2', code: 'F2' });
    await waitFor(() => expect(card).toHaveAttribute('data-dragging', 'true'));
    await user.keyboard('[Escape]');
    expect(readSourceNodeOrder('source')).toEqual([]);
    await moveRight('11', 2);
    expect(readSourceNodeOrder('source')).toEqual([...nodes.slice(0, 10).map((item) => item.id), '12', '11']);
    await user.click(screen.getByRole('button', { name: 'Previous page' }));
    expect(order()).toEqual(nodes.slice(0, 10).map((item) => item.id));
  });

  it('keeps controls clickable and excludes them from drag activation', async () => {
    const user = userEvent.setup();
    const onOpen = vi.fn();
    const onVisibility = vi.fn();
    const a = node('A');
    const b = { ...node('B'), hidden: true };
    render(<SubscriptionNodeGrid nodes={[a, b]} search='' onOpen={onOpen} onVisibility={onVisibility} />);
    const details = screen.getByRole('button', { name: 'View A' });
    fireEvent.mouseDown(details, { button: 0, clientX: 250, clientY: 20 });
    fireEvent.mouseMove(document, { clientX: 280, clientY: 20 });
    expect(screen.getByRole('article', { name: 'A' })).not.toHaveAttribute('data-dragging');
    fireEvent.mouseUp(document);
    await user.click(details);
    expect(onOpen).toHaveBeenCalledExactlyOnceWith(a);
    await user.click(screen.getByRole('button', { name: 'Hide A' }));
    expect(onVisibility).toHaveBeenCalledWith(a);
    await user.click(screen.getByRole('button', { name: 'Show B' }));
    expect(onVisibility).toHaveBeenCalledWith(b);
    expect(within(screen.getByRole('article', { name: 'B' })).getAllByRole('button')).toHaveLength(2);
  });

  it('ignores invalid stored order and shows a toast if the new order cannot be persisted', async () => {
    window.localStorage.setItem('sing-box-panel.source-node-order.source', '{invalid');
    expect(readSourceNodeOrder('source')).toEqual([]);
    const notice = vi.spyOn(toast, 'add');
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('quota');
    });
    render(<SubscriptionNodeGrid sourceID='source' nodes={[node('A'), node('B')]} search='' onOpen={vi.fn()} />);
    await moveRight('A', 2);
    expect(order()).toEqual(['B', 'A']);
    expect(notice).toHaveBeenCalledWith({ type: 'error', title: 'The order could not be saved in this browser.' });
  });
});
