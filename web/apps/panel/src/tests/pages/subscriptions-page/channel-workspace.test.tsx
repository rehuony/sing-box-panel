import { useState } from 'react';
import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import { toast } from '@shared/components/ui/toast-manager';
import { render, screen, waitFor, within } from '@testing-library/react';

import '@/i18n';
import type { SubscriptionChannel, SubscriptionNode } from '@/api/api-client';

import { ApiClientProvider } from '@/api/api-client-context';
import { TestRouter as MemoryRouter } from '@/tests/test-router';
import { newRuleGroup } from '@/pages/subscriptions-page/channel-policy';
import { ChannelWorkspace } from '@/pages/subscriptions-page/channel-workspace';
import { createMockApiClient, testSubscriptionChannels } from '@/tests/api/mock-api-client';

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
  config: { groups: [], default_exit: { kind: 'direct' } },
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
  const selected = sidebar.querySelector('[data-group-item][aria-pressed="true"]')!;
  await user.click(within(selected.parentElement!).getByRole('button', { name: /^Configure / }));
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
    expect(screen.queryByText('Not exported')).not.toBeInTheDocument();
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

  it('identifies skipped nodes with actionable details and a fallback for unknown codes', async () => {
    const user = userEvent.setup();
    const client = mount();
    const preview = await client.previewSubscriptionChannel(channel.id, '');
    preview.result.diagnostics = [
      { collection: 'outbounds', item_index: 1, code: 'unsupported_tls', format: 'loon', node_id: 'node-anytls', node_name: 'Tokyo AnyTLS', node_type: 'anytls', field_path: 'outbounds[1].tls.fragment', reason: 'unsupported_tls_option' },
      { collection: 'inbounds', item_index: 3, code: 'future_reason', format: 'loon' },
    ];
    vi.mocked(client.previewSubscriptionChannel).mockResolvedValue(preview);
    await user.click(screen.getByRole('button', { name: 'Subscription preview' }));
    const dialog = await screen.findByRole('dialog', { name: 'Subscription preview' });
    expect(within(dialog).queryByText('Tokyo AnyTLS')).not.toBeInTheDocument();
    await user.click(within(dialog).getByRole('button', { name: 'Issues (2)' }));
    expect(await screen.findByText('Tokyo AnyTLS')).toBeVisible();
    expect(screen.getByText('The Loon exporter does not yet support this TLS option.')).toBeVisible();
    expect(screen.getByText(/Review the field below/)).toBeVisible();
    expect(screen.getByText('outbounds[1].tls.fragment')).toBeVisible();
    expect(screen.getByText('Node 4')).toBeVisible();
    expect(screen.getByText(/This node could not be exported/)).toBeVisible();
    expect(screen.getByText('future_reason')).toBeVisible();
    await user.keyboard('{Escape}');
    expect(screen.queryByText('Tokyo AnyTLS')).not.toBeInTheDocument();
    expect(dialog).toBeVisible();
  });
  it('submits confirmed channel settings through the save action', async () => {
    const user = userEvent.setup();
    const client = mount();
    await user.click(screen.getByRole('button', { name: 'Channel settings' }));
    await user.click(screen.getByRole('combobox', { name: 'Client' }));
    await user.click(await screen.findByRole('option', { name: 'Mihomo' }));
    await user.click(screen.getByRole('button', { name: 'Apply' }));
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
    expect(screen.getByRole('button', { name: 'Apply' })).toBeDisabled();
    await user.type(screen.getByLabelText('Group name'), 'Cancelled');
    await user.click(screen.getByRole('button', { name: 'Cancel' }));
    await user.click(screen.getByRole('button', { name: 'Configure Strategy group 1' }));
    expect(screen.getByLabelText('Group name')).toHaveValue('Strategy group 1');
    await user.clear(screen.getByLabelText('Group name'));
    await user.type(screen.getByLabelText('Group name'), '  Work  ');
    await user.click(screen.getByRole('button', { name: 'Apply' }));
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
    const group = { ...newRuleGroup([{ kind: 'node', id: node.id }]), name: 'Imported group', enabled: false };
    const client = mount({ ...channel, config: { ...channel.config, groups: [group] } });
    await openGroupSettings(user);
    await user.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(screen.getByRole('button', { name: /Save changes/ })).toBeDisabled();
    expect(screen.getByRole('button', { name: /^Imported group/ })).toHaveTextContent('Disabled');
    await user.click(screen.getByRole('button', { name: 'Set as final exit' }));
    expect(screen.getByRole('button', { name: /^Imported group/ })).not.toHaveTextContent('Disabled');
    expect(screen.getByRole('button', { name: /^Imported group/ })).toHaveTextContent('Final exit');
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
    await user.click(await screen.findByRole('option', { name: 'Automatic latency test' }));
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
    await user.click(await screen.findByRole('option', { name: 'Fallback' }));
    expect(screen.queryByLabelText('Tolerance (ms)')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(screen.getByRole('button', { name: /Save changes/ })).toBeDisabled();
    await openGroupSettings(user);
    await user.click(screen.getByRole('combobox', { name: 'Group type' }));
    await user.click(await screen.findByRole('option', { name: 'Fallback' }));
    await user.click(screen.getByRole('button', { name: 'Apply' }));
    await openGroupSettings(user);
    await user.click(screen.getByRole('combobox', { name: 'Group type' }));
    await user.click(await screen.findByRole('option', { name: 'Manual selection' }));
    expect(screen.queryByLabelText('Test link')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Apply' }));
    await user.click(screen.getByRole('button', { name: /Save changes/ }));
    await waitFor(() => expect(client.updateSubscriptionChannel).toHaveBeenCalledTimes(2));
    const manualGroup = vi.mocked(client.updateSubscriptionChannel).mock.calls[1][1].config?.groups[0];
    expect(manualGroup?.health_check).toBeUndefined();
  });
  it('keeps the parent strategy group open while adding a remote reference and blocks an empty format', async () => {
    const user = userEvent.setup();
    const group = { ...newRuleGroup([{ kind: 'node', id: node.id }]), name: 'Proxy' };
    mount({ ...channel, config: { ...channel.config, groups: [group] } });
    const parent = screen.getByRole('region', { name: 'Edit strategy group' });
    await user.click(within(parent).getByRole('tab', { name: 'Exit rules' }));
    await user.click(within(parent).getByRole('button', { name: 'Add rule set' }));
    const child = screen.getByRole('dialog', { name: 'Edit rule set' });
    expect(within(child).queryByRole('combobox', { name: 'Traffic exit' })).not.toBeInTheDocument();
    expect(within(child).queryByRole('checkbox', { name: 'Enabled' })).not.toBeInTheDocument();
    await user.click(within(child).getByRole('button', { name: 'Apply' }));
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
    await user.click(within(child).getByRole('button', { name: 'Apply' }));
    await waitFor(() =>
      expect(screen.queryByRole('dialog', { name: 'Edit rule set' })).not.toBeInTheDocument(),
    );
    expect(screen.getByRole('region', { name: 'Edit strategy group' })).toBeInTheDocument();
    expect(within(parent).getByText('Domains')).toBeInTheDocument();
  });
  it('requires native Loon rule format after a switch and drops inapplicable source options', async () => {
    const user = userEvent.setup();
    const group = { ...newRuleGroup([{ kind: 'node', id: node.id }]), name: 'Selected', rules: [{
      id: 'remote', enabled: true, kind: 'remote' as const, exit: { kind: 'group-default' as const },
      remote: {
        name: 'Domains', url: 'https://example.com/rules.txt', format: 'text' as const,
        behavior: 'domain' as const, accelerated: false,
      },
    }] };
    const client = mount({ ...channel, format: 'loon', config: { ...channel.config, groups: [group] } });
    expect(screen.getByRole('button', { name: /Save changes/ })).toBeDisabled();
    await user.click(screen.getByRole('tab', { name: 'Exit rules' }));
    await user.click(screen.getByRole('button', { name: 'Edit rule' }));
    const dialog = screen.getByRole('dialog', { name: 'Edit rule set' });
    expect(within(dialog).queryByLabelText('Update interval (seconds)')).not.toBeInTheDocument();
    await user.click(within(dialog).getByRole('button', { name: 'Apply' }));
    expect(within(dialog).getByLabelText('Source format')).toHaveAttribute('aria-invalid', 'true');
    await user.click(within(dialog).getByRole('combobox', { name: 'Source format' }));
    await user.click(await screen.findByRole('option', { name: 'Loon' }));
    await user.click(within(dialog).getByRole('button', { name: 'Apply' }));
    await user.click(screen.getByRole('button', { name: /Save changes/ }));
    await waitFor(() => expect(client.updateSubscriptionChannel).toHaveBeenCalledOnce());
    const saved = vi.mocked(client.updateSubscriptionChannel).mock.calls[0][1].config;
    expect(saved?.groups[0].rules[0].remote).toEqual({
      name: 'Domains', url: 'https://example.com/rules.txt', format: 'loon', accelerated: false,
      behavior: undefined,
    });
  });
  it.each([false, true])('preserves disabled state and explicit exits when changing rule priority (remote: %s)', async (remote) => {
    const user = userEvent.setup();
    const rule = {
      id: 'stored-rule', enabled: false, exit: { kind: 'node' as const, id: node.id },
      ...(remote
        ? {
            kind: 'remote' as const,
            remote: { name: 'Domains', url: 'https://example.com/rules.srs', format: 'binary' as const, accelerated: false },
          }
        : { kind: 'domain' as const, value: 'example.com' }),
    };
    const group = { ...newRuleGroup([{ kind: 'node', id: node.id }]), name: 'Proxy', rules: [rule] };
    const client = mount({ ...channel, config: { ...channel.config, groups: [group] } });
    await user.click(screen.getByRole('tab', { name: 'Exit rules' }));
    const rules = screen.getByRole('list', { name: 'Exit rules' });
    expect(within(rules).queryByRole('checkbox')).not.toBeInTheDocument();
    expect(within(rules).getByText('Disabled')).toBeVisible();
    await user.click(screen.getByRole('button', { name: 'Edit rule' }));
    const dialog = screen.getByRole('dialog');
    expect(within(dialog).queryByLabelText('Traffic exit')).not.toBeInTheDocument();
    expect(within(dialog).queryByLabelText('Enabled')).not.toBeInTheDocument();
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }));
    expect(screen.getByRole('button', { name: /Save changes/ })).toBeDisabled();
    await user.click(screen.getByRole('button', { name: 'Edit rule' }));
    const index = within(screen.getByRole('dialog')).getByRole('textbox', { name: 'Sort index' });
    await user.clear(index);
    await user.type(index, remote ? '-20' : '0');
    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Apply' }));
    await user.click(screen.getByRole('button', { name: /Save changes/ }));
    await waitFor(() => expect(client.updateSubscriptionChannel).toHaveBeenCalledOnce());
    const saved = vi.mocked(client.updateSubscriptionChannel).mock.calls[0][1].config;
    expect(saved?.groups[0].rules[0]).toMatchObject({
      ...rule, sort_index: remote ? -20 : 0,
    });
  });
  it('shares the active tab across groups while showing each group’s own content', async () => {
    const user = userEvent.setup();
    const otherNode = { ...node, id: 'node-two', display_name: 'Osaka', origin_name: 'Osaka' };
    const groups = [node, otherNode].map((value) => ({
      ...newRuleGroup([{ kind: 'node', id: value.id }]), name: value.display_name,
      rules: [{
        id: `rule-${value.id}`, kind: 'domain' as const, value: `${value.display_name.toLowerCase()}.example`,
        enabled: true, exit: { kind: 'group-default' as const },
      }],
    }));
    const client = mount({ ...channel, config: { ...channel.config, groups } }, [node, otherNode]);
    const sidebar = screen.getByRole('complementary', { name: 'Strategy groups' });
    await user.click(screen.getByRole('tab', { name: 'Exit rules' }));
    await user.click(within(sidebar).getByRole('button', { name: /^Osaka/ }));
    expect(screen.getByRole('tab', { name: 'Exit rules' })).toHaveAttribute('aria-selected', 'true');
    expect(screen.getByRole('list', { name: 'Exit rules' })).toHaveTextContent('osaka.example');
    expect(screen.queryByText('tokyo.example')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Add rule' })).toBeVisible();
    await user.click(within(sidebar).getByRole('button', { name: /^Tokyo/ }));
    expect(screen.getByRole('list', { name: 'Exit rules' })).toHaveTextContent('tokyo.example');
    await user.click(screen.getByRole('tab', { name: 'Candidate nodes' }));
    await user.click(within(sidebar).getByRole('button', { name: /^Osaka/ }));
    expect(screen.getByRole('tab', { name: 'Candidate nodes' })).toHaveAttribute('aria-selected', 'true');
    expect(screen.getByRole('checkbox', { name: /Osaka/ })).toBeVisible();
    expect(screen.queryByRole('checkbox', { name: /Tokyo/ })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Add nodes' })).toBeVisible();
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
    await user.click(screen.getByRole('checkbox', { name: /Tokyo/ }));
    expect(screen.getByRole('checkbox', { name: /Tokyo/ })).toBeChecked();
    await user.type(screen.getByRole('textbox', { name: 'Search nodes, groups, sources or protocols' }), 'Tokyo');
    const sidebar = screen.getByRole('complementary', { name: 'Strategy groups' });
    await user.click(within(sidebar).getByRole('button', { name: /^Other/ }));
    expect(screen.getByRole('textbox', { name: 'Search nodes, groups, sources or protocols' })).toHaveValue('');
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
    expect(vi.mocked(client.updateSubscriptionChannel).mock.calls[0][1].config?.default_exit).toEqual({ kind: 'direct' });
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
    const saved = vi.mocked(client.updateSubscriptionChannel).mock.calls[1][1].config;
    expect(saved?.default_exit).toEqual({ kind: 'group', id: saved?.groups[1].id });
    await user.click(screen.getByRole('button', { name: /^Strategy group 1/ }));
    await openGroupSettings(user);
    await user.type(screen.getByLabelText('Group name'), ' renamed');
    await user.click(screen.getByRole('button', { name: 'Apply' }));
    expect(screen.getByRole('button', { name: /^Strategy group 2/ })).toHaveTextContent('Final exit');
    await user.click(screen.getByRole('button', { name: /^Strategy group 2/ }));
    await user.click(screen.getByRole('button', { name: 'Clear final exit' }));
    await user.click(screen.getByRole('button', { name: /Save changes/ }));
    await waitFor(() => expect(client.updateSubscriptionChannel).toHaveBeenCalledTimes(3));
    expect(vi.mocked(client.updateSubscriptionChannel).mock.calls[2][1].config?.default_exit).toEqual({ kind: 'direct' });
    await user.click(screen.getByRole('button', { name: 'Delete strategy group' }));
    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Delete strategy group' }));
    expect(screen.queryByRole('button', { name: /^Strategy group 2/ })).not.toBeInTheDocument();
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

it('infers source formats on link edits and preserves manual choices through acceleration and reopen', async () => {
  const user = userEvent.setup();
  const group = { ...newRuleGroup([]), name: 'Proxy', rules: [{
    id: 'remote', kind: 'remote' as const, enabled: true, exit: { kind: 'group-default' as const },
    remote: { name: 'Rules', url: 'https://raw.githubusercontent.com/a/r/main/rules.srs', format: 'source' as const, accelerated: false },
  }] };
  mount({ ...channel, config: { ...channel.config, groups: [group] } });
  await user.click(screen.getByRole('tab', { name: 'Exit rules' }));
  await user.click(screen.getByRole('button', { name: 'Edit rule' }));
  const dialog = screen.getByRole('dialog');
  const source = within(dialog).getByRole('combobox', { name: 'Source format' });
  expect(source).toHaveTextContent('JSON (source)');
  await user.click(within(dialog).getByRole('button', { name: 'Accelerate GitHub file' }));
  expect(source).toHaveTextContent('JSON (source)');
  const url = within(dialog).getByLabelText('Link');
  await user.clear(url);
  await user.type(url, 'https://raw.githubusercontent.com/a/r/main/other.SRS?raw=1');
  expect(source).toHaveTextContent('SRS (binary)');
  await user.click(source);
  await user.click(await screen.findByRole('option', { name: 'JSON (source)' }));
  await user.click(within(dialog).getByRole('button', { name: 'Apply' }));
  await user.click(screen.getByRole('button', { name: 'Edit rule' }));
  expect(within(screen.getByRole('dialog')).getByRole('combobox', { name: 'Source format' })).toHaveTextContent('JSON (source)');
});

it('rejects a rule-set name already used in a disabled group', async () => {
  const user = userEvent.setup();
  const rule = { id: 'remote', kind: 'remote' as const, enabled: false, exit: { kind: 'group-default' as const },
    remote: { name: 'Taken', url: 'https://rules.example/rules.srs', format: 'binary' as const, accelerated: false } };
  const groups = [
    { ...newRuleGroup([]), name: 'Editing', rules: [{ ...rule, remote: { ...rule.remote, name: 'Free' } }] },
    { ...newRuleGroup([]), name: 'Disabled', enabled: false, rules: [rule] },
  ];
  mount({ ...channel, config: { ...channel.config, groups } });
  await user.click(screen.getByRole('tab', { name: 'Exit rules' }));
  await user.click(screen.getByRole('button', { name: 'Edit rule' }));
  const dialog = screen.getByRole('dialog');
  const name = within(dialog).getByLabelText('Name');
  await user.clear(name);
  await user.type(name, ' Taken ');
  expect(name).toHaveAttribute('aria-invalid', 'true');
  await user.click(within(dialog).getByRole('button', { name: 'Apply' }));
  expect(dialog).toBeInTheDocument();
  await user.clear(name);
  await user.type(name, 'Unique');
  await user.click(within(dialog).getByRole('button', { name: 'Apply' }));
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
});

it('removes references to a deleted strategy group in the saved policy', async () => {
  const user = userEvent.setup();
  const child = { ...newRuleGroup([{ kind: 'node', id: node.id }]), id: 'child', name: 'Child' };
  const parent = { ...newRuleGroup([{ kind: 'group', id: 'child' }]), id: 'parent', name: 'Parent' };
  const client = mount({ ...channel, config: { ...channel.config, groups: [child, parent] } });
  await user.click(screen.getByRole('button', { name: 'Delete strategy group' }));
  await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Delete strategy group' }));
  await user.click(screen.getByRole('button', { name: /Save changes/ }));
  await waitFor(() => expect(client.updateSubscriptionChannel).toHaveBeenCalledOnce());
  expect(vi.mocked(client.updateSubscriptionChannel).mock.calls[0][1].config?.groups).toEqual([
    { ...parent, members: [] },
  ]);
});

it('edits rules as text, keeping each rule’s identity and holding invalid text back from saving', async () => {
  const user = userEvent.setup();
  const add = vi.spyOn(toast, 'add');
  const groups = [{
    ...newRuleGroup([{ kind: 'node', id: node.id }]), id: 'group-one', name: 'Tokyo',
    rules: [{
      id: 'rule-a', kind: 'domain_suffix' as const, value: 'example.com', enabled: true, sort_index: 10,
      exit: { kind: 'node' as const, id: node.id },
    }],
  }];
  const client = mount({ ...channel, config: { ...channel.config, groups } });
  await user.click(screen.getByRole('tab', { name: 'Exit rules' }));
  await user.click(screen.getByRole('tab', { name: 'Text' }));
  const text = screen.getByRole('textbox', { name: 'Rules as text, one per line' });
  expect(text).toHaveValue('DOMAIN-SUFFIX,example.com');
  // Retyping a letter passes through text without the rule; it keeps its exit.
  await user.type(text, '{Backspace}m{Enter}DOMAIN,api.example.com{Enter}GEOIP,CN');
  expect(screen.getByRole('list', { name: 'Problems in the rule text' })).toHaveTextContent(/Line 3.*Unsupported match type GEOIP/);
  await user.click(screen.getByRole('tab', { name: 'Entries' }));
  expect(screen.getByRole('tab', { name: 'Text' })).toHaveAttribute('aria-selected', 'true');
  await user.click(screen.getByRole('button', { name: /Save changes/ }));
  expect(client.updateSubscriptionChannel).not.toHaveBeenCalled();
  expect(add).toHaveBeenCalledWith(expect.objectContaining({ title: 'Fix the rule text of “Tokyo” before saving.' }));
  expect(text).toHaveValue('DOMAIN-SUFFIX,example.com\nDOMAIN,api.example.com\nGEOIP,CN');

  await user.type(text, '{Backspace>9}');
  await user.click(screen.getByRole('tab', { name: 'Entries' }));
  expect(screen.getByRole('list', { name: 'Exit rules' })).toHaveTextContent('api.example.com');
  await user.click(screen.getByRole('button', { name: /Save changes/ }));
  await waitFor(() => expect(client.updateSubscriptionChannel).toHaveBeenCalledOnce());
  const saved = vi.mocked(client.updateSubscriptionChannel).mock.calls[0][1].config?.groups[0].rules;
  expect(saved?.map(rule => [rule.id === 'rule-a', rule.kind, rule.value, rule.sort_index, rule.exit])).toEqual([
    [true, 'domain_suffix', 'example.com', 10, { kind: 'node', id: node.id }],
    [false, 'domain', 'api.example.com', 20, { kind: 'group-default' }],
  ]);
});
