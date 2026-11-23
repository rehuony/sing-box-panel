import { useState } from 'react';
import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import { toast } from '@shared/components/ui/toast-manager';
import { render, screen, waitFor, within } from '@testing-library/react';

import '@/i18n';
import type { SubscriptionChannel, SubscriptionNode } from '@/api/api-client';

import { ApiClientProvider } from '@/api/api-client-context';
import { TestRouter as MemoryRouter } from '@/tests/test-router';
import { ChannelWorkspace } from '@/pages/subscriptions-page/channel-workspace';
import { createMockApiClient, testSubscriptionChannels } from '@/tests/api/mock-api-client';
import { emptyChannelConfig, newRuleGroup } from '@/pages/subscriptions-page/channel-policy';

const node: SubscriptionNode = {
  id: 'node-one',
  display_name: 'Tokyo',
  origin_name: 'Tokyo',
  protocol: 'socks',
  origin: { kind: 'source', source_id: 'source_file', source_name: 'Operator additions' },
  server: 'node.example.com',
  port: 1080,
  updated_at: '2026-09-19T09:00:00Z',
};
const channel: SubscriptionChannel = {
  ...testSubscriptionChannels[0],
  format: 'sing-box',
  config: emptyChannelConfig(),
};
function mount(
  value = channel,
  nodes = [node],
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
      <ChannelWorkspace channel={saved} nodes={nodes} onBack={vi.fn()} onDelete={vi.fn()} onSaved={setSaved} />
    );
  }
  render(
    <MemoryRouter>
      <ApiClientProvider client={client}><Workspace /></ApiClientProvider>
    </MemoryRouter>,
  );
  return client;
}
const rail = () => screen.getByRole('navigation', { name: 'Channel setup' });
async function openGroupSettings(user: ReturnType<typeof userEvent.setup>) {
  if (screen.queryByRole('dialog', { name: 'Strategy group settings' })) return;
  const selected = rail().querySelector('[aria-current="true"]')!;
  await user.click(within(selected.parentElement!).getByRole('button', { name: /^Configure / }));
}
async function chooseExit(user: ReturnType<typeof userEvent.setup>, name: string | RegExp, option: string) {
  await user.click(screen.getByRole('combobox', { name }));
  await user.click(await screen.findByRole('option', { name: option }));
}
describe('channel workspace', () => {
  it('shows preview failures only through the title issues popover', async () => {
    const user = userEvent.setup();
    const client = mount();
    vi.mocked(client.previewSubscriptionChannel).mockRejectedValueOnce(new Error('Preview failed'));
    await user.click(screen.getByRole('button', { name: 'Subscription preview' }));
    const dialog = await screen.findByRole('dialog', { name: 'Subscription preview' });
    expect(within(dialog).queryByText('Preview failed')).not.toBeInTheDocument();
    expect(within(dialog).getByRole('button', { name: 'Copy' })).toBeDisabled();
    await user.click(within(dialog).getByRole('button', { name: 'Issues (1)' }));
    expect(await screen.findByText('Preview failed')).toBeVisible();
    expect(within(screen.getByRole('dialog', { name: 'Issues (1)' })).getByText('Subscription preview failed')).toBeVisible();
    await user.keyboard('{Escape}');
    await user.keyboard('{Escape}');
    await user.click(screen.getByRole('button', { name: 'Subscription preview' }));
    const success = await screen.findByRole('dialog', { name: 'Subscription preview' });
    expect(within(success).queryByRole('button', { name: /Issues/ })).not.toBeInTheDocument();
    expect(within(success).getByRole('button', { name: 'Copy' })).toBeDisabled();
  });
  it('copies preview content and reports clipboard success and failure', async () => {
    const user = userEvent.setup();
    const client = mount();
    const preview = await client.previewSubscriptionChannel(channel.id, '');
    preview.result.content = 'rules:\n  - MATCH,DIRECT';
    vi.mocked(client.previewSubscriptionChannel).mockResolvedValue(preview);
    const feedback = vi.spyOn(toast, 'add');
    await user.click(screen.getByRole('button', { name: 'Subscription preview' }));
    const dialog = await screen.findByRole('dialog', { name: 'Subscription preview' });
    await user.click(within(dialog).getByRole('button', { name: 'Copy' }));
    expect(await navigator.clipboard.readText()).toBe(preview.result.content);
    expect(feedback).toHaveBeenLastCalledWith(expect.objectContaining({ type: 'success' }));
    vi.spyOn(navigator.clipboard, 'writeText').mockRejectedValueOnce(new Error('Clipboard denied'));
    await user.click(within(dialog).getByRole('button', { name: 'Copy' }));
    expect(feedback).toHaveBeenLastCalledWith({ title: 'Clipboard denied', type: 'error' });
  });

  it('groups skipped nodes by code, naming them from the pool, with a fallback for unknown codes', async () => {
    const user = userEvent.setup();
    const client = mount();
    const preview = await client.previewSubscriptionChannel(channel.id, '');
    preview.result.diagnostics = [
      { collection: 'outbounds', item_index: 1, code: 'unsupported_tls', format: 'loon', node_id: 'node-anytls', node_name: 'Tokyo AnyTLS', node_type: 'anytls', field_path: 'outbounds[1].tls.fragment', reason: 'unsupported_tls_option' },
      { collection: 'outbounds', item_index: 2, code: 'unsupported_tls', format: 'loon', node_id: node.id, node_name: 'published name', reason: 'unsupported_tls_option' },
      { collection: 'inbounds', item_index: 3, code: 'future_reason', format: 'loon' },
    ];
    vi.mocked(client.previewSubscriptionChannel).mockResolvedValue(preview);
    await user.click(screen.getByRole('button', { name: 'Subscription preview' }));
    const dialog = await screen.findByRole('dialog', { name: 'Subscription preview' });
    expect(within(dialog).queryByText('Tokyo AnyTLS, Tokyo')).not.toBeInTheDocument();
    await user.click(within(dialog).getByRole('button', { name: 'Issues (3)' }));
    expect(await screen.findByText('Tokyo AnyTLS, Tokyo')).toBeVisible();
    expect(screen.getAllByText('The loon exporter does not yet support this TLS option.')).toHaveLength(1);
    expect(screen.getByText(/Review the field below/)).toBeVisible();
    expect(screen.getByText('Node 4')).toBeVisible();
    expect(screen.getByText(/This node could not be exported/)).toBeVisible();
    expect(screen.getByText('future_reason')).toBeVisible();
    await user.keyboard('{Escape}');
    expect(screen.queryByText('Tokyo AnyTLS, Tokyo')).not.toBeInTheDocument();
    expect(dialog).toBeVisible();
  });
  it('submits confirmed channel settings through the save action', async () => {
    const user = userEvent.setup();
    const client = mount();
    expect(screen.getByText(channel.name)).toBeVisible();
    expect(screen.getByText('sing-box')).toBeVisible();
    await user.click(screen.getByRole('button', { name: 'Channel settings' }));
    await user.click(screen.getByRole('combobox', { name: 'Client' }));
    await user.click(await screen.findByRole('option', { name: 'mihomo' }));
    await user.click(screen.getByRole('button', { name: 'Apply' }));
    expect(client.updateSubscriptionChannel).not.toHaveBeenCalled();
    await user.click(screen.getByRole('button', { name: /Save changes/ }));
    await waitFor(() => expect(client.updateSubscriptionChannel).toHaveBeenCalledWith(
      channel.id, expect.objectContaining({ format: 'mihomo' }), channel.updated_at, expect.any(AbortSignal),
    ));
  });
  it('shows rule set rows over the final exit, flagging a set the client lacks', async () => {
    const proxy = { ...newRuleGroup([{ kind: 'node', id: node.id }]), name: 'Proxy' };
    mount({ ...channel, format: 'mihomo', config: { ...channel.config, groups: [proxy], routes: [
      { id: 'streaming', enabled: true, rule_set_id: 'rule_set_streaming', exit: { kind: 'group', id: proxy.id } },
      { id: 'other', enabled: true, rule_set_id: 'rule_set_direct', exit: { kind: 'direct' } },
    ] } });
    const routes = screen.getByRole('list', { name: 'Exit rules' });
    await within(routes).findByText('Streaming');
    const rows = within(routes).getAllByRole('listitem');
    expect(rows[0]).toHaveTextContent('Rule set · 2 rules');
    expect(within(rows[0]).getByRole('combobox', { name: 'Exit of rule 1' })).toHaveTextContent('Proxy');
    expect(rows[1]).toHaveTextContent('No matching mihomo rule set');
    expect(screen.getByRole('combobox', { name: 'Final exit' })).toHaveTextContent('DIRECT');
    expect(within(rail()).getByRole('button', { name: /^Exit rules/ })).toHaveTextContent('Final exit DIRECT');
    expect(within(rail()).getByRole('button', { name: /^Proxy/ })).toHaveTextContent('select · 1 rule');
    expect(screen.getByRole('button', { name: /Save changes/ })).toBeDisabled();
  });
  it('adds a rule set once, leaving through the first group', async () => {
    const user = userEvent.setup();
    const proxy = { ...newRuleGroup([{ kind: 'node', id: node.id }]), name: 'Proxy' };
    const client = mount({ ...channel, config: { ...channel.config, groups: [proxy] } });
    expect(screen.getByText('No exit rules yet')).toBeVisible();
    await user.click(screen.getByRole('button', { name: 'Add rule set' }));
    await user.click(await screen.findByRole('menuitem', { name: /^Direct/ }));
    expect(screen.getByRole('list', { name: 'Exit rules' })).toHaveTextContent('Rule set · 1 rule');
    await user.click(screen.getByRole('button', { name: 'Add rule set' }));
    expect(await screen.findByRole('menuitem', { name: /^Direct/ })).toHaveAttribute('aria-disabled', 'true');
    await user.keyboard('{Escape}');
    await chooseExit(user, 'Final exit', 'REJECT');
    await user.click(screen.getByRole('button', { name: /Save changes/ }));
    await waitFor(() => expect(client.updateSubscriptionChannel).toHaveBeenCalledOnce());
    const saved = vi.mocked(client.updateSubscriptionChannel).mock.calls[0][1].config;
    expect(saved?.routes).toEqual([{ id: expect.any(String), enabled: true, rule_set_id: 'rule_set_direct', exit: { kind: 'group', id: proxy.id } }]);
    expect(saved?.default_exit).toEqual({ kind: 'reject' });
  });
  it('renames directly from an unselected group and discards cancelled edits', async () => {
    const user = userEvent.setup();
    const client = mount();
    await user.click(screen.getByRole('button', { name: 'Add strategy group' }));
    await user.click(screen.getByRole('button', { name: 'Add strategy group' }));
    await user.click(screen.getByRole('button', { name: 'Configure Strategy group 1' }));
    expect(screen.getByLabelText('Group name')).toHaveValue('Strategy group 1');
    await user.clear(screen.getByLabelText('Group name'));
    expect(screen.getByRole('button', { name: 'Apply' })).toBeDisabled();
    await user.type(screen.getByLabelText('Group name'), 'Cancelled');
    await user.click(screen.getByRole('button', { name: 'Cancel' }));
    await user.click(screen.getByRole('button', { name: 'Configure Strategy group 1' }));
    expect(screen.getByLabelText('Group name')).toHaveValue('Strategy group 1');
    await user.clear(screen.getByLabelText('Group name'));
    await user.type(screen.getByLabelText('Group name'), '  Work  ');
    await user.click(screen.getByRole('button', { name: 'Apply' }));
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(within(rail()).getByRole('button', { name: /^Work/ })).not.toHaveAttribute('aria-current');
    expect(within(rail()).getByRole('button', { name: /^Strategy group 2/ })).toHaveAttribute('aria-current', 'true');
    await user.click(screen.getByRole('button', { name: 'Configure Work' }));
    await user.type(screen.getByLabelText('Group name'), ' discarded');
    await user.keyboard('{Escape}');
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(within(rail()).getByRole('button', { name: /^Work/ })).not.toHaveTextContent('discarded');
    expect(client.updateSubscriptionChannel).not.toHaveBeenCalled();
  });
  it('preserves stored disabled groups on cancel and offers them as exits once applied', async () => {
    const user = userEvent.setup();
    const group = { ...newRuleGroup([{ kind: 'node', id: node.id }]), name: 'Imported group', enabled: false };
    const client = mount({ ...channel, config: { ...channel.config, groups: [group] } });
    await user.click(within(rail()).getByRole('button', { name: /^Imported group/ }));
    await openGroupSettings(user);
    await user.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(screen.getByRole('button', { name: /Save changes/ })).toBeDisabled();
    expect(within(rail()).getByRole('button', { name: /^Imported group/ })).toHaveTextContent('Disabled');
    await openGroupSettings(user);
    await user.click(screen.getByRole('button', { name: 'Apply' }));
    expect(within(rail()).getByRole('button', { name: /^Imported group/ })).not.toHaveTextContent('Disabled');
    await user.click(within(rail()).getByRole('button', { name: /^Exit rules/ }));
    await chooseExit(user, 'Final exit', 'Imported group');
    await user.click(screen.getByRole('button', { name: /Save changes/ }));
    await waitFor(() => expect(client.updateSubscriptionChannel).toHaveBeenCalled());
    const saved = vi.mocked(client.updateSubscriptionChannel).mock.calls[0][1].config;
    expect(saved?.groups[0]).toMatchObject({ id: group.id, enabled: true });
    expect(saved?.default_exit).toEqual({ kind: 'group', id: group.id });
  });
  it('stages strategy types and health checks until confirmation', async () => {
    const user = userEvent.setup();
    const client = mount({ ...channel, format: 'mihomo' });
    await user.click(screen.getByRole('button', { name: 'Add strategy group' }));
    await openGroupSettings(user);
    await user.click(screen.getByRole('combobox', { name: 'Group type' }));
    await user.click(await screen.findByRole('option', { name: 'url-test' }));
    await user.clear(screen.getByLabelText('Test link'));
    await user.type(screen.getByLabelText('Test link'), 'https://probe.example/check');
    await user.clear(screen.getByLabelText('Interval (s)'));
    await user.type(screen.getByLabelText('Interval (s)'), '600');
    await user.clear(screen.getByLabelText('Tolerance (ms)'));
    await user.type(screen.getByLabelText('Tolerance (ms)'), '0');
    await user.click(screen.getByRole('button', { name: 'Apply' }));
    await user.click(screen.getByRole('button', { name: /Save changes/ }));
    await waitFor(() => expect(client.updateSubscriptionChannel).toHaveBeenCalledTimes(1));
    expect(vi.mocked(client.updateSubscriptionChannel).mock.calls[0][1].config?.groups[0]).toMatchObject({ type: 'url-test', members: [], health_check: { url: 'https://probe.example/check', interval: 600, tolerance: 0 } });
    await openGroupSettings(user);
    await user.click(screen.getByRole('combobox', { name: 'Group type' }));
    await user.click(await screen.findByRole('option', { name: 'fallback' }));
    expect(screen.queryByLabelText('Tolerance (ms)')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(screen.getByRole('button', { name: /Save changes/ })).toBeDisabled();
    await openGroupSettings(user);
    await user.click(screen.getByRole('combobox', { name: 'Group type' }));
    await user.click(await screen.findByRole('option', { name: 'fallback' }));
    await user.click(screen.getByRole('button', { name: 'Apply' }));
    await openGroupSettings(user);
    await user.click(screen.getByRole('combobox', { name: 'Group type' }));
    await user.click(await screen.findByRole('option', { name: 'select' }));
    expect(screen.queryByLabelText('Test link')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Apply' }));
    await user.click(screen.getByRole('button', { name: /Save changes/ }));
    await waitFor(() => expect(client.updateSubscriptionChannel).toHaveBeenCalledTimes(2));
    const manualGroup = vi.mocked(client.updateSubscriptionChannel).mock.calls[1][1].config?.groups[0];
    expect(manualGroup?.health_check).toBeUndefined();
  });
  it('lists the pool nodes a group’s match takes after its members', async () => {
    const user = userEvent.setup();
    const osaka = { ...node, id: 'node-two', display_name: 'Osaka', origin_name: 'Osaka' };
    const group = { ...newRuleGroup([{ kind: 'node', id: node.id }]), name: 'Proxy' };
    const client = mount({ ...channel, config: { ...channel.config, groups: [group] } }, [node, osaka]);
    await user.click(within(rail()).getByRole('button', { name: /^Proxy/ }));
    expect(screen.queryByRole('region', { name: /^Automatic match/ })).not.toBeInTheDocument();
    await openGroupSettings(user);
    await user.type(screen.getByLabelText('Include'), 'o{Enter}');
    await user.click(screen.getByRole('button', { name: 'Apply' }));
    const matched = screen.getByRole('region', { name: 'Automatic match · 1' });
    expect(within(matched).getByRole('listitem', { name: 'Osaka' })).toBeVisible();
    expect(screen.getByRole('checkbox', { name: 'Tokyo' })).toBeVisible();
    await user.click(screen.getByRole('button', { name: /Save changes/ }));
    await waitFor(() => expect(client.updateSubscriptionChannel).toHaveBeenCalledOnce());
    expect(vi.mocked(client.updateSubscriptionChannel).mock.calls[0][1].config?.groups[0].match)
      .toEqual({ include: ['o'], exclude: [], origins: [], protocols: [] });
  });
  it('shows each group’s own exit rules in their places in the whole list', async () => {
    const user = userEvent.setup();
    const otherNode = { ...node, id: 'node-two', display_name: 'Osaka', origin_name: 'Osaka' };
    const groups = [node, otherNode].map((value) => ({ ...newRuleGroup([{ kind: 'node', id: value.id }]), name: value.display_name }));
    const routes = groups.map(group => ({
      id: `rule-${group.id}`, enabled: true, exit: { kind: 'group' as const, id: group.id },
      rule: { kind: 'domain' as const, value: `${group.name.toLowerCase()}.example` },
    }));
    const client = mount({ ...channel, config: { ...channel.config, groups, routes } }, [node, otherNode]);
    await user.click(within(rail()).getByRole('button', { name: /^Osaka/ }));
    await user.click(screen.getByRole('tab', { name: 'Exit rules' }));
    const list = screen.getByRole('list', { name: 'Exit rules' });
    expect(list).toHaveTextContent('osaka.example');
    expect(list).not.toHaveTextContent('tokyo.example');
    expect(within(list).getByRole('combobox', { name: 'Exit of rule 2' })).toBeVisible();
    expect(screen.queryByRole('button', { name: /^Move rule/ })).not.toBeInTheDocument();
    expect(screen.queryByRole('combobox', { name: 'Final exit' })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Add custom rule' })).toBeVisible();
    await user.click(within(rail()).getByRole('button', { name: /^Tokyo/ }));
    expect(screen.getByRole('tab', { name: 'Exit rules' })).toHaveAttribute('aria-selected', 'true');
    expect(screen.getByRole('list', { name: 'Exit rules' })).toHaveTextContent('tokyo.example');
    await user.click(screen.getByRole('tab', { name: 'Candidate nodes' }));
    await user.click(within(rail()).getByRole('button', { name: /^Osaka/ }));
    expect(screen.getByRole('tab', { name: 'Candidate nodes' })).toHaveAttribute('aria-selected', 'true');
    expect(screen.getByRole('checkbox', { name: /Osaka/ })).toBeVisible();
    expect(screen.queryByRole('checkbox', { name: /Tokyo/ })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Save changes/ })).toBeDisabled();
    expect(client.updateSubscriptionChannel).not.toHaveBeenCalled();
  });
  it('resets card selection and search when switching groups without changing either membership', async () => {
    const user = userEvent.setup();
    const groups = [
      { ...newRuleGroup([{ kind: 'node', id: 'node-one' }]), name: 'Proxy' },
      { ...newRuleGroup([{ kind: 'node', id: 'node-one' }]), name: 'Other' },
    ];
    const client = mount({ ...channel, config: { ...channel.config, groups } });
    await user.click(within(rail()).getByRole('button', { name: /^Proxy/ }));
    await user.click(screen.getByRole('checkbox', { name: /Tokyo/ }));
    expect(screen.getByRole('checkbox', { name: /Tokyo/ })).toBeChecked();
    await user.type(screen.getByRole('textbox', { name: 'Search nodes, groups, sources or protocols' }), 'Tokyo');
    await user.click(within(rail()).getByRole('button', { name: /^Other/ }));
    expect(screen.getByRole('textbox', { name: 'Search nodes, groups, sources or protocols' })).toHaveValue('');
    expect(screen.getByRole('checkbox', { name: /Tokyo/ })).not.toBeChecked();
    expect(screen.queryByRole('button', { name: 'Remove selected nodes' })).not.toBeInTheDocument();
    await user.click(within(rail()).getByRole('button', { name: /^Proxy/ }));
    expect(screen.getByRole('checkbox', { name: /Tokyo/ })).not.toBeChecked();
    expect(screen.getByRole('button', { name: /Save changes/ })).toBeDisabled();
    expect(client.updateSubscriptionChannel).not.toHaveBeenCalled();
  });
  it('refuses to delete a group an exit uses and removes its references once free', async () => {
    const addToast = vi.spyOn(toast, 'add');
    const user = userEvent.setup();
    const child = { ...newRuleGroup([{ kind: 'node', id: node.id }]), id: 'child', name: 'Child' };
    const parent = { ...newRuleGroup([{ kind: 'group', id: 'child' }]), id: 'parent', name: 'Parent' };
    const client = mount({ ...channel, config: { ...channel.config, groups: [child, parent], default_exit: { kind: 'group', id: 'child' } } });
    await user.click(screen.getByRole('button', { name: 'More for Child' }));
    await user.click(await screen.findByRole('menuitem', { name: 'Delete' }));
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(addToast).toHaveBeenCalledWith(expect.objectContaining({ type: 'error' }));
    await chooseExit(user, 'Final exit', 'DIRECT');
    await user.click(screen.getByRole('button', { name: 'More for Child' }));
    await user.click(await screen.findByRole('menuitem', { name: 'Delete' }));
    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Delete strategy group' }));
    await user.click(screen.getByRole('button', { name: /Save changes/ }));
    await waitFor(() => expect(client.updateSubscriptionChannel).toHaveBeenCalledOnce());
    const saved = vi.mocked(client.updateSubscriptionChannel).mock.calls[0][1].config;
    expect(saved?.groups).toEqual([{ ...parent, members: [] }]);
    addToast.mockRestore();
  });
  it('keeps channel settings local and exposes no secondary node organization', async () => {
    const user = userEvent.setup();
    const client = mount();
    await user.click(screen.getByRole('button', { name: 'Channel settings' }));
    expect(screen.queryByRole('button', { name: 'Organize nodes' })).not.toBeInTheDocument();
    await user.clear(screen.getByLabelText('Channel name'));
    await user.type(screen.getByLabelText('Channel name'), 'Office');
    await user.click(screen.getByRole('button', { name: 'Apply' }));
    expect(client.updateSubscriptionChannel).not.toHaveBeenCalled();
    await user.click(screen.getByRole('button', { name: /Save changes/ }));
    await waitFor(() => expect(client.updateSubscriptionChannel).toHaveBeenCalled());
    const saved = vi.mocked(client.updateSubscriptionChannel).mock.calls[0][1];
    expect(saved.name).toBe('Office');
    expect(saved.config).not.toHaveProperty('organizer');
  });
});

it('rejects a remote rule set name another own rule uses', async () => {
  const user = userEvent.setup();
  const remote = (id: string, name: string) => ({ id, enabled: true, exit: { kind: 'direct' as const },
    rule: { kind: 'remote' as const, remote: { name, url: `https://rules.example/${id}.srs`, format: 'binary' as const, accelerated: false } } });
  mount({ ...channel, config: { ...channel.config, routes: [remote('free', 'Free'), remote('taken', 'Taken')] } });
  await user.click(within(screen.getByRole('list', { name: 'Exit rules' })).getAllByRole('button', { name: 'Edit' })[0]);
  const dialog = screen.getByRole('dialog');
  const name = within(dialog).getByLabelText('Name');
  await user.clear(name);
  await user.type(name, 'Taken');
  await user.click(within(dialog).getByRole('button', { name: 'Apply' }));
  expect(dialog).toBeInTheDocument();
  await user.clear(name);
  await user.type(name, 'Unique');
  await user.click(within(dialog).getByRole('button', { name: 'Apply' }));
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
  expect(screen.getByRole('list', { name: 'Exit rules' })).toHaveTextContent('Unique');
});
