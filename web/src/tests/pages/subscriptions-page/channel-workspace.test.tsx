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
  render(
    <MemoryRouter>
      <ApiClientProvider client={client}>
        <ChannelWorkspace
          channel={value}
          nodes={nodes}
          onBack={vi.fn()}
          onSaved={vi.fn()}
          onRefresh={async () => {}}
        />
      </ApiClientProvider>
    </MemoryRouter>,
  );
  return client;
}
describe('channel workspace', () => {
  it('does not offer edits that cannot be persisted for legacy Loon output', () => {
    mount({ ...channel, format: 'loon', config: {} });
    expect(screen.getByRole('checkbox', { name: 'Select Tokyo' })).toBeDisabled();
    expect(screen.getByRole('button', { name: /Save changes/ })).toBeDisabled();
  });
  it('excludes hidden cards and persists selection with CAS', async () => {
    const user = userEvent.setup();
    const client = mount();
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
  it('keeps the parent rule group open while adding a remote reference and blocks an empty format', async () => {
    const user = userEvent.setup();
    mount();
    await user.click(screen.getByRole('tab', { name: 'Subscription rules' }));
    await user.click(screen.getByRole('button', { name: 'Add rule group' }));
    const parent = screen.getByRole('dialog', { name: 'Edit rule group' });
    await user.type(within(parent).getByLabelText('Group name'), 'Proxy');
    expect(within(parent).getByRole('checkbox', { name: /Tokyo/ })).toBeChecked();
    await user.click(within(parent).getByRole('tab', { name: 'Matches' }));
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
    expect(screen.getByRole('dialog', { name: 'Edit rule group' })).toBeInTheDocument();
    expect(within(parent).getByRole('button', { name: 'Domains' })).toBeInTheDocument();
  });
  it('sends unsaved native template bytes to the server and saves only after successful validation', async () => {
    const user = userEvent.setup();
    const client = mount();
    await user.click(screen.getByRole('tab', { name: 'Subscription rules' }));
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
