import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';

import type { SubscriptionNodeSummary } from '@/api/api-client';

import '@/i18n';

import type { TelemetryState } from '@/components/app-shell/use-telemetry';

import { toast } from '@/components/ui/toast-manager';
import * as reviewedSchemas from '@/schemas/generated';
import { ApiClientProvider } from '@/api/api-client-context';
import { TestRouter as MemoryRouter } from '@/tests/test-router';
import { TelemetryContext } from '@/components/app-shell/telemetry-context';
import { SubscriptionNodeGrid } from '@/pages/subscriptions-page/subscription-node-grid';
import { SubscriptionNodeEditor } from '@/pages/subscriptions-page/subscription-node-editor';
import { SubscriptionSourcePanel } from '@/pages/subscriptions-page/subscription-source-panel';
import {
  createMockApiClient,
  testRuntimeHistory,
  testSubscriptionSources,
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
  it('opens node configuration only from the corner details action', async () => {
    const user = userEvent.setup();
    const onOpen = vi.fn();
    render(<SubscriptionNodeGrid nodes={[node]} search='' onOpen={onOpen} />);

    await user.click(screen.getByText('香港', { exact: true }));
    await user.click(screen.getByText('proxy.example:1080'));
    await user.click(screen.getByRole('article'));
    expect(onOpen).not.toHaveBeenCalled();
    expect(screen.queryByRole('button', { name: '香港' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Manual nodes.*socks/ })).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'View 香港' }));
    expect(onOpen).toHaveBeenCalledExactlyOnceWith(node);
    await user.keyboard('{Enter}');
    expect(onOpen).toHaveBeenCalledTimes(2);
  });

  it('formats new and edited node JSON without changing numeric lexemes or incomplete input', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient();
    render(
      <MemoryRouter>
        <ApiClientProvider client={client}>
          <SubscriptionNodeEditor node={null} onClose={vi.fn()} onSaved={vi.fn()} />
        </ApiClientProvider>
      </MemoryRouter>,
    );
    await act(() => vi.dynamicImportSettled());
    await user.click(await screen.findByRole('tab', { name: 'Advanced JSON' }));
    const editor = screen.getByRole('textbox', { name: 'Advanced JSON' });
    expect(editor).toHaveValue('{\n  "type": "socks",\n  "tag": "",\n  "server": "",\n  "server_port": 1080\n}');
    const raw = '{"type":"socks","future":{"counter":900719925474099312345,"threshold":4.2000e+99}}';
    const formatted = '{\n  "type": "socks",\n  "future": {\n    "counter": 900719925474099312345,\n    "threshold": 4.2000e+99\n  }\n}';
    fireEvent.change(editor, { target: { value: raw } });
    expect(editor).toHaveValue(raw);
    fireEvent.blur(editor);
    expect(editor).toHaveValue(formatted);
    fireEvent.change(editor, { target: { value: raw } });
    fireEvent.click(screen.getByRole('button', { name: 'Format' }));
    expect(editor).toHaveValue(formatted);
    await user.click(screen.getByRole('button', { name: 'Copy displayed JSON' }));
    expect(await navigator.clipboard.readText()).toBe(formatted);
    fireEvent.change(editor, { target: { value: '{"future":' } });
    fireEvent.blur(editor);
    expect(editor).toHaveValue('{"future":');
    expect(screen.getByRole('button', { name: 'Format' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Save node' })).toBeDisabled();
    fireEvent.change(editor, { target: { value: raw } });
    await user.click(screen.getByRole('button', { name: 'Save node' }));
    await waitFor(() => expect(client.createSubscriptionNode).toHaveBeenCalledWith(formatted));
  });

  it('switches editor tabs with the keyboard and preserves loaded node JSON', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient({
      getSubscriptionNode: vi.fn().mockResolvedValue({ ...node, outbound_json: '{"type":"socks","future":9007199254740993}' }),
    });
    render(
      <MemoryRouter>
        <ApiClientProvider client={client}>
          <SubscriptionNodeEditor node={node} onClose={vi.fn()} onSaved={vi.fn()} />
        </ApiClientProvider>
      </MemoryRouter>,
    );
    await act(() => vi.dynamicImportSettled());
    await user.click(await screen.findByRole('tab', { name: 'Visual editor' }));
    await user.keyboard('{ArrowRight}');
    expect(screen.getByRole('tab', { name: 'Advanced JSON' })).toHaveFocus();
    await user.keyboard('{Enter}');
    expect(screen.getByRole('tab', { name: 'Advanced JSON', selected: true })).toHaveFocus();
    const panel = screen.getByRole('tabpanel', { name: 'Advanced JSON' });
    expect(within(panel).getByRole('textbox', { name: 'Advanced JSON' })).toHaveValue('{\n  "type": "socks",\n  "future": 9007199254740993\n}');
    await user.keyboard('{ArrowLeft}{Enter}');
    expect(screen.getByRole('tab', { name: 'Visual editor', selected: true })).toHaveFocus();
    expect(screen.queryByRole('textbox', { name: 'Advanced JSON' })).not.toBeInTheDocument();
    await user.keyboard('{ArrowRight}{Enter}');
    expect(screen.getByRole('textbox', { name: 'Advanced JSON' })).toHaveValue('{\n  "type": "socks",\n  "future": 9007199254740993\n}');
    expect(client.updateSubscriptionNode).not.toHaveBeenCalled();
  });

  it('keeps a loading placeholder until the visual editor is ready without flashing JSON', async () => {
    const schema = await reviewedSchemas.loadReviewedSchema('1.14.0');
    let finishLoading!: () => void;
    const pendingSchema = new Promise<typeof schema>((resolve) => {
      finishLoading = () => resolve(schema);
    });
    const loadSchema = vi.spyOn(reviewedSchemas, 'loadReviewedSchema').mockReturnValueOnce(pendingSchema);
    const client = createMockApiClient({
      getSubscriptionNode: vi.fn().mockResolvedValue({
        ...node,
        outbound_json: '{"type":"socks","tag":"香港","server":"proxy.example","server_port":1080}',
      }),
    });
    try {
      render(
        <MemoryRouter>
          <ApiClientProvider client={client}>
            <SubscriptionNodeEditor node={node} onClose={vi.fn()} onSaved={vi.fn()} />
          </ApiClientProvider>
        </MemoryRouter>,
      );
      await waitFor(() => expect(client.getSubscriptionNode).toHaveBeenCalled());
      expect(screen.getByRole('status', { name: 'Loading…' })).toBeInTheDocument();
      expect(screen.queryByRole('textbox', { name: 'Advanced JSON' })).not.toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Save node' })).toBeDisabled();

      await act(async () => {
        finishLoading();
        await pendingSchema;
      });
      expect(await screen.findByRole('tab', { name: 'Visual editor', selected: true })).toBeInTheDocument();
      expect(screen.getByRole('combobox', { name: 'Protocol' })).toBeInTheDocument();
      expect(screen.queryByRole('status', { name: 'Loading…' })).not.toBeInTheDocument();
      expect(screen.queryByRole('textbox', { name: 'Advanced JSON' })).not.toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Save node' })).toBeEnabled();
    } finally {
      loadSchema.mockRestore();
    }
  });

  it('uses the last recorded core start for manual nodes, independently of source updates', async () => {
    const client = mount();
    const row = screen.getByRole('cell', { name: 'Manual nodes' }).closest('tr')!;
    const started = new Intl.DateTimeFormat('en', { dateStyle: 'short', timeStyle: 'short' }).format(new Date(testRuntimeHistory.items[0]!.process_started_at!));
    await waitFor(() => expect(row).toHaveTextContent(started));
    expect(client.getRuntimeHistory).toHaveBeenCalledWith({ state: 'running', limit: 1 }, expect.any(AbortSignal));
    expect(client.getRuntimeStatus).not.toHaveBeenCalled();
  });

  it('prefers a newer live start and retains the recorded start after stopping', async () => {
    const client = createMockApiClient();
    const renderPanel = (startedAt?: string) => (
      <ApiClientProvider client={client}>
        <TelemetryContext value={{
          runtimeStatus: startedAt ? { running: { started_at: startedAt } } : null,
        } as TelemetryState}>
          <SubscriptionSourcePanel />
        </TelemetryContext>
      </ApiClientProvider>
    );
    const { rerender } = render(renderPanel('2026-09-20T00:00:00Z'), { wrapper: MemoryRouter });
    const row = screen.getByRole('cell', { name: 'Manual nodes' }).closest('tr')!;
    const format = (date: string) => new Intl.DateTimeFormat('en', {
      dateStyle: 'short', timeStyle: 'short',
    }).format(new Date(date));
    expect(row).toHaveTextContent(format('2026-09-20T00:00:00Z'));
    await waitFor(() => expect(client.getSubscriptionNodeCatalog).toHaveBeenCalled());
    rerender(renderPanel());
    expect(row).toHaveTextContent(format('2026-09-20T00:00:00Z'));
  });

  it.each([{ items: [] }, { items: [{ ...testRuntimeHistory.items[0], process_started_at: 'invalid' }] }])(
    'leaves an unknown manual update time empty', async ({ items }) => {
      const client = mount(createMockApiClient({
        getRuntimeHistory: vi.fn().mockResolvedValue({ ...testRuntimeHistory, items }),
      }));
      await waitFor(() => expect(client.getSubscriptionNodeCatalog).toHaveBeenCalled());
      const row = screen.getByRole('cell', { name: 'Manual nodes' }).closest('tr')!;
      expect(within(row).getAllByRole('cell')[2]).toHaveTextContent('—');
    });

  it('returns focus after cancel and starts a fresh source form when reopened', async () => {
    const user = userEvent.setup();
    mount();
    const add = screen.getByRole('button', { name: 'Attach source' });
    await user.click(add);
    await user.type(within(screen.getByRole('dialog')).getByLabelText('Name'), 'Unsaved');
    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Cancel' }));
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    await waitFor(() => expect(add).toHaveFocus());
    await user.click(add);
    expect(within(screen.getByRole('dialog')).getByLabelText('Name')).toHaveValue('');
  });

  it('offers editing and refresh without source deletion in the list or settings', async () => {
    const user = userEvent.setup();
    const fresh = { ...remote, updated_at: '2026-09-20T00:00:00Z' };
    const client = mount(createMockApiClient({
      listSubscriptionSources: vi.fn().mockResolvedValue({ items: [remote] }),
      getSubscriptionSource: vi.fn().mockResolvedValue(fresh),
    }));
    const row = (await screen.findByRole('cell', { name: 'Global Edge' })).closest('tr')!;
    const manual = screen.getByRole('cell', { name: 'Manual nodes' }).closest('tr')!;
    expect(within(manual).queryByRole('button', { name: 'Delete source' })).not.toBeInTheDocument();
    expect(within(row).queryByRole('button', { name: 'Delete source' })).not.toBeInTheDocument();
    expect(within(row).getByRole('button', { name: 'Refresh' })).toBeEnabled();
    await user.click(within(row).getByRole('button', { name: 'Edit' }));
    await user.click(screen.getByRole('tab', { name: 'Source settings' }));
    const settings = await screen.findByRole('tabpanel', { name: 'Source settings' });
    expect(within(settings).getByRole('button', { name: 'Save source' })).toBeEnabled();
    expect(screen.queryByRole('button', { name: 'Delete source' })).not.toBeInTheDocument();
    expect(client.deleteSubscriptionSource).not.toHaveBeenCalled();
  });

  it('guards source settings when returning to nodes or the source list', async () => {
    const user = userEvent.setup();
    const client = mount(createMockApiClient({
      listSubscriptionSources: vi.fn().mockResolvedValue({ items: [remote] }),
      getSubscriptionSource: vi.fn().mockResolvedValue(remote),
    }));
    const row = (await screen.findByRole('cell', { name: 'Global Edge' })).closest('tr')!;
    await user.click(within(row).getByRole('button', { name: 'Edit' }));
    await user.click(screen.getByRole('tab', { name: 'Source settings' }));
    await user.type(await screen.findByLabelText('Name'), ' edited');
    await user.click(screen.getByRole('tab', { name: 'Nodes' }));
    await user.click(screen.getByRole('button', { name: 'Keep editing' }));
    expect(screen.getByLabelText('Name')).toHaveValue('Global Edge edited');
    await user.click(screen.getByRole('button', { name: 'Back' }));
    await user.click(screen.getByRole('button', { name: 'Discard changes' }));
    expect(await screen.findByRole('cell', { name: 'Global Edge' })).toBeVisible();
    expect(client.updateSubscriptionSource).not.toHaveBeenCalled();
  });

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
    const sourceRow = (await screen.findByRole('cell', { name: 'Manual nodes' })).closest('tr')!;
    await user.click(within(sourceRow).getByRole('button', { name: 'Edit' }));
    await user.click(screen.getByRole('button', { name: 'View 香港' }));
    const dialog = await screen.findByRole('dialog', { name: '香港' });
    const protocol = await within(dialog).findByRole('combobox', { name: 'Protocol' });
    await user.click(protocol);
    await user.click(await screen.findByRole('option', { name: 'hysteria2' }));
    await user.click(within(dialog).getByRole('combobox', { name: 'Server connection' }));
    await user.click(await screen.findByRole('option', { name: 'Port hopping' }));
    await user.click(within(dialog).getByRole('tab', { name: 'Advanced JSON' }));
    const hopping = within(dialog).getByRole('textbox', { name: 'Advanced JSON' });
    expect((hopping as HTMLTextAreaElement).value).toContain('"server_ports"');
    expect((hopping as HTMLTextAreaElement).value).not.toContain('"server_port":');
    expect((hopping as HTMLTextAreaElement).value).not.toContain('"version":');
    expect((hopping as HTMLTextAreaElement).value).toContain('9007199254740993');
    expect((hopping as HTMLTextAreaElement).value).toContain('"enabled": true');
    await user.click(within(dialog).getByRole('tab', { name: 'Visual editor' }));
    await user.click(within(dialog).getByRole('combobox', { name: 'Protocol' }));
    await user.click(await screen.findByRole('option', { name: 'ssh' }));
    await user.click(within(dialog).getByRole('combobox', { name: 'Authentication' }));
    await user.click(await screen.findByRole('option', { name: 'Private key file' }));
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
    await screen.findByRole('cell', { name: 'Global Edge' });
    expect(screen.queryByRole('cell', { name: 'Old imports' })).not.toBeInTheDocument();
    const sourceRow = (screen.getByRole('cell', { name: 'Manual nodes' })).closest('tr')!;
    await user.click(within(sourceRow).getByRole('button', { name: 'Edit' }));
    expect(screen.getByText('Existing node', { exact: true })).toBeInTheDocument();
    expect(screen.getAllByRole('article')).toHaveLength(2);
  });

  it('displays and copies complete credentials directly from the node details toolbar', async () => {
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
    const sourceRow = (await screen.findByRole('cell', { name: 'Global Edge' })).closest('tr')!;
    await user.click(within(sourceRow).getByRole('button', { name: 'Edit' }));
    await user.click(screen.getByRole('button', { name: 'View 香港' }));
    const dialog = await screen.findByRole('dialog', { name: '香港' });
    await within(dialog).findByText(/private-psk/);
    expect(dialog).toHaveTextContent('private-psk');
    expect(dialog).toHaveTextContent('private-client-key');
    expect(within(dialog).queryByRole('button', { name: 'Show credentials' })).not.toBeInTheDocument();
    expect(within(dialog).queryByRole('button', { name: 'Hide credentials' })).not.toBeInTheDocument();
    await user.click(within(dialog).getByRole('button', { name: 'Copy displayed JSON' }));
    expect(JSON.parse(await navigator.clipboard.readText())).toMatchObject({
      psk: 'private-psk', tls: { client_key: 'private-client-key' },
    });
  });

  it('creates URL sources with a minute-based refresh interval and no single-node import', async () => {
    const addToast = vi.spyOn(toast, 'add');
    const user = userEvent.setup();
    const client = mount();
    await user.click(screen.getByRole('button', { name: 'Attach source' }));
    const dialog = screen.getByRole('dialog');
    expect(within(dialog).queryByRole('tab')).not.toBeInTheDocument();
    expect(within(dialog).queryByRole('checkbox', { name: 'Enable' })).not.toBeInTheDocument();
    await user.type(within(dialog).getByLabelText('Name'), 'Global Edge');
    await user.type(within(dialog).getByLabelText('Subscription URL'), 'socks://node.example:1080');
    await user.click(within(dialog).getByRole('button', { name: 'Save source' }));
    await waitFor(() => expect(addToast).toHaveBeenCalledWith(expect.objectContaining({ type: 'error', title: expect.stringContaining('HTTP or HTTPS') })));
    expect(within(dialog).queryByRole('alert')).not.toBeInTheDocument();
    addToast.mockRestore();
    expect(client.createSubscriptionSource).not.toHaveBeenCalled();
    await user.clear(within(dialog).getByLabelText('Subscription URL'));
    await user.type(
      within(dialog).getByLabelText('Subscription URL'),
      'https://source.example/sub',
    );
    await user.click(within(dialog).getByRole('combobox', { name: 'Refresh interval' }));
    await user.click(await screen.findByRole('option', { name: '360 min' }));
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

  it.each([true, false])('preserves source enablement %s without a form toggle and uses fresh metadata for a CAS save', async (enabled) => {
    const user = userEvent.setup();
    const fresh = { ...remote, enabled, updated_at: '2026-09-19T00:00:01Z' };
    const client = mount(
      createMockApiClient({
        listSubscriptionSources: vi.fn().mockResolvedValue({ items: [remote] }),
        getSubscriptionSource: vi.fn().mockResolvedValue(fresh),
        updateSubscriptionSource: vi.fn().mockResolvedValue(fresh),
      }),
    );
    const sourceRow = (await screen.findByRole('cell', { name: 'Global Edge' })).closest('tr')!;
    await user.click(within(sourceRow).getByRole('button', { name: 'Edit' }));
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    const navigation = screen.getByRole('tablist', { name: 'Source settings' });
    expect(within(navigation).getAllByRole('tab')).toHaveLength(2);
    await user.click(within(navigation).getByRole('tab', { name: 'Nodes' }));
    await user.keyboard('{ArrowRight}');
    expect(within(navigation).getByRole('tab', { name: 'Source settings' })).toHaveFocus();
    await user.keyboard('{Enter}');
    await screen.findByRole('tabpanel', { name: 'Source settings' });
    await user.clear(await screen.findByLabelText('Name'));
    expect(screen.queryByRole('checkbox', { name: 'Enable' })).not.toBeInTheDocument();
    await user.type(screen.getByLabelText('Name'), 'Renamed source');
    await user.click(screen.getByRole('button', { name: 'Save source' }));
    await waitFor(() =>
      expect(client.updateSubscriptionSource).toHaveBeenCalledWith(
        remote.id,
        { name: 'Renamed source', source_kind: 'remote', config: remote.config, enabled },
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
          .mockResolvedValueOnce({ ...node, hidden: true, visibility_revision: 1 })
          .mockResolvedValueOnce({ ...node, hidden: false, visibility_revision: 2 }),
        refreshSubscriptionSource: vi.fn().mockRejectedValue(new Error('Source refresh failed')),
      }),
    );
    const sourceRow = (await screen.findByRole('cell', { name: 'Manual nodes' })).closest('tr')!;
    await user.click(within(sourceRow).getByRole('button', { name: 'Edit' }));
    await user.click(screen.getByRole('button', { name: 'Hide 香港' }));
    expect(await screen.findByText('Hidden')).toBeInTheDocument();
    expect(client.setSubscriptionNodeVisibility).toHaveBeenCalledWith(
      node.id,
      true,
      0,
      expect.any(AbortSignal),
    );
    const maskedBody = screen.getByText('proxy.example:1080').closest('.subscription-node-card__body');
    expect(maskedBody).toHaveAttribute('aria-hidden', 'true');
    expect(screen.queryByRole('button', { name: /Manual nodes.*socks/ })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'View 香港' })).toBeEnabled();
    expect(screen.queryByRole('button', { name: 'Hide 香港' })).not.toBeInTheDocument();
    const reveal = screen.getByRole('button', { name: 'Show 香港' });
    expect(reveal).toHaveTextContent('Hidden');
    expect(within(screen.getByRole('article').querySelector('header')!).getAllByRole('button')).toHaveLength(1);
    await user.click(reveal);
    await waitFor(() => expect(screen.queryByText('Hidden')).not.toBeInTheDocument());
    expect(client.setSubscriptionNodeVisibility).toHaveBeenLastCalledWith(
      node.id,
      false,
      1,
      expect.any(AbortSignal),
    );
    expect(maskedBody).not.toHaveAttribute('aria-hidden');
    expect(screen.queryByRole('button', { name: /Manual nodes.*socks/ })).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Back' }));
    const row = screen.getByRole('cell', { name: 'Global Edge' }).closest('tr')!;
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
    const sourceRow = (await screen.findByRole('cell', { name: 'Manual nodes' })).closest('tr')!;
    await user.click(within(sourceRow).getByRole('button', { name: 'Edit' }));
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
      expect(client.updateSubscriptionNode).toHaveBeenCalledWith(node.id, `{
  "type": "socks",
  "tag": "香港",
  "server": "new.example",
  "server_port": 1080,
  "udp_fragment": false,
  "future": {
    "large": 9007199254740993
  }
}`, 1),
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
    expect(screen.queryByText('Node 0', { exact: true })).not.toBeInTheDocument();
    expect(screen.queryByText('Node 1', { exact: true })).not.toBeInTheDocument();
    expect(screen.getByRole('checkbox', { name: 'Select Node 2' })).toBeChecked();
    expect(screen.getAllByRole('article')).toHaveLength(10);
    await user.click(screen.getByRole('combobox', { name: 'Items per page' }));
    await user.keyboard('[ArrowDown]');
    await user.click(screen.getByRole('option', { name: '5 per page' }));
    expect(screen.getAllByRole('article')).toHaveLength(5);
    await user.click(screen.getByRole('button', { name: 'Next page' }));
    expect(screen.getByText('Node 7', { exact: true })).toBeInTheDocument();
    await user.click(screen.getByRole('combobox', { name: 'Items per page' }));
    await user.keyboard('[ArrowDown]');
    await user.click(await screen.findByRole('option', { name: '50 per page' }));
    expect(screen.getAllByRole('article')).toHaveLength(13);
  });
});
