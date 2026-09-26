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
describe('channel workspace', () => {
  it('submits confirmed channel settings through the save action', async () => {
    const user = userEvent.setup();
    const client = mount();
    await user.click(screen.getByRole('button', { name: 'Channel settings' }));
    await user.click(screen.getByRole('combobox', { name: 'Output client' }));
    await user.click(await screen.findByRole('option', { name: 'Mihomo' }));
    await user.click(screen.getByRole('button', { name: 'Done' }));
    expect(client.updateSubscriptionChannel).not.toHaveBeenCalled();
    await user.click(screen.getByRole('button', { name: /Save changes/ }));
    await waitFor(() => expect(client.updateSubscriptionChannel).toHaveBeenCalledWith(
      channel.id, expect.objectContaining({ format: 'mihomo' }), channel.updated_at, expect.any(AbortSignal),
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
  it('stages strategy types and health checks until confirmation', async () => {
    const user = userEvent.setup();
    const client = mount({ ...channel, format: 'mihomo' });
    await user.click(screen.getByRole('button', { name: 'Add strategy group' }));
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
    await user.click(screen.getByRole('button', { name: /Save changes/ }));
    await waitFor(() => expect(client.updateSubscriptionChannel).toHaveBeenCalledTimes(1));
    expect(vi.mocked(client.updateSubscriptionChannel).mock.calls[0][1].config?.policy?.groups[0]).toMatchObject({ type: 'url-test', builtin_nodes: [], health_check: { url: 'https://probe.example/check', interval: 600, tolerance: 0 } });
    await openGroupSettings(user);
    await user.click(screen.getByRole('combobox', { name: 'Group type' }));
    await user.click(await screen.findByRole('option', { name: 'Fallback' }));
    expect(screen.queryByLabelText('Tolerance (ms)')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(screen.getByRole('button', { name: /Save changes/ })).toBeDisabled();
    await openGroupSettings(user);
    await user.click(screen.getByRole('combobox', { name: 'Group type' }));
    await user.click(await screen.findByRole('option', { name: 'Fallback' }));
    await user.click(screen.getByRole('button', { name: 'Done' }));
    await openGroupSettings(user);
    await user.click(screen.getByRole('combobox', { name: 'Group type' }));
    await user.click(await screen.findByRole('option', { name: 'Manual selection' }));
    expect(screen.queryByLabelText('Test link')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Done' }));
    await user.click(screen.getByRole('button', { name: /Save changes/ }));
    await waitFor(() => expect(client.updateSubscriptionChannel).toHaveBeenCalledTimes(2));
    const manualGroup = vi.mocked(client.updateSubscriptionChannel).mock.calls[1][1].config?.policy?.groups[0];
    expect(manualGroup?.health_check).toBeUndefined();
  });
  it('keeps the parent strategy group open while adding a remote reference and blocks an empty format', async () => {
    const user = userEvent.setup();
    const group = { ...newRuleGroup([node.id]), name: 'Proxy' };
    mount({ ...channel, config: { policy: { ...channel.config.policy!, groups: [group] } } });
    const parent = screen.getByRole('region', { name: 'Edit strategy group' });
    await user.click(within(parent).getByRole('tab', { name: 'Exit rules' }));
    await user.click(within(parent).getByRole('button', { name: 'Add rule set' }));
    const child = screen.getByRole('dialog', { name: 'Edit rule set' });
    expect(within(child).queryByRole('combobox', { name: 'Traffic exit' })).not.toBeInTheDocument();
    expect(within(child).queryByRole('checkbox', { name: 'Enabled' })).not.toBeInTheDocument();
    await user.click(within(child).getByRole('button', { name: 'Done' }));
    expect(within(child).getByLabelText('Source format')).toHaveAttribute('aria-invalid', 'true');
    await user.type(within(child).getByLabelText('Name'), 'Domains');
    await user.type(
      within(child).getByLabelText('Link'),
      'https://raw.githubusercontent.com/example/rules/main/list.srs',
    );
    await user.click(within(child).getByRole('button', { name: 'Accelerate GitHub file' }));
    expect(within(child).getByLabelText('Link')).toHaveValue(
      'https://gh-proxy.com/https://raw.githubusercontent.com/example/rules/main/list.srs',
    );
    await user.click(within(child).getByRole('combobox', { name: 'Source format' }));
    await user.click(await screen.findByRole('option', { name: 'SRS (binary)' }));
    await user.click(within(child).getByRole('button', { name: 'Done' }));
    await waitFor(() =>
      expect(screen.queryByRole('dialog', { name: 'Edit rule set' })).not.toBeInTheDocument(),
    );
    expect(screen.getByRole('region', { name: 'Edit strategy group' })).toBeInTheDocument();
    expect(within(parent).getByText('Domains')).toBeInTheDocument();
  });
  it('requires native Loon rule format after a switch and drops inapplicable source options', async () => {
    const user = userEvent.setup();
    const group = { ...newRuleGroup([node.id]), name: 'Selected', rules: [{
      id: 'remote', enabled: true, kind: 'remote' as const, exit: { kind: 'group-default' as const },
      remote: {
        name: 'Domains', url: 'https://example.com/rules.txt', format: 'text' as const,
        behavior: 'domain' as const, accelerated: false, update_interval: 3600,
      },
    }] };
    const client = mount({ ...channel, format: 'loon', config: { policy: { ...channel.config.policy!, groups: [group] } } });
    expect(screen.getByRole('button', { name: /Save changes/ })).toBeDisabled();
    await user.click(screen.getByRole('tab', { name: 'Exit rules' }));
    await user.click(screen.getByRole('button', { name: 'Edit rule' }));
    const dialog = screen.getByRole('dialog', { name: 'Edit rule set' });
    expect(within(dialog).queryByLabelText('Update interval (seconds)')).not.toBeInTheDocument();
    await user.click(within(dialog).getByRole('button', { name: 'Done' }));
    expect(within(dialog).getByLabelText('Source format')).toHaveAttribute('aria-invalid', 'true');
    await user.click(within(dialog).getByRole('combobox', { name: 'Source format' }));
    await user.click(await screen.findByRole('option', { name: 'Loon' }));
    await user.click(within(dialog).getByRole('button', { name: 'Done' }));
    await user.click(screen.getByRole('button', { name: /Save changes/ }));
    await waitFor(() => expect(client.updateSubscriptionChannel).toHaveBeenCalledOnce());
    const saved = vi.mocked(client.updateSubscriptionChannel).mock.calls[0][1].config?.policy;
    expect(saved?.groups[0].rules[0].remote).toEqual({
      name: 'Domains', url: 'https://example.com/rules.txt', format: 'loon', accelerated: false,
      behavior: undefined, update_interval: undefined,
    });
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
});
