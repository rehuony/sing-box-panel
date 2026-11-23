import userEvent from '@testing-library/user-event';
import { toast } from '@shared/components/ui/toast-manager';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';

import '@/i18n';
import type { SubscriptionNode, SubscriptionNodeList } from '@/api/api-client';

import { TestRouter } from '@/tests/test-router';
import { ApiRequestError } from '@/api/api-client';
import { ApiClientProvider } from '@/api/api-client-context';
import { createMockApiClient } from '@/tests/api/mock-api-client';
import { SubscriptionsPage } from '@/pages/subscriptions-page/subscriptions-page';

function inbound(id: string): SubscriptionNode {
  return {
    id, display_name: id, origin_name: `${id}-in`, protocol: 'vless', server: 'edge.example', port: 443, tls: true,
    origin: { kind: 'inbound', node_id: 'node_edge', node_name: 'edge', inbound_tag: `${id}-in` },
    updated_at: '2026-11-19T08:00:00.000000001Z',
  };
}
function sourced(id: string): SubscriptionNode {
  return {
    id, display_name: id, origin_name: id, protocol: 'trojan', server: 'provider.example', port: 443,
    origin: { kind: 'source', source_id: 'source_file', source_name: 'Provider' },
    updated_at: '2026-11-19T08:00:00.000000001Z',
  };
}
const pool = (items: SubscriptionNode[]): SubscriptionNodeList => ({ items, node_publications: [], diagnostics: [] });
const cards = () => within(screen.getByRole('list', { name: 'Nodes' })).getAllByRole('button').map(card => card.getAttribute('aria-label'));

function mount(overrides: Parameters<typeof createMockApiClient>[0] = {}) {
  const client = createMockApiClient(overrides);
  render(<TestRouter initialEntries={['/subscriptions#subscription-nodes']}><ApiClientProvider client={client}><SubscriptionsPage /></ApiClientProvider></TestRouter>);
  return client;
}

/** Moves a card one place right with the keyboard sensor, among count visible cards. */
async function moveRight(name: string, count: number) {
  const user = userEvent.setup();
  const card = screen.getByRole('button', { name });
  card.focus();
  fireEvent.keyDown(card, { key: 'F2', code: 'F2' });
  await waitFor(() => expect(card).toHaveAttribute('data-dragging', 'true'));
  await user.keyboard('[ArrowRight]');
  await waitFor(() => expect(screen.getByText(`Position 2 of ${count}.`)).toBeInTheDocument());
  fireEvent.keyDown(card, { key: 'F2', code: 'F2' });
  await waitFor(() => expect(card).not.toHaveAttribute('data-dragging'));
}

beforeEach(() => {
  window.history.replaceState(null, '', '/subscriptions#subscription-nodes');
  const originalRect = HTMLElement.prototype.getBoundingClientRect;
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
    if (this.matches('[data-slot="pool-node-grid"] > li')) {
      const index = Array.from(this.parentElement!.children).indexOf(this);
      return DOMRect.fromRect({ x: (index % 2) * 330, y: Math.floor(index / 2) * 128, width: 320, height: 116 });
    }
    return originalRect.call(this);
  });
});
afterEach(() => vi.restoreAllMocks());

describe('subscription node pool', () => {
  it('shows the pool in its saved order and saves every pool ID after a drop among filtered cards', async () => {
    const items = [inbound('tokyo'), sourced('hk-01'), inbound('osaka'), sourced('us-01')];
    const byID = new Map(items.map(item => [item.id, item]));
    const client = mount({
      listSubscriptionNodes: vi.fn().mockResolvedValue(pool(items)),
      setSubscriptionNodeOrder: vi.fn(async (ids: string[]) => pool(ids.map(id => byID.get(id)!))),
    });
    await screen.findByRole('button', { name: 'tokyo' });
    expect(cards()).toEqual(['tokyo', 'hk-01', 'osaka', 'us-01']);
    const tokyo = within(screen.getByRole('button', { name: 'tokyo' }));
    expect(tokyo.getByText('edge.example:443')).toBeInTheDocument();
    expect(tokyo.getByText('edge · tokyo-in')).toBeInTheDocument();
    await userEvent.setup().type(screen.getByRole('searchbox', { name: 'Search nodes' }), 'edge');
    await waitFor(() => expect(cards()).toEqual(['tokyo', 'osaka']));
    await moveRight('tokyo', 2);
    // The sources' nodes keep their slots; the request carries the whole pool.
    expect(client.setSubscriptionNodeOrder).toHaveBeenCalledExactlyOnceWith(['osaka', 'hk-01', 'tokyo', 'us-01']);
    await userEvent.setup().clear(screen.getByRole('searchbox', { name: 'Search nodes' }));
    await waitFor(() => expect(cards()).toEqual(['osaka', 'hk-01', 'tokyo', 'us-01']));
  });

  it('reads the pool again when it changed while the order saved', async () => {
    const notice = vi.spyOn(toast, 'add');
    const items = [inbound('tokyo'), inbound('osaka')];
    const client = mount({
      listSubscriptionNodes: vi.fn()
        .mockResolvedValueOnce(pool(items))
        .mockResolvedValue(pool([...items, sourced('new')])),
      setSubscriptionNodeOrder: vi.fn().mockRejectedValue(new ApiRequestError('changed', { status: 409, code: 'conflict' })),
    });
    await screen.findByRole('button', { name: 'tokyo' });
    await moveRight('tokyo', 2);
    await waitFor(() => expect(cards()).toEqual(['tokyo', 'osaka', 'new']));
    expect(client.listSubscriptionNodes).toHaveBeenCalledTimes(2);
    expect(notice).toHaveBeenCalledWith(expect.objectContaining({ type: 'warning', title: expect.stringContaining('read again') }));
  });

  it('renames a node at the version it was read and says when the name is taken', async () => {
    const user = userEvent.setup();
    const tokyo = inbound('tokyo');
    const client = mount({
      listSubscriptionNodes: vi.fn().mockResolvedValue(pool([tokyo])),
      getSubscriptionNode: vi.fn().mockResolvedValue({ ...tokyo, outbound: { type: 'vless', server: 'edge.example' } }),
      updateSubscriptionNode: vi.fn()
        .mockRejectedValueOnce(new ApiRequestError('taken', { status: 409, code: 'display_name_taken' }))
        .mockImplementation(async (_id, input) => ({ ...tokyo, ...input, updated_at: '2026-11-19T09:00:00Z', outbound: {} })),
    });
    await user.click(await screen.findByRole('button', { name: 'tokyo' }));
    const dialog = await screen.findByRole('dialog', { name: 'tokyo' });
    expect(await within(dialog).findByRole('region', { name: 'Outbound' })).toHaveTextContent('"server": "edge.example"');
    expect(within(dialog).getByRole('link', { name: 'edge · tokyo-in' })).toHaveAttribute('href', '/nodes/node_edge');
    expect(within(dialog).getByRole('link', { name: 'Open configuration' })).toHaveAttribute('href', '/nodes/node_edge/configuration?inbound=tokyo-in');
    await user.click(within(dialog).getByRole('button', { name: 'Edit display name' }));
    const name = within(dialog).getByLabelText('Display name');
    await user.clear(name);
    await user.type(name, 'Tokyo 1');
    await user.click(within(dialog).getByRole('button', { name: 'Save' }));
    expect(await within(dialog).findByText('Another node already uses this name')).toBeVisible();
    expect(client.updateSubscriptionNode).toHaveBeenCalledWith('tokyo', { display_name: 'Tokyo 1' }, tokyo.updated_at);
    await user.click(within(dialog).getByRole('button', { name: 'Save' }));
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(screen.getByRole('button', { name: 'Tokyo 1' })).toBeVisible();
  });
});
