import userEvent from '@testing-library/user-event';
import { beforeAll, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';

import type { ApiClient, SubscriptionNodeSummary } from '@/api/api-client';

import '@/i18n';
import type { TelemetryState } from '@/components/app-shell/use-telemetry';

import * as reviewedSchemas from '@/schemas/generated';
import { ApiClientProvider } from '@/api/api-client-context';
import { TestRouter as MemoryRouter } from '@/tests/test-router';
import { TelemetryContext } from '@/components/app-shell/telemetry-context';
import { SubscriptionNodeGrid } from '@/pages/subscriptions-page/subscription-node-grid';
import { SubscriptionNodeEditor } from '@/pages/subscriptions-page/subscription-node-editor';
import { SubscriptionSourcePanel } from '@/pages/subscriptions-page/subscription-source-panel';
import {
  createMockApiClient,
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

let editorSchema: Awaited<ReturnType<typeof reviewedSchemas.loadReviewedSchema>>;
beforeAll(async () => {
  const version = Object.keys(reviewedSchemas.reviewedSchemaManifest)
    .sort((a, b) => b.localeCompare(a, undefined, { numeric: true }))[0];
  editorSchema = await reviewedSchemas.loadReviewedSchema(version);
});

function mount(client: ApiClient = createMockApiClient()) {
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

  it('preserves incomplete JSON and creates nodes with losslessly formatted JSON', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient();
    render(
      <MemoryRouter>
        <ApiClientProvider client={client}>
          <SubscriptionNodeEditor node={null} onClose={vi.fn()} onSaved={vi.fn()} />
        </ApiClientProvider>
      </MemoryRouter>,
    );
    // Preloading does not settle the schema imports started by the mounted editor.
    await act(() => vi.dynamicImportSettled());
    await user.click(await screen.findByRole('tab', { name: 'Advanced JSON' }));
    const editor = screen.getByRole('textbox', { name: 'Advanced JSON' });
    fireEvent.change(editor, { target: { value: '{"future":' } });
    fireEvent.blur(editor);
    expect(editor).toHaveValue('{"future":');
    expect(screen.getByRole('button', { name: 'Save node' })).toBeDisabled();

    const raw = '{"type":"socks","future":{"counter":900719925474099312345,"threshold":4.2000e+99}}';
    const formatted = '{\n  "type": "socks",\n  "future": {\n    "counter": 900719925474099312345,\n    "threshold": 4.2000e+99\n  }\n}';
    fireEvent.change(editor, { target: { value: raw } });
    fireEvent.click(screen.getByRole('button', { name: 'Format' }));
    expect(editor).toHaveValue(formatted);
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
    let finishLoading!: () => void;
    const pendingSchema = new Promise<typeof editorSchema>((resolve) => {
      finishLoading = () => resolve(editorSchema);
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

  it('wires a new source submission to the create action', async () => {
    const client = mount();
    await userEvent.click(screen.getByRole('button', { name: 'Attach source' }));
    const dialog = screen.getByRole('dialog');
    fireEvent.change(within(dialog).getByLabelText('Name'), { target: { value: 'Remote' } });
    fireEvent.change(within(dialog).getByLabelText('Subscription URL'), { target: { value: 'https://example.com/sub' } });
    await userEvent.click(within(dialog).getByRole('button', { name: 'Save source' }));
    expect(client.createSubscriptionSource).toHaveBeenCalledOnce();
  });
});
