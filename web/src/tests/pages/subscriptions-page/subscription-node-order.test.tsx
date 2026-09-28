import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';

import '@/i18n';
import type { ApiClient, SubscriptionNodeCatalog, SubscriptionNodeSummary } from '@/api/api-client';

import { deferred } from '@/tests/deferred';
import { TestRouter } from '@/tests/test-router';
import { toast } from '@/components/ui/toast-manager';
import { ApiClientProvider } from '@/api/api-client-context';
import { createMockApiClient } from '@/tests/api/mock-api-client';
import { SubscriptionsPage } from '@/pages/subscriptions-page/subscriptions-page';
import { SubscriptionNodeGrid } from '@/pages/subscriptions-page/subscription-node-grid';

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
  it.each([false, true])('finishes a drag save across tab navigation and reports failures: %s', async fails => {
    const user = userEvent.setup();
    const notice = vi.spyOn(toast, 'add');
    const nodes = ['A', 'B'].map(id => ({ ...node(id), source_id: 'manual', origin: 'manual' as const }));
    let catalog: SubscriptionNodeCatalog = { nodes, node_orders: {}, diagnostics: [], applied_bundle_id: '' };
    const pending = deferred<Awaited<ReturnType<ApiClient['setSubscriptionNodeOrder']>>>();
    const client = createMockApiClient({
      getSubscriptionNodeCatalog: vi.fn(async () => structuredClone(catalog)),
      setSubscriptionNodeOrder: vi.fn(() => pending.promise),
    });
    render(<TestRouter><ApiClientProvider client={client}><SubscriptionsPage /></ApiClientProvider></TestRouter>);
    const editManual = () => within(screen.getByRole('row', { name: /Manual nodes/ })).getByRole('button', { name: 'Edit' });
    await screen.findByRole('row', { name: /Manual nodes/ });
    await user.click(editManual());
    await screen.findByRole('article', { name: 'A' });
    await moveRight('A', 2);
    expect(client.setSubscriptionNodeOrder).toHaveBeenCalledExactlyOnceWith('manual', ['B', 'A'], 0);
    await user.click(screen.getByRole('tab', { name: 'Channels' }));
    await user.click(screen.getByRole('tab', { name: 'Sources' }));
    expect(editManual()).toBeDisabled();
    await act(async () => {
      if (fails) {
        pending.reject(new Error('offline'));
      } else {
        const saved = { ids: ['B', 'A'], revision: 1 };
        catalog = { ...catalog, nodes: [...nodes].reverse(), node_orders: { manual: saved } };
        pending.resolve(saved);
      }
    });
    await waitFor(() => expect(editManual()).toBeEnabled());
    await user.click(editManual());
    expect(order()).toEqual(fails ? ['A', 'B'] : ['B', 'A']);
    if (fails) {
      expect(notice).toHaveBeenCalledWith({ type: 'error', title: 'The node order could not be saved. The last confirmed order has been restored.' });
    }
  });

  it('previews a save, disables further dragging, and uses server order instead of browser storage', async () => {
    window.localStorage.setItem('sing-box-panel.source-node-order.source', '["B","A"]');
    const pending = deferred<void>();
    const onReorder = vi.fn().mockReturnValue(pending.promise);
    const props = { nodes: [node('A'), node('B')], search: '', onOpen: vi.fn(), onReorder };
    const view = render(<SubscriptionNodeGrid {...props} />);
    expect(order()).toEqual(['A', 'B']);
    await moveRight('A', 2);
    expect(order()).toEqual(['B', 'A']);
    expect(onReorder).toHaveBeenCalledExactlyOnceWith(['B', 'A']);
    expect(screen.getByRole('article', { name: 'B' })).toHaveAttribute('tabindex', '-1');
    fireEvent.keyDown(screen.getByRole('article', { name: 'B' }), { key: 'F2', code: 'F2' });
    expect(onReorder).toHaveBeenCalledOnce();
    view.rerender(<SubscriptionNodeGrid {...props} nodes={[node('B'), node('A'), node('C')]} />);
    await act(() => pending.resolve());
    expect(order()).toEqual(['B', 'A', 'C']);
    expect(screen.getByRole('article', { name: 'B' })).toHaveAttribute('tabindex', '0');
    view.unmount();
    render(<SubscriptionNodeGrid {...props} nodes={[node('B'), node('A'), node('C')]} />);
    expect(order()).toEqual(['B', 'A', 'C']);
  });

  it('only changes the current page and cancels keyboard drags without saving', async () => {
    const user = userEvent.setup();
    const nodes = Array.from({ length: 12 }, (_, index) => node(String(index + 1)));
    const onReorder = vi.fn().mockResolvedValue(undefined);
    render(<SubscriptionNodeGrid nodes={nodes} search='' onOpen={vi.fn()} onReorder={onReorder} />);
    await user.click(screen.getByRole('button', { name: 'Next page' }));
    expect(order()).toEqual(['11', '12']);
    const card = screen.getByRole('article', { name: '11' });
    await user.click(card);
    fireEvent.keyDown(card, { key: 'F2', code: 'F2' });
    await waitFor(() => expect(card).toHaveAttribute('data-dragging', 'true'));
    await user.keyboard('[Escape]');
    expect(onReorder).not.toHaveBeenCalled();
    await moveRight('11', 2);
    expect(onReorder).toHaveBeenCalledExactlyOnceWith([...nodes.slice(0, 10).map((item) => item.id), '12', '11']);
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

  it('restores confirmed order and reports a failed save', async () => {
    const notice = vi.spyOn(toast, 'add');
    const onReorder = vi.fn().mockRejectedValue(new Error('offline'));
    render(<SubscriptionNodeGrid nodes={[node('A'), node('B')]} search='' onOpen={vi.fn()} onReorder={onReorder} />);
    await moveRight('A', 2);
    await waitFor(() => expect(order()).toEqual(['A', 'B']));
    expect(notice).toHaveBeenCalledWith({ type: 'error', title: 'The node order could not be saved. The last confirmed order has been restored.' });
  });
});
