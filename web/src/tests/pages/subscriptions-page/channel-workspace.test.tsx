import { useState } from 'react';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import { render, screen, waitFor, within } from '@testing-library/react';

import '@/i18n';

import type { SubscriptionChannel, SubscriptionNodeSummary } from '@/api/api-client';

import { ApiClientProvider } from '@/api/api-client-context';
import { ChannelWorkspace } from '@/pages/subscriptions-page/channel-workspace';
import { createMockApiClient, testSubscriptionChannels } from '@/tests/api/mock-api-client';

const node: SubscriptionNodeSummary = {
  id: 'node-one',
  key: 'manual:one',
  tag: 'Tokyo',
  name: 'Tokyo',
  origin: 'manual',
  type: 'socks',
  source_id: 'manual',
  source_name: 'Manual nodes',
  available: true,
  hidden: false,
  tls: false,
  reality: false,
  revision: 1,
  visibility_revision: 0,
  server: 'node.example.com',
  port: 1080,
};
const channel: SubscriptionChannel = {
  ...testSubscriptionChannels[0],
  format: 'sing-box',
  config: {
    policy: {
      selection: { ids: ['node-one'], excluded_ids: [], new_node_policy: 'exclude' },
      organizer: {
        prefix: '',
        exclude_names: [],
        sort: 'none',
        deduplicate: false,
        incompatible: 'skip',
      },
      groups: [],
      default_exit: { kind: 'direct' },
    },
  },
};
function mount(
  value = channel,
  nodes = [node, { ...node, id: 'hidden', name: 'Hidden node', hidden: true }],
) {
  const client = createMockApiClient({
    updateSubscriptionChannel: vi
      .fn()
      .mockImplementation(async (_id, input) => ({
        ...value,
        ...input,
        updated_at: '2026-09-19T10:00:00Z',
      })),
  });
  function Workspace() {
    const [saved, setSaved] = useState(value);
    return (
      <ChannelWorkspace channel={saved} nodes={nodes} onBack={vi.fn()} onSaved={setSaved} onRefresh={async () => {}} />
    );
  }
  render(
    <MemoryRouter>
      <ApiClientProvider client={client}><Workspace /></ApiClientProvider>
    </MemoryRouter>,
  );
  return client;
}
describe('channel workspace', () => {
  it.each([true, false])('preserves channel enablement %s without a form toggle', async (enabled) => {
    const user = userEvent.setup();
    const client = mount({ ...channel, enabled });
    await user.click(screen.getByRole('tab', { name: 'Strategy groups' }));
    expect(screen.queryByRole('checkbox', { name: 'Channel enabled' })).not.toBeInTheDocument();
    await user.clear(screen.getByRole('textbox', { name: 'Channel name' }));
    await user.type(screen.getByRole('textbox', { name: 'Channel name' }), 'Renamed');
    await user.click(screen.getByRole('button', { name: /Save changes/ }));
    await waitFor(() => expect(client.updateSubscriptionChannel).toHaveBeenCalledWith(
      channel.id, expect.objectContaining({ name: 'Renamed', enabled }), channel.updated_at, expect.any(AbortSignal),
    ));
  });
  it('does not offer edits that cannot be persisted for legacy Loon output', () => {
    mount({ ...channel, format: 'loon', config: {} });
    expect(screen.getByRole('checkbox', { name: 'Select Tokyo' })).toBeDisabled();
    expect(screen.getByRole('button', { name: /Save changes/ })).toBeDisabled();
  });
  it('excludes hidden cards and persists selection with CAS', async () => {
    const user = userEvent.setup();
    const client = mount();
    await user.click(screen.getByRole('tab', { name: 'Node selection' }));
    expect(screen.queryByText('Hidden node')).not.toBeInTheDocument();
    await user.click(screen.getByRole('checkbox', { name: 'Select Tokyo' }));
    await user.click(screen.getByRole('button', { name: /Save changes/ }));
    await waitFor(() =>
      expect(client.updateSubscriptionChannel).toHaveBeenCalledWith(
        channel.id,
        expect.objectContaining({
          config: {
            policy: expect.objectContaining({
              selection: { ids: [], excluded_ids: ['node-one'], new_node_policy: 'exclude' },
            }),
          },
        }),
        channel.updated_at,
        expect.any(AbortSignal),
      ),
    );
  });
  it('keeps the parent strategy group open while adding a remote reference and blocks an empty format', async () => {
    const user = userEvent.setup();
    mount();
    await user.click(screen.getByRole('tab', { name: 'Strategy groups' }));
    await user.click(screen.getByRole('button', { name: 'Add strategy group' }));
    const parent = screen.getByRole('region', { name: 'Edit strategy group' });
    await user.clear(within(parent).getByLabelText('Group name'));
    await user.type(within(parent).getByLabelText('Group name'), 'Proxy');
    expect(within(parent).getByRole('checkbox', { name: /Tokyo/ })).toBeChecked();
    await user.click(within(parent).getByRole('tab', { name: 'Exit rules' }));
    await user.click(within(parent).getByRole('button', { name: 'Add rule set' }));
    const child = screen.getByRole('dialog', { name: 'Edit rule set' });
    await user.click(within(child).getByRole('button', { name: 'Done' }));
    expect(within(child).getByLabelText('Source format')).toHaveAttribute('aria-invalid', 'true');
    expect(within(child).queryByRole('alert')).not.toBeInTheDocument();
    await user.type(within(child).getByLabelText('Name'), 'Domains');
    await user.type(
      within(child).getByLabelText('Source URL'),
      'https://raw.githubusercontent.com/example/rules/main/list.srs',
    );
    await user.click(within(child).getByRole('button', { name: 'Accelerate GitHub file' }));
    expect(within(child).getByLabelText('Source URL')).toHaveValue(
      'https://gh-proxy.com/https://raw.githubusercontent.com/example/rules/main/list.srs',
    );
    await user.selectOptions(within(child).getByLabelText('Source format'), 'binary');
    await user.click(within(child).getByRole('button', { name: 'Done' }));
    await waitFor(() =>
      expect(screen.queryByRole('dialog', { name: 'Edit rule set' })).not.toBeInTheDocument(),
    );
    expect(screen.getByRole('region', { name: 'Edit strategy group' })).toBeInTheDocument();
    expect(within(parent).getByText('Domains')).toBeInTheDocument();
  });
  it('keeps group drafts when switching and saves selected nodes and exit rules together', async () => {
    const user = userEvent.setup();
    const client = mount({
      ...channel,
      config: { policy: { ...channel.config.policy!, selection: { ids: ['node-one'], excluded_ids: ['node-two'], new_node_policy: 'exclude' } } },
    }, [node, { ...node, id: 'node-two', name: 'Seattle' }]);
    await user.click(screen.getByRole('button', { name: 'Add strategy group' }));
    await user.clear(screen.getByLabelText('Group name'));
    await user.type(screen.getByLabelText('Group name'), 'Proxy');
    await user.click(screen.getByRole('checkbox', { name: /Seattle/ }));
    await user.click(screen.getByRole('tab', { name: 'Exit rules' }));
    await user.selectOptions(screen.getByLabelText('Default exit'), 'node:node-two');
    await user.click(screen.getByRole('button', { name: 'Add rule' }));
    const rule = screen.getByRole('dialog', { name: 'Edit rule' });
    await user.type(within(rule).getByLabelText('Match value'), 'example.com');
    await user.selectOptions(within(rule).getByLabelText('Traffic exit'), 'node:node-one');
    await user.click(within(rule).getByRole('button', { name: 'Done' }));
    await user.click(screen.getByRole('button', { name: 'Add strategy group' }));
    await user.clear(screen.getByLabelText('Group name'));
    await user.type(screen.getByLabelText('Group name'), 'Direct sites');
    const sidebar = screen.getByRole('complementary', { name: 'Strategy groups' });
    await user.click(within(sidebar).getByRole('button', { name: /Proxy/ }));
    expect(screen.getByLabelText('Group name')).toHaveValue('Proxy');
    expect(screen.getByRole('checkbox', { name: /Seattle/ })).toBeChecked();
    await user.click(screen.getByRole('tab', { name: 'Exit rules' }));
    expect(screen.getByLabelText('Default exit')).toHaveValue('node:node-two');
    expect(screen.getByText('example.com')).toBeVisible();
    await user.click(screen.getByRole('button', { name: /Save changes/ }));
    await waitFor(() => expect(client.updateSubscriptionChannel).toHaveBeenCalledWith(
      channel.id,
      expect.objectContaining({ config: expect.objectContaining({ policy: expect.objectContaining({
        selection: { ids: ['node-one', 'node-two'], excluded_ids: [], new_node_policy: 'exclude' },
        groups: [
          expect.objectContaining({ name: 'Proxy', node_ids: ['node-one', 'node-two'], default_exit: { kind: 'node', id: 'node-two' }, rules: [expect.objectContaining({ value: 'example.com', exit: { kind: 'node', id: 'node-one' } })] }),
          expect.objectContaining({ name: 'Direct sites' }),
        ],
      }) }) }),
      channel.updated_at, expect.any(AbortSignal),
    ));
    expect(screen.getByRole('button', { name: /Save changes/ })).toBeDisabled();
    await user.type(screen.getByLabelText('Group name'), ' edited');
    await user.click(screen.getByRole('button', { name: /Save changes/ }));
    await waitFor(() => expect(client.updateSubscriptionChannel).toHaveBeenLastCalledWith(
      channel.id, expect.anything(), '2026-09-19T10:00:00Z', expect.any(AbortSignal),
    ));
  });
  it('filters candidate cards and selects visible matches without losing existing members', async () => {
    const user = userEvent.setup();
    const client = mount(channel, [
      node,
      { ...node, id: 'node-two', name: 'Seattle', source_name: 'Remote provider', type: 'trojan' },
      { ...node, id: 'node-three', name: 'London' },
      { ...node, id: 'hidden', name: 'Hidden node', hidden: true },
    ]);
    await user.click(screen.getByRole('button', { name: 'Add strategy group' }));
    const search = screen.getByRole('textbox', { name: 'Search nodes, sources or protocols' });
    await user.type(search, 'REMOTE');
    expect(screen.getByRole('checkbox', { name: /Seattle/ })).not.toBeChecked();
    expect(screen.queryByRole('checkbox', { name: /Tokyo|London|Hidden/ })).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Select all' }));
    expect(screen.getByRole('checkbox', { name: /Seattle/ })).toBeChecked();
    await user.clear(search);
    await user.type(search, 'trojan');
    expect(screen.getByRole('checkbox', { name: /Seattle/ })).toBeChecked();
    await user.clear(search);
    await user.type(search, 'unknown');
    expect(screen.getByText('No matching nodes')).toBeVisible();
    await user.clear(search);
    expect(screen.getByRole('checkbox', { name: /Tokyo/ })).toBeChecked();
    expect(screen.getByRole('checkbox', { name: /London/ })).not.toBeChecked();
    await user.click(screen.getByRole('button', { name: /Save changes/ }));
    await waitFor(() => expect(client.updateSubscriptionChannel).toHaveBeenCalledWith(
      channel.id,
      expect.objectContaining({ config: expect.objectContaining({ policy: expect.objectContaining({
        groups: [expect.objectContaining({ node_ids: ['node-one', 'node-two'] })],
      }) }) }),
      channel.updated_at, expect.any(AbortSignal),
    ));
  });
  it('protects referenced exits and allows removal after the reference changes', async () => {
    const user = userEvent.setup();
    const client = mount();
    await user.click(screen.getByRole('button', { name: 'Add strategy group' }));
    await user.click(screen.getByRole('checkbox', { name: /Tokyo/ }));
    expect(screen.getByRole('alert')).toHaveTextContent('This node is used by an exit');
    expect(screen.getByRole('checkbox', { name: /Tokyo/ })).toBeChecked();
    await user.click(screen.getByRole('tab', { name: 'Exit rules' }));
    await user.selectOptions(screen.getByLabelText('Default exit'), 'direct');
    await user.click(screen.getByRole('tab', { name: 'Candidate nodes' }));
    await user.click(screen.getByRole('checkbox', { name: /Tokyo/ }));
    expect(screen.getByRole('checkbox', { name: /Tokyo/ })).not.toBeChecked();
    await user.selectOptions(screen.getByLabelText('Unmatched traffic'), screen.getByRole('option', { name: 'Strategy group 1' }));
    await user.click(screen.getByRole('button', { name: 'Delete strategy group' }));
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    await user.click(screen.getByRole('checkbox', { name: 'Enabled' }));
    expect(screen.getByRole('checkbox', { name: 'Enabled' })).toBeChecked();
    await user.selectOptions(screen.getByLabelText('Unmatched traffic'), 'direct');
    await user.click(screen.getByRole('button', { name: 'Delete strategy group' }));
    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Delete strategy group' }));
    expect(screen.getByText('No strategy groups yet')).toBeVisible();
    expect(client.updateSubscriptionChannel).not.toHaveBeenCalled();
  });
  it('retains group edits after a rejected save', async () => {
    const user = userEvent.setup();
    const client = mount();
    vi.mocked(client.updateSubscriptionChannel).mockRejectedValueOnce(new Error('revision conflict'));
    await user.click(screen.getByRole('button', { name: 'Add strategy group' }));
    await user.click(screen.getByRole('button', { name: /Save changes/ }));
    await waitFor(() => expect(screen.getByRole('button', { name: /Save changes/ })).toBeEnabled());
    expect(screen.getByLabelText('Group name')).toHaveValue('Strategy group 1');
    expect(screen.getByRole('checkbox', { name: /Tokyo/ })).toBeChecked();
  });
  it('copies a channel URL using an existing key without persisting the key', async () => {
    const user = userEvent.setup();
    const client = mount();
    await user.click(screen.getByRole('button', { name: 'Distribution settings' }));
    const dialog = screen.getByRole('dialog', { name: 'Distribution settings' });
    expect(within(dialog).getByRole('button', { name: 'Copy subscription URL' })).toBeDisabled();
    await user.type(within(dialog).getByLabelText('Subscription key'), '  shared-key  ');
    await user.click(within(dialog).getByRole('button', { name: 'Copy subscription URL' }));
    expect(await navigator.clipboard.readText()).toBe(new URL(`sub/shared-key/${channel.id}`, document.baseURI).toString());
    expect(client.updateSubscriptionChannel).not.toHaveBeenCalled();
    await user.click(within(dialog).getByRole('button', { name: 'Done' }));
    expect(screen.getByRole('button', { name: /Save changes/ })).toBeDisabled();
    await user.click(screen.getByRole('button', { name: 'Distribution settings' }));
    expect(screen.getByLabelText('Subscription key')).toHaveValue('');
    await user.click(screen.getByRole('button', { name: 'Cancel' }));
    await user.click(screen.getByRole('button', { name: 'Add strategy group' }));
    await user.click(screen.getByRole('button', { name: 'Distribution settings' }));
    await user.type(screen.getByLabelText('Subscription key'), 'shared-key');
    expect(screen.getByRole('button', { name: 'Copy subscription URL' })).toBeDisabled();
    expect(screen.getByText('Save channel changes before copying the subscription URL.')).toBeVisible();
  });
  it('sends unsaved native template bytes to the server and saves only after successful validation', async () => {
    const user = userEvent.setup();
    const client = mount();
    await user.click(screen.getByRole('tab', { name: 'Strategy groups' }));
    await user.click(screen.getByRole('button', { name: 'Distribution settings' }));
    await user.click(screen.getByRole('button', { name: 'Default configuration' }));
    const modal = screen.getByRole('dialog', { name: 'Edit template' });
    await user.clear(within(modal).getByRole('textbox', { name: 'Native configuration' }));
    await user.type(
      within(modal).getByRole('textbox', { name: 'Native configuration' }),
      '{{"log":{{"level":"error"}}',
    );
    await user.click(within(modal).getByRole('button', { name: 'Save template' }));
    await waitFor(() =>
      expect(client.previewSubscriptionChannel).toHaveBeenCalledWith(
        channel.id,
        '',
        expect.any(AbortSignal),
        expect.objectContaining({
          config: expect.objectContaining({
            policy: expect.objectContaining({
              template: { format: 'sing-box', content: '{"log":{"level":"error"}}' },
            }),
          }),
        }),
      ),
    );
    await waitFor(() => expect(client.updateSubscriptionChannel).toHaveBeenCalled());
  });
});
