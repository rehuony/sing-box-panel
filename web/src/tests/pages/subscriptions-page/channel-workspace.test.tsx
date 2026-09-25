import { useState } from 'react';
import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import { render, screen, waitFor, within } from '@testing-library/react';

import type { SubscriptionChannel, SubscriptionNodeSummary } from '@/api/api-client';

import '@/i18n';
import { toast } from '@/components/ui/toast-manager';
import { ApiClientProvider } from '@/api/api-client-context';
import { TestRouter as MemoryRouter } from '@/tests/test-router';
import { newRuleGroup } from '@/pages/subscriptions-page/channel-policy';
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
      <ChannelWorkspace channel={saved} nodes={nodes} onBack={vi.fn()} onSaved={setSaved} />
    );
  }
  render(
    <MemoryRouter>
      <ApiClientProvider client={client}><Workspace /></ApiClientProvider>
    </MemoryRouter>,
  );
  return client;
}
async function openGroupSettings(user: ReturnType<typeof userEvent.setup>) {
  if (screen.queryByRole('dialog', { name: 'Strategy group settings' })) return;
  const sidebar = screen.getByRole('complementary', { name: 'Strategy groups' });
  const selected = sidebar.querySelector('.channel-group-item[aria-pressed="true"]')!;
  await user.click(within(selected.parentElement!).getByRole('button', { name: /^Configure / }));
}
async function addNodes(user: ReturnType<typeof userEvent.setup>, names: (string | RegExp)[] = [/Tokyo/]) {
  await user.click(screen.getByRole('button', { name: 'Add nodes' }));
  const dialog = screen.getByRole('dialog', { name: 'Add nodes' });
  for (const name of names) await user.click(within(dialog).getByRole('checkbox', { name }));
  await user.click(within(dialog).getByRole('button', { name: /^Add \d+ nodes?$/ }));
}
describe('channel workspace', () => {
  it.each([true, false])('preserves channel enablement %s without a form toggle', async (enabled) => {
    const user = userEvent.setup();
    const client = mount({ ...channel, enabled });
    expect(screen.queryByRole('checkbox', { name: 'Channel enabled' })).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Channel settings' }));
    await user.clear(screen.getByRole('textbox', { name: 'Channel name' }));
    await user.type(screen.getByRole('textbox', { name: 'Channel name' }), 'Renamed');
    await user.click(screen.getByRole('button', { name: 'Done' }));
    await user.click(screen.getByRole('button', { name: /Save changes/ }));
    await waitFor(() => expect(client.updateSubscriptionChannel).toHaveBeenCalledWith(
      channel.id, expect.objectContaining({ name: 'Renamed', enabled }), channel.updated_at, expect.any(AbortSignal),
    ));
  });
  it('keeps channel settings local until Done and preserves the save and discard boundaries', async () => {
    const user = userEvent.setup();
    const client = mount();
    expect(screen.queryByRole('button', { name: 'Organize nodes' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Refresh nodes' })).not.toBeInTheDocument();
    expect(screen.queryByRole('textbox', { name: 'Channel name' })).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Channel settings' }));
    await user.clear(screen.getByLabelText('Channel name'));
    expect(screen.getByRole('button', { name: 'Done' })).toBeDisabled();
    await user.type(screen.getByLabelText('Channel name'), 'Cancelled name');
    await user.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(screen.queryByRole('heading', { name: channel.name })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Save changes/ })).toBeDisabled();

    await user.click(screen.getByRole('button', { name: 'Channel settings' }));
    expect(screen.getByLabelText('Channel name')).toHaveValue(channel.name);
    await user.click(screen.getByRole('combobox', { name: 'Output client' }));
    await user.click(await screen.findByRole('option', { name: 'Mihomo' }));
    expect(screen.getByRole('button', { name: 'Default configuration' })).toBeDisabled();
    await user.click(screen.getByRole('button', { name: 'Done' }));
    expect(client.updateSubscriptionChannel).not.toHaveBeenCalled();
    expect(screen.getByRole('button', { name: /Save changes/ })).toBeEnabled();
    await user.click(screen.getByRole('button', { name: 'Back' }));
    const confirmation = screen.getByRole('alertdialog', { name: 'Discard unsaved changes?' });
    await user.click(within(confirmation).getByRole('button', { name: 'Keep editing' }));
    await user.click(screen.getByRole('button', { name: /Save changes/ }));
    await waitFor(() => expect(client.updateSubscriptionChannel).toHaveBeenCalledWith(
      channel.id, expect.objectContaining({ format: 'mihomo' }), channel.updated_at, expect.any(AbortSignal),
    ));
  });
  it('does not offer edits that cannot be persisted for legacy Loon output', () => {
    mount({ ...channel, format: 'loon', config: {} });
    expect(screen.queryByRole('button', { name: 'Add strategy group' })).not.toBeInTheDocument();
    expect(screen.getByText(/This channel retains Loon output/)).toBeVisible();
    expect(screen.getByRole('button', { name: /Save changes/ })).toBeDisabled();
  });
  it('selects nodes inside a group without a separate channel selection step', async () => {
    const user = userEvent.setup();
    const client = mount({ ...channel, config: { policy: {
      ...channel.config.policy!, selection: { ids: [], excluded_ids: ['node-one'], new_node_policy: 'exclude' },
    } } });
    expect(screen.queryByRole('tab', { name: 'Node selection' })).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Add strategy group' }));
    expect(screen.queryByText('Hidden node')).not.toBeInTheDocument();
    expect(screen.queryByRole('checkbox', { name: /Tokyo/ })).not.toBeInTheDocument();
    await addNodes(user);
    await user.click(screen.getByRole('button', { name: /Save changes/ }));
    await waitFor(() => expect(client.updateSubscriptionChannel).toHaveBeenCalledWith(
      channel.id,
      expect.objectContaining({ config: { policy: expect.objectContaining({
        selection: { ids: ['node-one'], excluded_ids: [], new_node_policy: 'exclude' },
        groups: [expect.objectContaining({ node_ids: ['node-one'] })],
      }) } }),
      channel.updated_at, expect.any(AbortSignal),
    ));
  });
  it('renames directly from an unselected group card and discards cancelled edits', async () => {
    const user = userEvent.setup();
    const client = mount();
    await user.click(screen.getByRole('button', { name: 'Add strategy group' }));
    await user.click(screen.getByRole('button', { name: 'Add strategy group' }));
    expect(screen.queryByRole('button', { name: 'Edit strategy group' })).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Configure Strategy group 1' }));
    expect(screen.getByLabelText('Group name')).toHaveValue('Strategy group 1');
    expect(screen.queryByRole('switch')).not.toBeInTheDocument();
    await user.clear(screen.getByLabelText('Group name'));
    expect(screen.getByRole('button', { name: 'Done' })).toBeDisabled();
    await user.type(screen.getByLabelText('Group name'), 'Cancelled');
    await user.click(screen.getByRole('button', { name: 'Cancel' }));
    await user.click(screen.getByRole('button', { name: 'Configure Strategy group 1' }));
    expect(screen.getByLabelText('Group name')).toHaveValue('Strategy group 1');
    await user.clear(screen.getByLabelText('Group name'));
    await user.type(screen.getByLabelText('Group name'), '  Work  ');
    await user.click(screen.getByRole('button', { name: 'Done' }));
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: /^Work/ })).toHaveAttribute('aria-pressed', 'false');
    expect(screen.getByRole('button', { name: /^Strategy group 2/ })).toHaveAttribute('aria-pressed', 'true');
    await user.click(screen.getByRole('button', { name: 'Configure Work' }));
    await user.type(screen.getByLabelText('Group name'), ' discarded');
    await user.keyboard('{Escape}');
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: /^Work/ })).not.toHaveTextContent('discarded');
    expect(client.updateSubscriptionChannel).not.toHaveBeenCalled();
  });
  it('preserves stored disabled groups on cancel and enables a group when made final', async () => {
    const user = userEvent.setup();
    const group = { ...newRuleGroup([node.id]), name: 'Imported group', enabled: false };
    const client = mount({ ...channel, config: { policy: { ...channel.config.policy!, groups: [group] } } });
    await openGroupSettings(user);
    await user.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(screen.getByRole('button', { name: /Save changes/ })).toBeDisabled();
    expect(screen.getByRole('button', { name: /^Imported group/ })).toHaveTextContent('Disabled');
    await user.click(screen.getByRole('button', { name: 'Set as final exit' }));
    expect(screen.getByRole('button', { name: /^Imported group/ })).not.toHaveTextContent('Disabled');
    expect(screen.getByRole('button', { name: /^Imported group/ })).toHaveTextContent('Final exit');
    await user.click(screen.getByRole('button', { name: /Save changes/ }));
    await waitFor(() => expect(client.updateSubscriptionChannel).toHaveBeenCalled());
    const saved = vi.mocked(client.updateSubscriptionChannel).mock.calls[0][1].config?.policy;
    expect(saved?.groups[0]).toMatchObject({ id: group.id, enabled: true });
    expect(saved?.default_exit).toEqual({ kind: 'group', id: group.id });
  });
  it('configures strategy types and health checks with matching card icons', async () => {
    const user = userEvent.setup();
    const client = mount({ ...channel, format: 'mihomo' });
    await user.click(screen.getByRole('button', { name: 'Add strategy group' }));
    const card = () => screen.getByRole('button', { name: /^Strategy group 1/ });
    expect(card().querySelector('.lucide-mouse-pointer-2')).toBeInTheDocument();
    await openGroupSettings(user);
    await user.click(screen.getByRole('combobox', { name: 'Group type' }));
    await user.click(await screen.findByRole('option', { name: 'Automatic latency test' }));
    await user.clear(screen.getByLabelText('Test link'));
    await user.type(screen.getByLabelText('Test link'), 'https://probe.example/check');
    await user.clear(screen.getByLabelText('Interval (s)'));
    await user.type(screen.getByLabelText('Interval (s)'), '600');
    await user.clear(screen.getByLabelText('Tolerance (ms)'));
    await user.type(screen.getByLabelText('Tolerance (ms)'), '0');
    await user.click(screen.getByRole('button', { name: 'Done' }));
    expect(card().querySelector('.lucide-gauge')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /Save changes/ }));
    await waitFor(() => expect(client.updateSubscriptionChannel).toHaveBeenCalledTimes(1));
    expect(vi.mocked(client.updateSubscriptionChannel).mock.calls[0][1].config?.policy?.groups[0]).toMatchObject({ type: 'url-test', builtin_nodes: [], health_check: { url: 'https://probe.example/check', interval: 600, tolerance: 0 } });
    await openGroupSettings(user);
    await user.click(screen.getByRole('combobox', { name: 'Group type' }));
    await user.click(await screen.findByRole('option', { name: 'Fallback' }));
    expect(screen.queryByLabelText('Tolerance (ms)')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(card().querySelector('.lucide-gauge')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Save changes/ })).toBeDisabled();
    await openGroupSettings(user);
    await user.click(screen.getByRole('combobox', { name: 'Group type' }));
    await user.click(await screen.findByRole('option', { name: 'Fallback' }));
    await user.click(screen.getByRole('button', { name: 'Done' }));
    expect(card().querySelector('.lucide-list-restart')).toBeInTheDocument();
    await openGroupSettings(user);
    await user.click(screen.getByRole('combobox', { name: 'Group type' }));
    await user.click(await screen.findByRole('option', { name: 'Manual selection' }));
    expect(screen.queryByLabelText('Test link')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Done' }));
    expect(card().querySelector('.lucide-mouse-pointer-2')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /Save changes/ }));
    await waitFor(() => expect(client.updateSubscriptionChannel).toHaveBeenCalledTimes(2));
    const manualGroup = vi.mocked(client.updateSubscriptionChannel).mock.calls[1][1].config?.policy?.groups[0];
    expect(manualGroup?.health_check).toBeUndefined();
  });
  it('adds built-in candidates without publication IDs and removes them explicitly', async () => {
    const user = userEvent.setup();
    const client = mount({ ...channel, format: 'mihomo' });
    await user.click(screen.getByRole('button', { name: 'Add strategy group' }));
    await addNodes(user, ['DIRECT']);
    expect(screen.getByRole('checkbox', { name: 'DIRECT' })).not.toBeChecked();
    await user.click(screen.getByRole('button', { name: 'Add nodes' }));
    const picker = screen.getByRole('dialog', { name: 'Add nodes' });
    expect(within(picker).getByRole('checkbox', { name: 'DIRECT' })).toHaveAttribute('aria-disabled', 'true');
    await user.click(within(picker).getByRole('checkbox', { name: 'REJECT' }));
    await user.click(within(picker).getByRole('button', { name: /^Add 1 node/ }));
    await user.click(screen.getByRole('button', { name: /Save changes/ }));
    await waitFor(() => expect(client.updateSubscriptionChannel).toHaveBeenCalledTimes(1));
    expect(vi.mocked(client.updateSubscriptionChannel).mock.calls[0][1].config?.policy?.groups[0]).toMatchObject({ node_ids: [], builtin_nodes: ['direct', 'reject'], default_exit: { kind: 'direct' } });
    await user.click(screen.getByRole('checkbox', { name: 'DIRECT' }));
    expect(screen.getByRole('button', { name: /Save changes/ })).toBeDisabled();
    await user.click(screen.getByRole('button', { name: 'Remove selected nodes' }));
    await user.click(screen.getByRole('button', { name: /Save changes/ }));
    await waitFor(() => expect(client.updateSubscriptionChannel).toHaveBeenCalledTimes(2));
    expect(vi.mocked(client.updateSubscriptionChannel).mock.calls[1][1].config?.policy?.groups[0]).toMatchObject({ node_ids: [], builtin_nodes: ['reject'], default_exit: { kind: 'reject' } });
    await user.click(screen.getByRole('button', { name: 'Select all' }));
    await user.click(screen.getByRole('button', { name: 'Remove selected nodes' }));
    expect(screen.queryByRole('checkbox', { name: 'REJECT' })).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /Save changes/ }));
    await waitFor(() => expect(client.updateSubscriptionChannel).toHaveBeenCalledTimes(3));
    expect(vi.mocked(client.updateSubscriptionChannel).mock.calls[2][1].config?.policy?.groups[0]).toMatchObject({ builtin_nodes: [], default_exit: { kind: 'reject' } });
  });
  it('disables unsupported sing-box group and built-in types', async () => {
    const user = userEvent.setup();
    mount();
    await user.click(screen.getByRole('button', { name: 'Add strategy group' }));
    await user.click(screen.getByRole('button', { name: 'Add nodes' }));
    expect(screen.getByRole('checkbox', { name: 'REJECT' })).toHaveAttribute('aria-disabled', 'true');
    await user.keyboard('{Escape}');
    await openGroupSettings(user);
    await user.click(screen.getByRole('combobox', { name: 'Group type' }));
    expect(await screen.findByRole('option', { name: 'Fallback' })).toHaveAttribute('aria-disabled', 'true');
  });
  it('keeps the parent strategy group open while adding a remote reference and blocks an empty format', async () => {
    const addToast = vi.spyOn(toast, 'add');
    const user = userEvent.setup();
    mount();
    await user.click(screen.getByRole('button', { name: 'Add strategy group' }));
    const parent = screen.getByRole('region', { name: 'Edit strategy group' });
    await openGroupSettings(user);
    await user.clear(screen.getByLabelText('Group name'));
    await user.type(screen.getByLabelText('Group name'), 'Proxy');
    await user.click(screen.getByRole('button', { name: 'Done' }));
    await addNodes(user);
    expect(within(parent).getByRole('checkbox', { name: /Tokyo/ })).not.toBeChecked();
    await user.click(within(parent).getByRole('tab', { name: 'Exit rules' }));
    await user.click(within(parent).getByRole('button', { name: 'Add rule set' }));
    const child = screen.getByRole('dialog', { name: 'Edit rule set' });
    expect(within(child).queryByRole('combobox', { name: 'Traffic exit' })).not.toBeInTheDocument();
    expect(within(child).queryByRole('checkbox', { name: 'Enabled' })).not.toBeInTheDocument();
    const acceleration = within(child).getByRole('button', { name: 'Accelerate GitHub file' });
    expect(acceleration).toBeEnabled();
    await user.click(acceleration);
    expect(addToast).toHaveBeenCalledWith(expect.objectContaining({ type: 'info', title: 'Enter a GitHub file link to enable acceleration.' }));
    expect(acceleration).toHaveAttribute('aria-pressed', 'false');
    const link = within(child).getByLabelText('Link');
    await user.type(link, 'https://example.com/list.srs');
    await user.click(acceleration);
    expect(acceleration).toHaveAttribute('aria-pressed', 'false');
    expect(link).toHaveValue('https://example.com/list.srs');
    await user.clear(link);
    await user.click(within(child).getByRole('button', { name: 'Done' }));
    expect(within(child).getByLabelText('Source format')).toHaveAttribute('aria-invalid', 'true');
    expect(within(child).queryByRole('alert')).not.toBeInTheDocument();
    await user.type(within(child).getByLabelText('Name'), 'Domains');
    await user.type(
      within(child).getByLabelText('Link'),
      'https://raw.githubusercontent.com/example/rules/main/list.srs',
    );
    await user.click(within(child).getByRole('button', { name: 'Accelerate GitHub file' }));
    expect(within(child).getByLabelText('Link')).toHaveValue(
      'https://gh-proxy.com/https://raw.githubusercontent.com/example/rules/main/list.srs',
    );
    expect(acceleration).toHaveAttribute('aria-pressed', 'true');
    await user.click(acceleration);
    expect(acceleration).toHaveAttribute('aria-pressed', 'false');
    expect(link).toHaveValue('https://raw.githubusercontent.com/example/rules/main/list.srs');
    await user.click(acceleration);
    await user.click(within(child).getByRole('combobox', { name: 'Source format' }));
    await user.click(await screen.findByRole('option', { name: 'SRS (binary)' }));
    await user.click(within(child).getByRole('button', { name: 'Done' }));
    await waitFor(() =>
      expect(screen.queryByRole('dialog', { name: 'Edit rule set' })).not.toBeInTheDocument(),
    );
    expect(screen.getByRole('region', { name: 'Edit strategy group' })).toBeInTheDocument();
    expect(within(parent).getByText('Domains')).toBeInTheDocument();
    addToast.mockRestore();
  });
  it.each([false, true])('applies implicit enablement and group routing only after confirming a rule (remote: %s)', async (remote) => {
    const user = userEvent.setup();
    const rule = {
      id: 'stored-rule', enabled: false, exit: { kind: 'node' as const, id: node.id },
      ...(remote
        ? {
            kind: 'remote' as const,
            remote: { name: 'Domains', url: 'https://example.com/rules.srs', format: 'binary' as const, accelerated: false, update_interval: 3600 },
          }
        : { kind: 'domain' as const, value: 'example.com' }),
    };
    const group = { ...newRuleGroup([node.id]), name: 'Proxy', rules: [rule] };
    const client = mount({ ...channel, config: { policy: { ...channel.config.policy!, groups: [group] } } });
    await user.click(screen.getByRole('tab', { name: 'Exit rules' }));
    await user.click(screen.getByRole('button', { name: 'Edit rule' }));
    const dialog = screen.getByRole('dialog');
    expect(within(dialog).queryByLabelText('Traffic exit')).not.toBeInTheDocument();
    expect(within(dialog).queryByLabelText('Enabled')).not.toBeInTheDocument();
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }));
    expect(screen.getByRole('button', { name: /Save changes/ })).toBeDisabled();
    await user.click(screen.getByRole('button', { name: 'Edit rule' }));
    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Done' }));
    await user.click(screen.getByRole('button', { name: /Save changes/ }));
    await waitFor(() => expect(client.updateSubscriptionChannel).toHaveBeenCalledOnce());
    const saved = vi.mocked(client.updateSubscriptionChannel).mock.calls[0][1].config?.policy;
    expect(saved?.groups[0].rules[0]).toMatchObject({
      id: rule.id, enabled: true, exit: { kind: 'group-default' },
    });
  });
  it('keeps group drafts when switching and saves selected nodes and exit rules together', async () => {
    const user = userEvent.setup();
    const client = mount({
      ...channel,
      config: { policy: { ...channel.config.policy!, selection: { ids: ['node-one'], excluded_ids: ['node-two'], new_node_policy: 'exclude' } } },
    }, [node, { ...node, id: 'node-two', name: 'Seattle' }]);
    await user.click(screen.getByRole('button', { name: 'Add strategy group' }));
    await openGroupSettings(user);
    await user.clear(screen.getByLabelText('Group name'));
    await user.type(screen.getByLabelText('Group name'), 'Proxy');
    await user.click(screen.getByRole('button', { name: 'Done' }));
    await addNodes(user, [/Tokyo/, /Seattle/]);
    await user.click(screen.getByRole('tab', { name: 'Exit rules' }));
    expect(screen.queryByRole('combobox', { name: 'Default exit' })).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Add rule' }));
    const rule = screen.getByRole('dialog', { name: 'Edit rule' });
    await user.type(within(rule).getByLabelText('Match value'), 'example.com');
    expect(within(rule).queryByRole('combobox', { name: 'Traffic exit' })).not.toBeInTheDocument();
    expect(within(rule).queryByRole('checkbox', { name: 'Enabled' })).not.toBeInTheDocument();
    await user.click(within(rule).getByRole('button', { name: 'Done' }));
    await user.click(screen.getByRole('button', { name: 'Add strategy group' }));
    await openGroupSettings(user);
    await user.clear(screen.getByLabelText('Group name'));
    await user.type(screen.getByLabelText('Group name'), 'Direct sites');
    await user.click(screen.getByRole('button', { name: 'Done' }));
    const sidebar = screen.getByRole('complementary', { name: 'Strategy groups' });
    await user.click(within(sidebar).getByRole('button', { name: /^Proxy/ }));
    await openGroupSettings(user);
    expect(screen.getByLabelText('Group name')).toHaveValue('Proxy');
    await user.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(screen.getByRole('checkbox', { name: /Seattle/ })).not.toBeChecked();
    await user.click(screen.getByRole('tab', { name: 'Exit rules' }));
    expect(screen.queryByRole('combobox', { name: 'Default exit' })).not.toBeInTheDocument();
    expect(screen.getByText('example.com')).toBeVisible();
    await user.click(screen.getByRole('button', { name: /Save changes/ }));
    await waitFor(() => expect(client.updateSubscriptionChannel).toHaveBeenCalledWith(
      channel.id,
      expect.objectContaining({ config: expect.objectContaining({ policy: expect.objectContaining({
        selection: { ids: ['node-one', 'node-two'], excluded_ids: [], new_node_policy: 'exclude' },
        groups: [
          expect.objectContaining({ name: 'Proxy', node_ids: ['node-one', 'node-two'], default_exit: { kind: 'node', id: 'node-one' }, rules: [expect.objectContaining({ value: 'example.com', enabled: true, exit: { kind: 'group-default' } })] }),
          expect.objectContaining({ name: 'Direct sites' }),
        ],
      }) }) }),
      channel.updated_at, expect.any(AbortSignal),
    ));
    expect(screen.getByRole('button', { name: /Save changes/ })).toBeDisabled();
    await openGroupSettings(user);
    await user.type(screen.getByLabelText('Group name'), ' edited');
    await user.click(screen.getByRole('button', { name: 'Done' }));
    await user.click(screen.getByRole('button', { name: /Save changes/ }));
    await waitFor(() => expect(client.updateSubscriptionChannel).toHaveBeenLastCalledWith(
      channel.id, expect.anything(), '2026-09-19T10:00:00Z', expect.any(AbortSignal),
    ));
  });
  it('selects and clears filtered cards without changing membership, exits, rules or the saved draft', async () => {
    const user = userEvent.setup();
    const group = { ...newRuleGroup(['node-one', 'node-two']), name: 'Proxy', builtin_nodes: ['direct' as const], rules: [
      { id: 'rule-one', enabled: true, kind: 'domain' as const, value: 'example.com', exit: { kind: 'node' as const, id: 'node-one' } },
    ] };
    const client = mount({ ...channel, config: { policy: { ...channel.config.policy!, groups: [group] } } }, [
      node,
      { ...node, id: 'node-two', name: 'Seattle', source_name: 'Remote provider', type: 'trojan' },
      { ...node, id: 'node-three', name: 'London' },
      { ...node, id: 'hidden', name: 'Hidden node', hidden: true },
    ]);
    const editor = screen.getByRole('region', { name: 'Edit strategy group' });
    const actionNames = () => within(editor).getAllByRole('button').map((button) => button.getAttribute('aria-label'));
    expect(actionNames()).toEqual(['Select all', 'Add nodes']);
    expect(screen.getByRole('checkbox', { name: /Tokyo/ })).not.toBeChecked();
    await user.click(screen.getByRole('checkbox', { name: /Tokyo/ }));
    expect(actionNames()).toEqual(['Clear selection', 'Remove selected nodes', 'Select all', 'Add nodes']);
    expect(screen.getByRole('status', { name: '1 selected' })).toHaveTextContent('1');
    await user.keyboard(' ');
    expect(screen.getByRole('checkbox', { name: /Tokyo/ })).not.toBeChecked();
    await user.keyboard('{Enter}');
    expect(screen.getByRole('checkbox', { name: /Tokyo/ })).toBeChecked();
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
    expect(screen.getByRole('button', { name: 'Select all' })).toBeDisabled();
    expect(screen.getByRole('status', { name: '2 selected' })).toBeVisible();
    await user.click(screen.getByRole('button', { name: 'Clear selection' }));
    expect(actionNames()).toEqual(['Select all', 'Add nodes']);
    await user.clear(search);
    expect(screen.getByRole('checkbox', { name: /Tokyo/ })).not.toBeChecked();
    expect(screen.getByRole('checkbox', { name: /Seattle/ })).not.toBeChecked();
    expect(screen.getByRole('checkbox', { name: 'DIRECT' })).not.toBeChecked();
    expect(screen.queryByRole('checkbox', { name: /London/ })).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Select all' }));
    expect(screen.getByRole('status', { name: '3 selected' })).toBeVisible();
    expect(screen.getByRole('checkbox', { name: 'DIRECT' })).toBeChecked();
    await user.click(screen.getByRole('button', { name: 'Clear selection' }));
    expect(screen.getByRole('button', { name: /Save changes/ })).toBeDisabled();
    expect(client.updateSubscriptionChannel).not.toHaveBeenCalled();
    await user.click(screen.getByRole('button', { name: 'Channel settings' }));
    await user.type(screen.getByLabelText('Channel name'), ' renamed');
    await user.click(screen.getByRole('button', { name: 'Done' }));
    await user.click(screen.getByRole('button', { name: /Save changes/ }));
    await waitFor(() => expect(client.updateSubscriptionChannel).toHaveBeenCalledWith(
      channel.id,
      expect.objectContaining({ config: expect.objectContaining({ policy: expect.objectContaining({
        groups: [group],
      }) }) }),
      channel.updated_at, expect.any(AbortSignal),
    ));
  });
  it('resets card selection when switching groups without changing either membership', async () => {
    const user = userEvent.setup();
    const groups = [
      { ...newRuleGroup(['node-one']), name: 'Proxy' },
      { ...newRuleGroup(['node-one']), name: 'Other' },
    ];
    const client = mount({ ...channel, config: { policy: { ...channel.config.policy!, groups } } });
    await user.click(screen.getByRole('checkbox', { name: /Tokyo/ }));
    expect(screen.getByRole('checkbox', { name: /Tokyo/ })).toBeChecked();
    const sidebar = screen.getByRole('complementary', { name: 'Strategy groups' });
    await user.click(within(sidebar).getByRole('button', { name: /^Other/ }));
    expect(screen.getByRole('checkbox', { name: /Tokyo/ })).not.toBeChecked();
    expect(screen.queryByRole('button', { name: 'Remove selected nodes' })).not.toBeInTheDocument();
    await user.click(within(sidebar).getByRole('button', { name: /^Proxy/ }));
    expect(screen.getByRole('checkbox', { name: /Tokyo/ })).not.toBeChecked();
    expect(screen.getByRole('button', { name: /Save changes/ })).toBeDisabled();
    expect(client.updateSubscriptionChannel).not.toHaveBeenCalled();
  });
  it('can remove unavailable members while preserving remaining candidate order', async () => {
    const user = userEvent.setup();
    const group = {
      ...newRuleGroup(['node-one', 'hidden', 'missing']), name: 'Proxy', builtin_nodes: ['direct' as const],
      candidate_order: ['node:hidden', 'builtin:direct', 'node:missing', 'node:node-one'],
    };
    const client = mount({ ...channel, config: { policy: { ...channel.config.policy!, groups: [group] } } });
    await user.click(screen.getByRole('checkbox', { name: /Hidden node/ }));
    await user.click(screen.getByRole('checkbox', { name: 'Unavailable node' }));
    expect(screen.getByRole('status', { name: '2 selected' })).toBeVisible();
    await user.click(screen.getByRole('button', { name: 'Remove selected nodes' }));
    expect(screen.getAllByRole('checkbox').map((card) => card.getAttribute('aria-label')))
      .toEqual(['DIRECT', 'Tokyo Manual nodes']);
    await user.click(screen.getByRole('button', { name: /Save changes/ }));
    await waitFor(() => expect(client.updateSubscriptionChannel).toHaveBeenCalledOnce());
    expect(vi.mocked(client.updateSubscriptionChannel).mock.calls[0][1].config?.policy?.groups[0]).toMatchObject({
      node_ids: ['node-one'], builtin_nodes: ['direct'], candidate_order: ['builtin:direct', 'node:node-one'],
      default_exit: { kind: 'node', id: 'node-one' },
    });
  });
  it('removes only selected nodes, repairs their exits and leaves empty groups rejecting traffic', async () => {
    const user = userEvent.setup();
    const legacy = { ...newRuleGroup(['node-one', 'node-two']), name: 'Proxy', rules: [
      { id: 'old-rule', enabled: true, kind: 'domain' as const, value: 'example.com', exit: { kind: 'node' as const, id: 'node-one' } },
    ] };
    const client = mount({ ...channel, config: { policy: { ...channel.config.policy!, groups: [legacy] } } }, [node, { ...node, id: 'node-two', name: 'Seattle' }]);
    await user.click(screen.getByRole('checkbox', { name: /Tokyo/ }));
    expect(screen.getByRole('button', { name: /Save changes/ })).toBeDisabled();
    await user.type(screen.getByRole('textbox', { name: 'Search nodes, sources or protocols' }), 'Seattle');
    await user.click(screen.getByRole('button', { name: 'Remove selected nodes' }));
    await user.clear(screen.getByRole('textbox', { name: 'Search nodes, sources or protocols' }));
    expect(screen.queryByRole('checkbox', { name: /Tokyo/ })).not.toBeInTheDocument();
    expect(screen.getByRole('checkbox', { name: /Seattle/ })).not.toBeChecked();
    expect(screen.queryByRole('button', { name: 'Remove selected nodes' })).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /Save changes/ }));
    await waitFor(() => expect(client.updateSubscriptionChannel).toHaveBeenCalledTimes(1));
    expect(vi.mocked(client.updateSubscriptionChannel).mock.calls[0][1].config?.policy?.groups[0]).toMatchObject({
      node_ids: ['node-two'], default_exit: { kind: 'node', id: 'node-two' }, rules: [{ exit: { kind: 'group-default' } }],
    });
    await user.click(screen.getByRole('button', { name: 'Select all' }));
    await user.click(screen.getByRole('button', { name: 'Remove selected nodes' }));
    await user.click(screen.getByRole('button', { name: /Save changes/ }));
    await waitFor(() => expect(client.updateSubscriptionChannel).toHaveBeenCalledTimes(2));
    expect(vi.mocked(client.updateSubscriptionChannel).mock.calls[1][1].config?.policy?.groups[0]).toMatchObject({ node_ids: [], default_exit: { kind: 'reject' } });
    await addNodes(user);
    await user.click(screen.getByRole('button', { name: /Save changes/ }));
    await waitFor(() => expect(client.updateSubscriptionChannel).toHaveBeenCalledTimes(3));
    expect(vi.mocked(client.updateSubscriptionChannel).mock.calls[2][1].config?.policy?.groups[0]).toMatchObject({ node_ids: ['node-one'], default_exit: { kind: 'node', id: 'node-one' } });
  });
  it('toggles the exclusive final group from the sidebar and protects it from deletion', async () => {
    const addToast = vi.spyOn(toast, 'add');
    const user = userEvent.setup();
    const client = mount();
    await user.click(screen.getByRole('button', { name: 'Add strategy group' }));
    expect(screen.getByRole('button', { name: 'Set as final exit' })).toHaveAttribute('aria-pressed', 'false');
    await user.click(screen.getByRole('button', { name: /Save changes/ }));
    await waitFor(() => expect(client.updateSubscriptionChannel).toHaveBeenCalledTimes(1));
    expect(vi.mocked(client.updateSubscriptionChannel).mock.calls[0][1].config?.policy?.default_exit).toEqual({ kind: 'direct' });
    await user.click(screen.getByRole('button', { name: 'Set as final exit' }));
    expect(screen.getByRole('button', { name: 'Clear final exit' })).toHaveAttribute('aria-pressed', 'true');
    expect(screen.getByRole('button', { name: /^Strategy group 1/ })).toHaveTextContent('Final exit');
    await user.click(screen.getByRole('button', { name: 'Delete strategy group' }));
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(addToast).toHaveBeenCalledWith(expect.objectContaining({ type: 'error' }));
    await user.click(screen.getByRole('button', { name: 'Add strategy group' }));
    await user.click(screen.getByRole('button', { name: 'Set as final exit' }));
    expect(screen.getByRole('button', { name: /^Strategy group 1/ })).not.toHaveTextContent('Final exit');
    expect(screen.getByRole('button', { name: /^Strategy group 2/ })).toHaveTextContent('Final exit');
    await user.click(screen.getByRole('button', { name: /Save changes/ }));
    await waitFor(() => expect(client.updateSubscriptionChannel).toHaveBeenCalledTimes(2));
    const saved = vi.mocked(client.updateSubscriptionChannel).mock.calls[1][1].config?.policy;
    expect(saved?.default_exit).toEqual({ kind: 'group', id: saved?.groups[1].id });
    await user.click(screen.getByRole('button', { name: /^Strategy group 1/ }));
    await openGroupSettings(user);
    await user.type(screen.getByLabelText('Group name'), ' renamed');
    await user.click(screen.getByRole('button', { name: 'Done' }));
    expect(screen.getByRole('button', { name: /^Strategy group 2/ })).toHaveTextContent('Final exit');
    await user.click(screen.getByRole('button', { name: /^Strategy group 2/ }));
    await user.click(screen.getByRole('button', { name: 'Clear final exit' }));
    await user.click(screen.getByRole('button', { name: /Save changes/ }));
    await waitFor(() => expect(client.updateSubscriptionChannel).toHaveBeenCalledTimes(3));
    expect(vi.mocked(client.updateSubscriptionChannel).mock.calls[2][1].config?.policy?.default_exit).toEqual({ kind: 'direct' });
    await user.click(screen.getByRole('button', { name: 'Delete strategy group' }));
    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Delete strategy group' }));
    expect(screen.queryByRole('button', { name: /^Strategy group 2/ })).not.toBeInTheDocument();
    addToast.mockRestore();
  });
  it.each([{ kind: 'reject' as const }, { kind: 'node' as const, id: node.id }])('preserves an existing $kind fallback when adding a group', async (default_exit) => {
    const user = userEvent.setup();
    const client = mount({ ...channel, config: { policy: { ...channel.config.policy!, default_exit } } });
    await user.click(screen.getByRole('button', { name: 'Add strategy group' }));
    await openGroupSettings(user);
    expect(screen.queryByRole('switch')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Done' }));
    await user.click(screen.getByRole('button', { name: /Save changes/ }));
    await waitFor(() => expect(client.updateSubscriptionChannel).toHaveBeenCalledWith(
      channel.id,
      expect.objectContaining({
        config: expect.objectContaining({ policy: expect.objectContaining({ default_exit }) }),
      }),
      channel.updated_at, expect.any(AbortSignal),
    ));
  });
  it('retains group edits after a rejected save', async () => {
    const user = userEvent.setup();
    const client = mount();
    vi.mocked(client.updateSubscriptionChannel).mockRejectedValueOnce(new Error('revision conflict'));
    await user.click(screen.getByRole('button', { name: 'Add strategy group' }));
    await addNodes(user);
    await user.click(screen.getByRole('button', { name: /Save changes/ }));
    await waitFor(() => expect(screen.getByRole('button', { name: /Save changes/ })).toBeEnabled());
    await openGroupSettings(user);
    expect(screen.getByLabelText('Group name')).toHaveValue('Strategy group 1');
    await user.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(screen.getByRole('checkbox', { name: /Tokyo/ })).not.toBeChecked();
  });
  it('keeps link distribution out of channel settings while preserving channel edits', async () => {
    const user = userEvent.setup();
    const client = mount();
    await user.click(screen.getByRole('button', { name: 'Channel settings' }));
    expect(screen.queryByRole('combobox', { name: 'Bound keys' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Copy subscription URL' })).not.toBeInTheDocument();
    expect(client.listSubscriptionTokens).not.toHaveBeenCalled();
    await user.type(screen.getByLabelText('Channel name'), ' renamed');
    await user.click(screen.getByRole('button', { name: 'Done' }));
    await user.click(screen.getByRole('button', { name: /Save changes/ }));
    await waitFor(() => expect(client.updateSubscriptionChannel).toHaveBeenCalledOnce());
    const savedConfig = vi.mocked(client.updateSubscriptionChannel).mock.calls[0][1].config;
    expect(savedConfig).not.toHaveProperty('export_token_ids');
    expect(client.getSubscriptionTokenSecret).not.toHaveBeenCalled();
    await user.click(screen.getByRole('button', { name: 'Channel settings' }));
    expect(screen.queryByRole('button', { name: 'Copy subscription URL' })).not.toBeInTheDocument();
  });
  it('starts empty and adds only confirmed selections from the picker', async () => {
    const user = userEvent.setup();
    const client = mount({ ...channel, format: 'mihomo' });
    await user.click(screen.getByRole('button', { name: 'Add strategy group' }));
    expect(screen.queryByRole('checkbox')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Add nodes' }));
    await user.click(screen.getByRole('checkbox', { name: 'DIRECT' }));
    await user.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(screen.queryByRole('checkbox')).not.toBeInTheDocument();
    await addNodes(user, [/Tokyo/, 'REJECT']);
    expect(screen.queryByRole('checkbox', { name: 'DIRECT' })).not.toBeInTheDocument();
    expect(screen.getByRole('checkbox', { name: /Tokyo/ })).not.toBeChecked();
    expect(screen.getByRole('checkbox', { name: 'REJECT' })).not.toBeChecked();
    await user.click(screen.getByRole('button', { name: /Save changes/ }));
    await waitFor(() => expect(client.updateSubscriptionChannel).toHaveBeenCalledOnce());
    expect(vi.mocked(client.updateSubscriptionChannel).mock.calls[0][1].config?.policy?.groups[0]).toMatchObject({ node_ids: ['node-one'], builtin_nodes: ['reject'] });
  });
  it('keeps organizer changes local and saves the custom controls through the existing policy', async () => {
    const user = userEvent.setup();
    const client = mount();
    await user.click(screen.getByRole('button', { name: 'Channel settings' }));
    await user.click(screen.getByRole('button', { name: 'Organize nodes' }));
    await user.click(screen.getByRole('switch', { name: 'Deduplicate' }));
    await user.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(screen.getByRole('button', { name: /Save changes/ })).toBeDisabled();
    await user.click(screen.getByRole('button', { name: 'Channel settings' }));
    await user.click(screen.getByRole('button', { name: 'Organize nodes' }));
    expect(screen.getByRole('switch', { name: 'Deduplicate' })).not.toBeChecked();
    await user.type(screen.getByLabelText('Name prefix'), 'Office ');
    await user.type(screen.getByLabelText('Exclude names'), 'Tokyo\nTokyo\nSeattle');
    await user.click(screen.getByRole('switch', { name: 'Deduplicate' }));
    await user.click(screen.getByRole('combobox', { name: 'Node order' }));
    await user.click(await screen.findByRole('option', { name: 'By name' }));
    await user.click(screen.getByRole('combobox', { name: 'Incompatible nodes' }));
    await user.click(await screen.findByRole('option', { name: 'Block generation' }));
    const organizer = screen.getByRole('dialog', { name: 'Organize nodes' });
    await user.click(within(organizer).getByRole('button', { name: 'Back' }));
    await user.click(screen.getByRole('button', { name: 'Organize nodes' }));
    expect(screen.getByRole('switch', { name: 'Deduplicate' })).toBeChecked();
    expect(screen.getByLabelText('Name prefix')).toHaveValue('Office ');
    await user.click(screen.getByRole('button', { name: 'Done' }));
    expect(client.updateSubscriptionChannel).not.toHaveBeenCalled();
    await user.click(screen.getByRole('button', { name: /Save changes/ }));
    await waitFor(() => expect(client.updateSubscriptionChannel).toHaveBeenCalledWith(
      channel.id,
      expect.objectContaining({ config: expect.objectContaining({ policy: expect.objectContaining({
        organizer: {
          prefix: 'Office ', exclude_names: ['Tokyo', 'Seattle'], sort: 'name', deduplicate: true, incompatible: 'error',
        },
      }) }) }),
      channel.updated_at, expect.any(AbortSignal),
    ));
  });
  it('sends unsaved native template bytes to the server and saves only after successful validation', async () => {
    const user = userEvent.setup();
    const client = mount();
    await user.click(screen.getByRole('button', { name: 'Channel settings' }));
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
