import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';

import '@/i18n';

import type { SubscriptionNodeSummary } from '@/api/api-client';

import { toast } from '@/components/ui/toast-manager';
import { ApiClientProvider } from '@/api/api-client-context';
import { SubscriptionNodeGrid } from '@/pages/subscriptions-page/subscription-node-grid';
import { SubscriptionSourcePanel } from '@/pages/subscriptions-page/subscription-source-panel';
import {
  createMockApiClient,
  testSubscriptionSources,
  testTask,
} from '@/tests/api/mock-api-client';

const node: SubscriptionNodeSummary = {
  id: 'node_manual',
  key: 'manual:manual_1',
  name: '香港',
  tag: '香港',
  type: 'socks',
  origin: 'manual',
  source_id: 'manual',
  source_name: '手动节点',
  hidden: false,
  visibility_revision: 0,
  revision: 1,
  available: true,
  server: 'proxy.example',
  port: 1080,
  tls: false,
  reality: false,
};
const remote = {
  ...testSubscriptionSources[0],
  source_kind: 'remote' as const,
  name: 'Global Edge',
  config: { url: 'https://source.example/sub', format: 'auto', refresh_interval_minutes: 60 },
};

function mount(client = createMockApiClient()) {
  render(
    <MemoryRouter>
      <ApiClientProvider client={client}>
        <SubscriptionSourcePanel />
      </ApiClientProvider>
    </MemoryRouter>,
  );
  return client;
}

describe('subscription sources and nodes', () => {
  it('switches protocol-dependent endpoint and SSH authentication controls without losing extensions', async () => {
    const user = userEvent.setup();
    const initial
      = '{"type":"socks","tag":"香港","server":"proxy.example","server_port":1080,"version":"5","future":{"value":9007199254740993}}';
    mount(
      createMockApiClient({
        getSubscriptionNodeCatalog: vi.fn().mockResolvedValue({ nodes: [node], diagnostics: [] }),
        getSubscriptionNode: vi.fn().mockResolvedValue({ ...node, outbound_json: initial }),
      }),
    );
    await user.click(await screen.findByRole('button', { name: 'Manual nodes' }));
    await user.click(screen.getByRole('button', { name: 'View 香港' }));
    const dialog = await screen.findByRole('dialog', { name: '香港' });
    const protocol = await within(dialog).findByRole('combobox', { name: 'Protocol' });
    await user.selectOptions(protocol, 'hysteria2');
    await user.selectOptions(
      within(dialog).getByRole('combobox', { name: 'Server connection' }),
      'range',
    );
    await user.click(within(dialog).getByRole('tab', { name: 'Advanced JSON' }));
    const hopping = within(dialog).getByRole('textbox', { name: 'Advanced JSON' });
    expect((hopping as HTMLTextAreaElement).value).toContain('"server_ports"');
    expect((hopping as HTMLTextAreaElement).value).not.toContain('"server_port":');
    expect((hopping as HTMLTextAreaElement).value).not.toContain('"version":');
    expect((hopping as HTMLTextAreaElement).value).toContain('9007199254740993');
    expect((hopping as HTMLTextAreaElement).value).toContain('"enabled": true');
    await user.click(within(dialog).getByRole('tab', { name: 'Visual editor' }));
    await user.selectOptions(within(dialog).getByRole('combobox', { name: 'Protocol' }), 'ssh');
    await user.selectOptions(
      within(dialog).getByRole('combobox', { name: 'Authentication' }),
      'file',
    );
    await user.click(within(dialog).getByRole('tab', { name: 'Advanced JSON' }));
    const ssh = (
      within(dialog).getByRole('textbox', { name: 'Advanced JSON' }) as HTMLTextAreaElement
    ).value;
    expect(ssh).toContain('"private_key_path": ""');
    expect(ssh).not.toContain('"server_ports":');
  });

  it('folds old local imports into the manual collection without changing their identities', async () => {
    const user = userEvent.setup();
    const legacy = {
      ...remote,
      id: 'old_local',
      name: 'Old imports',
      source_kind: 'local' as const,
    };
    const imported = {
      ...node,
      id: 'node_old',
      name: 'Existing node',
      origin: 'source' as const,
      source_id: legacy.id,
    };
    mount(
      createMockApiClient({
        listSubscriptionSources: vi.fn().mockResolvedValue({ items: [legacy, remote] }),
        getSubscriptionNodeCatalog: vi
          .fn()
          .mockResolvedValue({ nodes: [node, imported], diagnostics: [] }),
      }),
    );
    await screen.findByRole('button', { name: 'Global Edge' });
    expect(screen.queryByRole('button', { name: 'Old imports' })).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Manual nodes' }));
    expect(screen.getByRole('button', { name: 'Existing node' })).toBeInTheDocument();
    expect(screen.getAllByRole('article')).toHaveLength(2);
  });

  it('masks protocol keys until the user explicitly reveals node details', async () => {
    const user = userEvent.setup();
    const external = { ...node, origin: 'source' as const, source_id: remote.id };
    mount(
      createMockApiClient({
        listSubscriptionSources: vi.fn().mockResolvedValue({ items: [remote] }),
        getSubscriptionNodeCatalog: vi
          .fn()
          .mockResolvedValue({ nodes: [external], diagnostics: [] }),
        getSubscriptionNode: vi
          .fn()
          .mockResolvedValue({
            ...external,
            outbound_json:
              '{"type":"snell","psk":"private-psk","tls":{"client_key":"private-client-key"}}',
          }),
      }),
    );
    await user.click(await screen.findByRole('button', { name: 'Global Edge' }));
    await user.click(screen.getByRole('button', { name: 'View 香港' }));
    const dialog = await screen.findByRole('dialog', { name: '香港' });
    await within(dialog).findByRole('button', { name: 'Show credentials' });
    expect(dialog).not.toHaveTextContent('private-psk');
    expect(dialog).not.toHaveTextContent('private-client-key');
    await user.click(within(dialog).getByRole('button', { name: 'Show credentials' }));
    expect(dialog).toHaveTextContent('private-psk');
  });

  it('creates URL sources with a minute-based refresh interval and no single-node import', async () => {
    const user = userEvent.setup();
    const client = mount();
    await user.click(screen.getByRole('button', { name: 'Attach source' }));
    const dialog = screen.getByRole('dialog');
    expect(within(dialog).queryByRole('tab')).not.toBeInTheDocument();
    await user.type(within(dialog).getByLabelText('Name'), 'Global Edge');
    await user.type(within(dialog).getByLabelText('Subscription URL'), 'socks://node.example:1080');
    await user.click(within(dialog).getByRole('button', { name: 'Save source' }));
    expect(await within(dialog).findByRole('alert')).toHaveTextContent('HTTP or HTTPS');
    expect(client.createSubscriptionSource).not.toHaveBeenCalled();
    await user.clear(within(dialog).getByLabelText('Subscription URL'));
    await user.type(
      within(dialog).getByLabelText('Subscription URL'),
      'https://source.example/sub',
    );
    await user.selectOptions(within(dialog).getByLabelText('Refresh interval'), '360');
    await user.click(within(dialog).getByRole('button', { name: 'Save source' }));
    await waitFor(() =>
      expect(client.createSubscriptionSource).toHaveBeenCalledWith(
        {
          name: 'Global Edge',
          source_kind: 'remote',
          enabled: true,
          config: {
            url: 'https://source.example/sub',
            format: 'auto',
            refresh_interval_minutes: 360,
          },
        },
        expect.any(AbortSignal),
      ),
    );
  });

  it('shows source detail inline and uses freshly loaded metadata for a CAS save', async () => {
    const user = userEvent.setup();
    const fresh = { ...remote, updated_at: '2026-09-19T00:00:01Z' };
    const client = mount(
      createMockApiClient({
        listSubscriptionSources: vi.fn().mockResolvedValue({ items: [remote] }),
        getSubscriptionSource: vi.fn().mockResolvedValue(fresh),
        updateSubscriptionSource: vi.fn().mockResolvedValue(fresh),
      }),
    );
    await user.click(await screen.findByRole('button', { name: 'Global Edge' }));
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    await user.click(screen.getByRole('tab', { name: 'Source settings' }));
    await user.clear(await screen.findByLabelText('Name'));
    await user.type(screen.getByLabelText('Name'), 'Renamed source');
    await user.click(screen.getByRole('button', { name: 'Save source' }));
    await waitFor(() =>
      expect(client.updateSubscriptionSource).toHaveBeenCalledWith(
        remote.id,
        { name: 'Renamed source', source_kind: 'remote', config: remote.config, enabled: true },
        fresh.updated_at,
        expect.any(AbortSignal),
      ),
    );
  });

  it('keeps hidden cards in their source and reports only real refresh outcomes', async () => {
    const user = userEvent.setup();
    const notified = vi.spyOn(toast, 'add');
    const client = mount(
      createMockApiClient({
        listSubscriptionSources: vi.fn().mockResolvedValue({ items: [remote] }),
        getSubscriptionNodeCatalog: vi.fn().mockResolvedValue({ nodes: [node], diagnostics: [] }),
        setSubscriptionNodeVisibility: vi
          .fn()
          .mockResolvedValue({ ...node, hidden: true, visibility_revision: 1 }),
        refreshSubscriptionSource: vi.fn().mockResolvedValue({ ...testTask, status: 'failed' }),
      }),
    );
    await user.click(await screen.findByRole('button', { name: 'Manual nodes' }));
    await user.click(screen.getByRole('button', { name: 'Hide 香港' }));
    expect(await screen.findByText('Hidden')).toBeInTheDocument();
    expect(client.setSubscriptionNodeVisibility).toHaveBeenCalledWith(
      node.id,
      true,
      0,
      expect.any(AbortSignal),
    );
    await user.click(screen.getByRole('button', { name: 'Back' }));
    const row = screen.getByRole('button', { name: 'Global Edge' }).closest('tr')!;
    await user.click(within(row).getByRole('button', { name: 'Refresh' }));
    await waitFor(() =>
      expect(notified).toHaveBeenCalledWith(expect.objectContaining({ type: 'error' })),
    );
    expect(notified).not.toHaveBeenCalledWith(
      expect.objectContaining({ title: 'Sources refreshed' }),
    );
    notified.mockRestore();
  });

  it('saves edited JSON losslessly and does not save just by opening a node', async () => {
    const user = userEvent.setup();
    const original
      = '{"type":"socks","tag":"香港","server":"proxy.example","server_port":1080,"udp_fragment":false,"future":{"large":9007199254740993}}';
    const client = mount(
      createMockApiClient({
        getSubscriptionNodeCatalog: vi.fn().mockResolvedValue({ nodes: [node], diagnostics: [] }),
        getSubscriptionNode: vi.fn().mockResolvedValue({ ...node, outbound_json: original }),
        updateSubscriptionNode: vi
          .fn()
          .mockResolvedValue({ ...node, revision: 2, outbound_json: original }),
      }),
    );
    await user.click(await screen.findByRole('button', { name: 'Manual nodes' }));
    await user.click(screen.getByRole('button', { name: 'View 香港' }));
    const dialog = await screen.findByRole('dialog', { name: '香港' });
    await user.click(await within(dialog).findByRole('tab', { name: 'Advanced JSON' }));
    expect(client.updateSubscriptionNode).not.toHaveBeenCalled();
    const changed = original.replace('proxy.example', 'new.example');
    fireEvent.change(within(dialog).getByRole('textbox', { name: 'Advanced JSON' }), {
      target: { value: changed },
    });
    await user.click(within(dialog).getByRole('button', { name: 'Save node' }));
    await waitFor(() =>
      expect(client.updateSubscriptionNode).toHaveBeenCalledWith(node.id, changed, 1),
    );
  });

  it('removes hidden and unavailable nodes from channel selection before paging', async () => {
    const user = userEvent.setup();
    const nodes = Array.from({ length: 15 }, (_, index) => ({
      ...node,
      id: `node_${index}`,
      name: `Node ${index}`,
      hidden: index === 0,
      available: index !== 1,
    }));
    render(
      <SubscriptionNodeGrid
        nodes={nodes}
        onOpen={vi.fn()}
        onSelect={vi.fn()}
        search=''
        selected={new Set(['node_0', 'node_2'])}
      />,
    );
    expect(screen.queryByRole('button', { name: 'Node 0' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Node 1' })).not.toBeInTheDocument();
    expect(screen.getByRole('checkbox', { name: 'Select Node 2' })).toBeChecked();
    expect(screen.getAllByRole('article')).toHaveLength(10);
    await user.selectOptions(screen.getByRole('combobox', { name: 'Rows per page' }), '5');
    expect(screen.getAllByRole('article')).toHaveLength(5);
    await user.click(screen.getByRole('button', { name: 'Next page' }));
    expect(screen.getByRole('button', { name: 'Node 7' })).toBeInTheDocument();
    await user.selectOptions(screen.getByRole('combobox', { name: 'Rows per page' }), '50');
    expect(screen.getAllByRole('article')).toHaveLength(13);
  });
});
