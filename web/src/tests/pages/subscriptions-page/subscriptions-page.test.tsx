import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';

import '@/i18n';

import type { SubscriptionListFilter, SubscriptionSourceVersion } from '@/api/api-client';

import { ApiClientProvider } from '@/api/api-client-context';
import { SubscriptionsPage } from '@/pages/subscriptions-page/subscriptions-page';
import { buildPublicSubscriptionURL } from '@/pages/subscriptions-page/public-subscription-url';
import { createMockApiClient, testSubscriptionChannels, testSubscriptionSources, testSubscriptionSourceVersion, testSubscriptionTokens } from '@/tests/api/mock-api-client';

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((accept, decline) => {
    reject = decline;
    resolve = accept;
  });
  return { promise, reject, resolve };
}

describe('subscriptionsPage', () => {
  it('builds the public subscription path under the configured document base', () => {
    expect(buildPublicSubscriptionURL(
      'secret/token',
      'channel main',
      'https://panel.example/panel/',
    )).toBe('https://panel.example/panel/sub/secret%2Ftoken/channel%20main');
  });

  it('keeps the current route, query and router history state for section links', async () => {
    const user = userEvent.setup();
    const originalURL = window.location.href;
    const originalState = window.history.state;
    const routerState = { idx: 4, key: 'router-key', usr: { from: 'tests' } };

    try {
      window.history.replaceState(routerState, '', '/panel/subscriptions/?view=active');
      const client = createMockApiClient();
      render(<ApiClientProvider client={client}><SubscriptionsPage /></ApiClientProvider>);

      const channelsTab = screen.getByRole('tab', { name: 'Channels' });
      await user.click(channelsTab);

      expect(window.location.pathname).toBe('/panel/subscriptions/');
      expect(window.location.search).toBe('?view=active');
      expect(window.location.hash).toBe('#subscription-channels');
      expect(window.history.state).toEqual(routerState);
    } finally {
      window.history.replaceState(originalState, '', originalURL);
    }
  });

  it('creates a channel through the strict management contract', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient();
    render(<ApiClientProvider client={client}><SubscriptionsPage /></ApiClientProvider>);

    await user.click(screen.getByRole('tab', { name: 'Channels' }));
    const panel = (await screen.findByRole('heading', { name: 'Channels' })).closest('section');
    expect(panel).not.toBeNull();
    await user.click(within(panel!).getByRole('button', { name: 'New channel' }));
    const dialog = await screen.findByRole('dialog', { name: 'Create channel' });
    await user.type(within(dialog).getByLabelText('Name'), 'Mobile clients');
    await user.type(within(dialog).getByLabelText('Public host'), 'proxy.example');
    await user.selectOptions(within(dialog).getByLabelText('Format'), 'mihomo');
    await user.type(within(dialog).getByLabelText('Excluded tags'), 'private, private, lab');
    await user.click(within(dialog).getByRole('button', { name: 'Save channel' }));

    expect(client.createSubscriptionChannel).toHaveBeenCalledWith({
      name: 'Mobile clients',
      format: 'mihomo',
      public_host: 'proxy.example',
      config: { exclude_tags: ['private', 'lab'], exclude_types: [] },
      enabled: true,
    });
  });

  it('creates a validated source version separately from metadata', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient();
    render(<ApiClientProvider client={client}><SubscriptionsPage /></ApiClientProvider>);

    await user.click(screen.getByRole('tab', { name: 'Sources' }));
    const panel = (await screen.findByRole('heading', { name: 'Sources' })).closest('section');
    expect(panel).not.toBeNull();
    await user.click(within(panel!).getByRole('button', { name: 'Edit' }));
    const dialog = await screen.findByRole('dialog', { name: 'Edit Operator additions' });
    const snapshot = within(dialog).getByLabelText('New source document');
    fireEvent.change(snapshot, { target: { value: '{"outbounds":[{"tag":"extra"}]}' } });
    await user.click(within(dialog).getByRole('button', { name: 'Validate and activate version' }));

    expect(client.createSubscriptionSourceVersion).toHaveBeenCalledWith(
      'source_local',
      'auto',
      '{"outbounds":[{"tag":"extra"}]}',
      '2026-08-26T07:06:00Z',
    );
    expect(client.updateSubscriptionSource).not.toHaveBeenCalled();
  });

  it('keeps a newly created source open when its initial version fails', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient({
      createSubscriptionSourceVersion: vi.fn().mockRejectedValue(new Error('version rejected')),
    });
    render(<ApiClientProvider client={client}><SubscriptionsPage /></ApiClientProvider>);

    await user.click(screen.getByRole('tab', { name: 'Sources' }));
    const panel = (await screen.findByRole('heading', { name: 'Sources' })).closest('section');
    expect(panel).not.toBeNull();
    await user.click(within(panel!).getByRole('button', { name: 'Attach source' }));
    const dialog = await screen.findByRole('dialog', { name: 'Attach source' });
    await user.type(within(dialog).getByLabelText('Name'), 'Imported source');
    fireEvent.change(within(dialog).getByLabelText('New source document'), {
      target: { value: 'socks://proxy.example:1080' },
    });
    const form = within(dialog).getByRole('heading', { name: 'Attach source' }).closest('form');
    expect(form).not.toBeNull();
    await user.click(within(form!).getByRole('button', { name: 'Attach source' }));

    expect(await within(dialog).findByText(/initial version was not saved/i)).toBeInTheDocument();
    expect(within(dialog).getByRole('heading', { name: /Edit Operator additions/i })).toBeInTheDocument();
    expect(within(dialog).getByRole('button', { name: 'Validate and activate version' })).toBeInTheDocument();
    expect(client.createSubscriptionSource).toHaveBeenCalledTimes(1);
    expect(client.createSubscriptionSourceVersion).toHaveBeenCalledTimes(1);
  });

  it('loads fresh source and immutable version evidence before a cancelable restore', async () => {
    const user = userEvent.setup();
    const freshSource = {
      ...testSubscriptionSources[0],
      updated_at: '2026-08-26T07:06:30Z',
    };
    const exactVersion = {
      ...testSubscriptionSourceVersion,
      id: 'version_1',
      sha256: 'c'.repeat(64),
    };
    const client = createMockApiClient({
      getSubscriptionSource: vi.fn().mockResolvedValue(freshSource),
      getSubscriptionSourceVersion: vi.fn().mockResolvedValue(exactVersion),
    });
    render(<ApiClientProvider client={client}><SubscriptionsPage /></ApiClientProvider>);

    await user.click(screen.getByRole('tab', { name: 'Sources' }));
    const panel = (await screen.findByRole('heading', { name: 'Sources' })).closest('section');
    expect(panel).not.toBeNull();
    await user.click(within(panel!).getByRole('button', { name: 'Versions' }));
    const versionSheet = await screen.findByRole('dialog', { name: 'Operator additions' });
    await user.click(within(versionSheet).getByRole('button', { name: 'Restore' }));

    const confirmation = await screen.findByRole('alertdialog');
    expect(within(confirmation).getByText(
      `Restore immutable version version_1 (SHA-256 ${exactVersion.sha256}) to Operator additions (source source_local) only if the source revision is still 2026-08-26T07:06:30Z.`,
    )).toBeInTheDocument();
    expect(client.getSubscriptionSource).toHaveBeenCalledWith('source_local');
    expect(client.getSubscriptionSourceVersion).toHaveBeenCalledWith('source_local', 'version_1');
    expect(client.restoreSubscriptionSourceVersion).not.toHaveBeenCalled();

    await user.click(within(confirmation).getByRole('button', { name: 'Cancel' }));
    await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument());
    expect(client.restoreSubscriptionSourceVersion).not.toHaveBeenCalled();
  });

  it('keeps restore identity and CAS evidence stable while fetched objects drift', async () => {
    const user = userEvent.setup();
    const freshSource = {
      ...testSubscriptionSources[0],
      updated_at: '2026-08-26T07:06:30Z',
    };
    const exactVersion = {
      ...testSubscriptionSourceVersion,
      id: 'version_1',
      sha256: 'c'.repeat(64),
    };
    const restoreSubscriptionSourceVersion = vi.fn().mockRejectedValue(new Error('source changed'));
    const client = createMockApiClient({
      getSubscriptionSource: vi.fn().mockResolvedValue(freshSource),
      getSubscriptionSourceVersion: vi.fn().mockResolvedValue(exactVersion),
      restoreSubscriptionSourceVersion,
    });
    render(<ApiClientProvider client={client}><SubscriptionsPage /></ApiClientProvider>);

    await user.click(screen.getByRole('tab', { name: 'Sources' }));
    const panel = (await screen.findByRole('heading', { name: 'Sources' })).closest('section');
    expect(panel).not.toBeNull();
    await user.click(within(panel!).getByRole('button', { name: 'Versions' }));
    const versionSheet = await screen.findByRole('dialog', { name: 'Operator additions' });
    await user.click(within(versionSheet).getByRole('button', { name: 'Restore' }));
    const confirmation = await screen.findByRole('alertdialog');

    freshSource.updated_at = '2026-08-26T07:07:00Z';
    exactVersion.id = 'version_drifted';
    await user.click(within(confirmation).getByRole('button', { name: 'Restore immutable version' }));

    await waitFor(() => expect(restoreSubscriptionSourceVersion).toHaveBeenCalledWith(
      'source_local',
      'version_1',
      '2026-08-26T07:06:30Z',
    ));
    expect(await within(versionSheet).findByText('source changed')).toBeInTheDocument();
  });

  it('restores the confirmed immutable source version with the fresh CAS base', async () => {
    const user = userEvent.setup();
    const freshSource = {
      ...testSubscriptionSources[0],
      updated_at: '2026-08-26T07:06:30Z',
    };
    const restoredSource = {
      ...freshSource,
      current_version_id: 'version_1',
      updated_at: '2026-08-26T07:07:00Z',
    };
    const restoreSubscriptionSourceVersion = vi.fn().mockResolvedValue(restoredSource);
    const client = createMockApiClient({
      getSubscriptionSource: vi.fn().mockResolvedValue(freshSource),
      getSubscriptionSourceVersion: vi.fn().mockResolvedValue({
        ...testSubscriptionSourceVersion,
        id: 'version_1',
      }),
      restoreSubscriptionSourceVersion,
    });
    render(<ApiClientProvider client={client}><SubscriptionsPage /></ApiClientProvider>);

    await user.click(screen.getByRole('tab', { name: 'Sources' }));
    const panel = (await screen.findByRole('heading', { name: 'Sources' })).closest('section');
    expect(panel).not.toBeNull();
    await user.click(within(panel!).getByRole('button', { name: 'Versions' }));
    const versionSheet = await screen.findByRole('dialog', { name: 'Operator additions' });
    await user.click(within(versionSheet).getByRole('button', { name: 'Restore' }));
    const confirmation = await screen.findByRole('alertdialog');
    await user.click(within(confirmation).getByRole('button', { name: 'Restore immutable version' }));

    await waitFor(() => expect(restoreSubscriptionSourceVersion).toHaveBeenCalledWith(
      'source_local',
      'version_1',
      '2026-08-26T07:06:30Z',
    ));
    expect(await screen.findByText(
      'Restored immutable version version_1 for Operator additions.',
    )).toBeInTheDocument();
  });

  it('requires inline confirmation for every destructive subscription action', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient();
    render(<ApiClientProvider client={client}><SubscriptionsPage /></ApiClientProvider>);

    await user.click(screen.getByRole('tab', { name: 'Users' }));
    const userPanel = (await screen.findByRole('heading', { name: 'Users and node grants' })).closest('section');
    expect(userPanel).not.toBeNull();

    const deleteUser = within(userPanel!).getByRole('button', { name: 'Delete user Primary user' });
    await user.click(deleteUser);
    const confirmUserDelete = within(userPanel!).getByRole('button', {
      name: 'Confirm delete user Primary user',
    });
    const keepUser = within(userPanel!).getByRole('button', { name: 'Keep user Primary user' });
    expect(keepUser).toHaveFocus();
    expect(confirmUserDelete).not.toHaveFocus();
    expect(client.deleteSubscriptionUser).not.toHaveBeenCalled();
    await user.keyboard('{Enter}');
    expect(client.deleteSubscriptionUser).not.toHaveBeenCalled();

    await user.click(within(userPanel!).getByRole('button', { name: 'Delete user Primary user' }));
    await user.click(within(userPanel!).getByRole('button', {
      name: 'Confirm delete user Primary user',
    }));
    await waitFor(() => expect(client.deleteSubscriptionUser).toHaveBeenCalledWith(
      'user_1',
      '2026-08-26T07:00:00Z',
    ));

    await user.click(screen.getByRole('tab', { name: 'Channels' }));
    const channelPanel = screen.getByRole('heading', { name: 'Channels' }).closest('section');
    expect(channelPanel).not.toBeNull();
    await user.click(within(channelPanel!).getByRole('button', {
      name: 'Delete channel Primary sing-box',
    }));
    expect(client.deleteSubscriptionChannel).not.toHaveBeenCalled();
    const confirmChannelDelete = within(channelPanel!).getByRole('button', {
      name: 'Confirm delete channel Primary sing-box',
    });
    expect(within(channelPanel!).getByRole('button', {
      name: 'Keep channel Primary sing-box',
    })).toHaveFocus();
    await user.tab({ shift: true });
    expect(confirmChannelDelete).toHaveFocus();
    await user.keyboard('{Enter}');
    await waitFor(() => expect(client.deleteSubscriptionChannel).toHaveBeenCalledWith(
      'channel_sing_box',
      '2026-08-26T07:05:00.000000001Z',
    ));

    await user.click(screen.getByRole('tab', { name: 'Sources' }));
    const sourcePanel = screen.getByRole('heading', { name: 'Sources' }).closest('section');
    expect(sourcePanel).not.toBeNull();
    await user.click(within(sourcePanel!).getByRole('button', {
      name: 'Delete source Operator additions',
    }));
    expect(client.deleteSubscriptionSource).not.toHaveBeenCalled();
    await user.click(within(sourcePanel!).getByRole('button', {
      name: 'Confirm delete source Operator additions',
    }));
    await waitFor(() => expect(client.deleteSubscriptionSource).toHaveBeenCalledWith(
      'source_local',
      '2026-08-26T07:06:00Z',
    ));

    await user.click(screen.getByRole('tab', { name: 'Tokens' }));
    const tokenPanel = screen.getByRole('heading', { name: 'Tokens' }).closest('section');
    expect(tokenPanel).not.toBeNull();
    await user.click(within(tokenPanel!).getByRole('button', { name: 'Revoke token phone' }));
    expect(client.revokeSubscriptionToken).not.toHaveBeenCalled();
    await user.click(within(tokenPanel!).getByRole('button', { name: 'Confirm revoke token phone' }));
    await waitFor(() => expect(client.revokeSubscriptionToken).toHaveBeenCalledWith('token_primary'));

    await user.click(within(tokenPanel!).getByRole('button', { name: 'Delete token phone' }));
    expect(client.deleteSubscriptionToken).not.toHaveBeenCalled();
    await user.click(within(tokenPanel!).getByRole('button', { name: 'Confirm delete token phone' }));
    await waitFor(() => expect(client.deleteSubscriptionToken).toHaveBeenCalledWith('token_primary'));
  });

  it('preserves the visible one-time secret on issue failure and resets copied for a new secret', async () => {
    const user = userEvent.setup();
    const createSubscriptionToken = vi.fn()
      .mockResolvedValueOnce({
        metadata: { ...testSubscriptionTokens[0], id: 'token_first' },
        token: 'secret-first',
      })
      .mockRejectedValueOnce(new Error('issue denied'))
      .mockResolvedValueOnce({
        metadata: { ...testSubscriptionTokens[0], id: 'token_second' },
        token: 'secret-second',
      });
    const client = createMockApiClient({ createSubscriptionToken });
    render(<ApiClientProvider client={client}><SubscriptionsPage /></ApiClientProvider>);

    await user.click(screen.getByRole('tab', { name: 'Tokens' }));
    const panel = (await screen.findByRole('heading', { name: 'Tokens' })).closest('section');
    expect(panel).not.toBeNull();
    await user.type(within(panel!).getByLabelText('Label'), 'travel phone');

    await user.click(within(panel!).getByRole('button', { name: 'Issue token' }));
    expect(await within(panel!).findByText('secret-first')).toBeInTheDocument();
    expect(await within(panel!).findByLabelText('Client subscription URL')).toHaveTextContent(
      '/sub/secret-first/channel_sing_box',
    );
    await user.click(within(panel!).getByRole('button', { name: 'Copy subscription URL' }));
    expect(within(panel!).getByRole('button', { name: 'URL copied' })).toBeInTheDocument();
    await user.click(within(panel!).getByRole('button', { name: 'Copy token' }));
    expect(within(panel!).getByRole('button', { name: 'Copied' })).toBeInTheDocument();

    await user.click(within(panel!).getByRole('button', { name: 'Issue token' }));
    expect(await within(panel!).findByText('issue denied')).toBeInTheDocument();
    expect(within(panel!).getByText('secret-first')).toBeInTheDocument();
    expect(within(panel!).getByRole('button', { name: 'Copied' })).toBeInTheDocument();

    await user.click(within(panel!).getByRole('button', { name: 'Issue token' }));
    expect(await within(panel!).findByText('secret-second')).toBeInTheDocument();
    expect(within(panel!).queryByText('secret-first')).not.toBeInTheDocument();
    expect(within(panel!).getByRole('button', { name: 'Copy token' })).toBeInTheDocument();
  });

  it('reports unavailable and rejected clipboard writes without losing success feedback', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient();
    render(<ApiClientProvider client={client}><SubscriptionsPage /></ApiClientProvider>);

    await user.click(screen.getByRole('tab', { name: 'Tokens' }));
    const panel = (await screen.findByRole('heading', { name: 'Tokens' })).closest('section');
    expect(panel).not.toBeNull();
    await user.type(within(panel!).getByLabelText('Label'), 'clipboard test');
    await user.click(within(panel!).getByRole('button', { name: 'Issue token' }));
    expect(await within(panel!).findByText('one-time-public-token')).toBeInTheDocument();

    const originalClipboard = Object.getOwnPropertyDescriptor(navigator, 'clipboard');
    try {
      Object.defineProperty(navigator, 'clipboard', { configurable: true, value: undefined });
      await user.click(within(panel!).getByRole('button', { name: 'Copy token' }));
      expect(await within(panel!).findByText('Clipboard access is unavailable.')).toBeInTheDocument();
      expect(within(panel!).getByRole('button', { name: 'Copy token' })).toBeInTheDocument();

      const writeText = vi.fn()
        .mockRejectedValueOnce(new Error('clipboard permission denied'))
        .mockResolvedValueOnce(undefined);
      Object.defineProperty(navigator, 'clipboard', {
        configurable: true,
        value: { writeText },
      });
      await user.click(within(panel!).getByRole('button', { name: 'Copy token' }));
      expect(await within(panel!).findByText(
        'Could not copy to the clipboard: clipboard permission denied',
      )).toBeInTheDocument();
      expect(within(panel!).getByRole('button', { name: 'Copy token' })).toBeInTheDocument();

      await user.click(within(panel!).getByRole('button', { name: 'Copy token' }));
      expect(within(panel!).getByRole('button', { name: 'Copied' })).toBeInTheDocument();
      expect(writeText).toHaveBeenCalledTimes(2);
    } finally {
      if (originalClipboard === undefined) Reflect.deleteProperty(navigator, 'clipboard');
      else Object.defineProperty(navigator, 'clipboard', originalClipboard);
    }
  });

  it('does not mark a newly issued secret copied when an older token copy finishes late', async () => {
    const pendingCopy = deferred<void>();
    const user = userEvent.setup();
    const createSubscriptionToken = vi.fn()
      .mockResolvedValueOnce({
        metadata: { ...testSubscriptionTokens[0], id: 'token_first' },
        token: 'secret-first',
      })
      .mockResolvedValueOnce({
        metadata: { ...testSubscriptionTokens[0], id: 'token_second' },
        token: 'secret-second',
      });
    const client = createMockApiClient({ createSubscriptionToken });
    render(<ApiClientProvider client={client}><SubscriptionsPage /></ApiClientProvider>);

    await user.click(screen.getByRole('tab', { name: 'Tokens' }));
    const panel = (await screen.findByRole('heading', { name: 'Tokens' })).closest('section');
    expect(panel).not.toBeNull();
    await user.type(within(panel!).getByLabelText('Label'), 'clipboard race');

    const originalClipboard = Object.getOwnPropertyDescriptor(navigator, 'clipboard');
    const writeText = vi.fn().mockReturnValue(pendingCopy.promise);
    try {
      Object.defineProperty(navigator, 'clipboard', {
        configurable: true,
        value: { writeText },
      });

      await user.click(within(panel!).getByRole('button', { name: 'Issue token' }));
      expect(await within(panel!).findByText('secret-first')).toBeInTheDocument();
      await user.click(within(panel!).getByRole('button', { name: 'Copy token' }));
      expect(writeText).toHaveBeenCalledWith('secret-first');

      await user.click(within(panel!).getByRole('button', { name: 'Issue token' }));
      expect(await within(panel!).findByText('secret-second')).toBeInTheDocument();
      expect(within(panel!).getByRole('button', { name: 'Copy token' })).toBeInTheDocument();

      await act(async () => {
        pendingCopy.resolve();
        await pendingCopy.promise;
      });
      expect(within(panel!).getByRole('button', { name: 'Copy token' })).toBeInTheDocument();
      expect(within(panel!).queryByRole('button', { name: 'Copied' })).not.toBeInTheDocument();
    } finally {
      if (originalClipboard === undefined) Reflect.deleteProperty(navigator, 'clipboard');
      else Object.defineProperty(navigator, 'clipboard', originalClipboard);
    }
  });

  it('does not report a stale public URL copy rejection against a newly issued secret', async () => {
    const pendingCopy = deferred<void>();
    const user = userEvent.setup();
    const createSubscriptionToken = vi.fn()
      .mockResolvedValueOnce({
        metadata: { ...testSubscriptionTokens[0], id: 'token_first' },
        token: 'secret-first',
      })
      .mockResolvedValueOnce({
        metadata: { ...testSubscriptionTokens[0], id: 'token_second' },
        token: 'secret-second',
      });
    const client = createMockApiClient({ createSubscriptionToken });
    render(<ApiClientProvider client={client}><SubscriptionsPage /></ApiClientProvider>);

    await user.click(screen.getByRole('tab', { name: 'Tokens' }));
    const panel = (await screen.findByRole('heading', { name: 'Tokens' })).closest('section');
    expect(panel).not.toBeNull();
    await user.type(within(panel!).getByLabelText('Label'), 'URL clipboard race');

    const originalClipboard = Object.getOwnPropertyDescriptor(navigator, 'clipboard');
    const writeText = vi.fn().mockReturnValue(pendingCopy.promise);
    try {
      Object.defineProperty(navigator, 'clipboard', {
        configurable: true,
        value: { writeText },
      });

      await user.click(within(panel!).getByRole('button', { name: 'Issue token' }));
      expect(await within(panel!).findByText('secret-first')).toBeInTheDocument();
      const firstPublicURL = within(panel!).getByLabelText('Client subscription URL').textContent;
      await user.click(within(panel!).getByRole('button', { name: 'Copy subscription URL' }));
      expect(writeText).toHaveBeenCalledWith(firstPublicURL);

      await user.click(within(panel!).getByRole('button', { name: 'Issue token' }));
      expect(await within(panel!).findByText('secret-second')).toBeInTheDocument();
      expect(within(panel!).getByLabelText('Client subscription URL')).toHaveTextContent(
        '/sub/secret-second/channel_sing_box',
      );
      expect(within(panel!).getByRole('button', { name: 'Copy subscription URL' })).toBeInTheDocument();

      await act(async () => {
        pendingCopy.reject(new Error('late clipboard failure'));
        await pendingCopy.promise.catch(() => undefined);
      });
      expect(within(panel!).getByRole('button', { name: 'Copy subscription URL' })).toBeInTheDocument();
      expect(within(panel!).queryByRole('button', { name: 'URL copied' })).not.toBeInTheDocument();
      expect(within(panel!).queryByText(/late clipboard failure/i)).not.toBeInTheDocument();
    } finally {
      if (originalClipboard === undefined) Reflect.deleteProperty(navigator, 'clipboard');
      else Object.defineProperty(navigator, 'clipboard', originalClipboard);
    }
  });

  it('confirms rotation safely, retains the current secret on failure, and resets copied on success', async () => {
    const user = userEvent.setup();
    const rotateSubscriptionToken = vi.fn()
      .mockRejectedValueOnce(new Error('rotation denied'))
      .mockResolvedValueOnce({
        revoked: { ...testSubscriptionTokens[0], active: false },
        created: { ...testSubscriptionTokens[0], id: 'token_rotated' },
        token: 'secret-rotated',
      });
    const client = createMockApiClient({ rotateSubscriptionToken });
    render(<ApiClientProvider client={client}><SubscriptionsPage /></ApiClientProvider>);

    await user.click(screen.getByRole('tab', { name: 'Tokens' }));
    const panel = (await screen.findByRole('heading', { name: 'Tokens' })).closest('section');
    expect(panel).not.toBeNull();
    await user.type(within(panel!).getByLabelText('Label'), 'travel phone');
    await user.click(within(panel!).getByRole('button', { name: 'Issue token' }));
    expect(await within(panel!).findByText('one-time-public-token')).toBeInTheDocument();
    await user.click(within(panel!).getByRole('button', { name: 'Copy token' }));

    await user.click(within(panel!).getByRole('button', { name: 'Rotate token phone' }));
    const cancelRotation = within(panel!).getByRole('button', { name: 'Cancel rotate token phone' });
    expect(cancelRotation).toHaveFocus();
    expect(rotateSubscriptionToken).not.toHaveBeenCalled();
    await user.keyboard('{Enter}');
    expect(rotateSubscriptionToken).not.toHaveBeenCalled();

    await user.click(within(panel!).getByRole('button', { name: 'Rotate token phone' }));
    await user.click(within(panel!).getByRole('button', { name: 'Confirm rotate token phone' }));
    expect(await within(panel!).findByText('rotation denied')).toBeInTheDocument();
    expect(within(panel!).getByText('one-time-public-token')).toBeInTheDocument();
    expect(within(panel!).getByRole('button', { name: 'Copied' })).toBeInTheDocument();

    await user.click(within(panel!).getByRole('button', { name: 'Confirm rotate token phone' }));
    expect(await within(panel!).findByText('secret-rotated')).toBeInTheDocument();
    expect(within(panel!).getByRole('button', { name: 'Copy token' })).toBeInTheDocument();
  });

  it('locks each paginated subscription request to one in-flight cursor', async () => {
    const cursor = { created_at: '2026-08-26T07:00:00Z', id: 'page_1' };
    const channelPage = deferred<{ items: typeof testSubscriptionChannels; next: undefined }>();
    const sourcePage = deferred<{ items: typeof testSubscriptionSources; next: undefined }>();
    const tokenPage = deferred<{ items: typeof testSubscriptionTokens; next: undefined }>();
    const listSubscriptionChannels = vi.fn()
      .mockResolvedValueOnce({ items: testSubscriptionChannels, next: cursor })
      .mockReturnValueOnce(channelPage.promise);
    const listSubscriptionSources = vi.fn()
      .mockResolvedValueOnce({ items: testSubscriptionSources, next: cursor })
      .mockReturnValueOnce(sourcePage.promise);
    const listSubscriptionTokens = vi.fn()
      .mockResolvedValueOnce({ items: testSubscriptionTokens, next: cursor })
      .mockReturnValueOnce(tokenPage.promise);
    const client = createMockApiClient({
      listSubscriptionChannels,
      listSubscriptionSources,
      listSubscriptionTokens,
    });
    render(<ApiClientProvider client={client}><SubscriptionsPage /></ApiClientProvider>);

    const buttons: HTMLElement[] = [];
    for (const [tab, buttonName] of [
      ['Channels', 'Load more channels'],
      ['Sources', 'Load more sources'],
      ['Tokens', 'Load more tokens'],
    ] as const) {
      fireEvent.click(screen.getByRole('tab', { name: tab }));
      const button = await screen.findByRole('button', { name: buttonName });
      buttons.push(button);
      fireEvent.click(button);
      fireEvent.click(button);
      expect(button).toBeDisabled();
      expect(button).toHaveAttribute('aria-busy', 'true');
    }
    expect(listSubscriptionChannels).toHaveBeenCalledTimes(2);
    expect(listSubscriptionSources).toHaveBeenCalledTimes(2);
    expect(listSubscriptionTokens).toHaveBeenCalledTimes(2);

    await act(async () => {
      channelPage.resolve({ items: [], next: undefined });
      sourcePage.resolve({ items: [], next: undefined });
      tokenPage.resolve({ items: [], next: undefined });
      await Promise.all([channelPage.promise, sourcePage.promise, tokenPage.promise]);
    });
    await waitFor(() => {
      expect(screen.queryByText('Loading more channels…')).not.toBeInTheDocument();
      expect(screen.queryByText('Loading more sources…')).not.toBeInTheDocument();
      expect(screen.queryByText('Loading more tokens…')).not.toBeInTheDocument();
    });
  });

  it('aggregates every user page for authorization and both user selectors', async () => {
    const cursor = { created_at: '2026-08-26T07:00:00Z', id: 'user_1' };
    const olderUser = {
      id: 'user_older',
      name: 'Older user',
      description: 'Loaded from the next page',
      enabled: true,
      created_at: '2026-08-25T07:00:00Z',
      updated_at: '2026-08-25T07:00:00Z',
    };
    const listSubscriptionUsers = vi.fn(async (filter: SubscriptionListFilter = {}) =>
      filter.beforeID === cursor.id
        ? { items: [olderUser] }
        : { items: [{
            id: 'user_1',
            name: 'Primary user',
            description: 'Personal devices',
            enabled: true,
            created_at: '2026-08-26T07:00:00Z',
            updated_at: '2026-08-26T07:00:00Z',
          }], next: cursor });
    const client = createMockApiClient({ listSubscriptionUsers });
    render(<ApiClientProvider client={client}><SubscriptionsPage /></ApiClientProvider>);

    fireEvent.click(screen.getByRole('tab', { name: 'Users' }));
    const userPanel = (await screen.findByRole('heading', { name: 'Users and node grants' })).closest('section');
    expect(userPanel).not.toBeNull();
    expect(await within(userPanel!).findByText('Older user')).toBeInTheDocument();
    expect(await screen.findAllByRole('option', { name: 'Older user', hidden: true })).toHaveLength(2);
    await waitFor(() => expect(listSubscriptionUsers.mock.calls.filter(
      ([filter]) => filter?.beforeID === cursor.id,
    )).toHaveLength(1));
    expect(listSubscriptionUsers).toHaveBeenCalledTimes(2);
  });

  it('shares user-directory failure and disables dependent token issuance', async () => {
    const listSubscriptionUsers = vi.fn().mockRejectedValue(new Error('user directory offline'));
    const client = createMockApiClient({ listSubscriptionUsers });
    render(<ApiClientProvider client={client}><SubscriptionsPage /></ApiClientProvider>);
    const user = userEvent.setup();

    expect(await screen.findByText('user directory offline')).toBeInTheDocument();
    expect(listSubscriptionUsers).toHaveBeenCalledTimes(1);
    await user.click(screen.getByRole('tab', { name: 'Tokens' }));
    expect(screen.getByRole('button', { name: 'Issue token' })).toBeDisabled();
    expect(screen.getByLabelText('User')).toBeDisabled();
  });

  it('shares node-catalog failure, keeps grants fail-closed, and reloads explicitly', async () => {
    const user = userEvent.setup();
    const getSubscriptionNodeCatalog = vi.fn().mockRejectedValue(new Error('node catalog offline'));
    const client = createMockApiClient({ getSubscriptionNodeCatalog });
    render(<ApiClientProvider client={client}><SubscriptionsPage /></ApiClientProvider>);

    expect(await screen.findByText('node catalog offline')).toBeInTheDocument();
    expect(getSubscriptionNodeCatalog).toHaveBeenCalledTimes(1);
    await user.click(screen.getByRole('tab', { name: 'Users' }));
    await user.click(screen.getByRole('button', { name: 'Retry' }));
    await waitFor(() => expect(getSubscriptionNodeCatalog).toHaveBeenCalledTimes(2));

    await user.click(screen.getByRole('button', { name: 'Permissions' }));
    const permissionsSheet = await screen.findByRole('dialog', { name: 'Primary user' });
    expect(within(permissionsSheet).getByRole('button', { name: 'Save permissions' })).toBeDisabled();
  });

  it('links an accepted source refresh to its durable task', async () => {
    const remoteSource = {
      ...testSubscriptionSources[0],
      source_kind: 'remote' as const,
    };
    const client = createMockApiClient({
      listSubscriptionSources: vi.fn().mockResolvedValue({ items: [remoteSource] }),
    });
    render(
      <MemoryRouter>
        <ApiClientProvider client={client}><SubscriptionsPage /></ApiClientProvider>
      </MemoryRouter>,
    );
    const user = userEvent.setup();

    await user.click(screen.getByRole('tab', { name: 'Sources' }));
    await user.click(await screen.findByRole('button', { name: 'Refresh' }));

    const taskLink = await screen.findByRole('link', { name: 'View accepted task' });
    expect(taskLink).toHaveAttribute('href', '/tasks');
    expect(client.refreshSubscriptionSource).toHaveBeenCalledWith(remoteSource.id);
  });

  it('loads the exact user before editing node grants', async () => {
    const user = userEvent.setup();
    const exactUser = {
      id: 'user_1',
      name: 'Primary user (fresh)',
      description: 'Fresh detail response',
      enabled: true,
      created_at: '2026-08-26T07:00:00Z',
      updated_at: '2026-08-26T07:00:01Z',
    };
    const getSubscriptionUser = vi.fn().mockResolvedValue(exactUser);
    const getSubscriptionUserGrants = vi.fn().mockResolvedValue({
      user: { ...exactUser, name: 'Primary user (stale)' },
      grants: [],
    });
    const client = createMockApiClient({ getSubscriptionUser, getSubscriptionUserGrants });
    render(<ApiClientProvider client={client}><SubscriptionsPage /></ApiClientProvider>);

    await user.click(screen.getByRole('tab', { name: 'Users' }));
    const panel = (await screen.findByRole('heading', { name: 'Users and node grants' })).closest('section');
    expect(panel).not.toBeNull();
    const permissionsTrigger = within(panel!).getByRole('button', { name: 'Permissions' });
    await user.click(permissionsTrigger);

    const permissionsSheet = await screen.findByRole('dialog', { name: exactUser.name });
    expect(permissionsSheet).toBeInTheDocument();
    expect(panel).not.toContainElement(permissionsSheet);
    expect(getSubscriptionUser).toHaveBeenCalledWith('user_1', expect.any(AbortSignal));
    expect(getSubscriptionUserGrants).toHaveBeenCalledWith('user_1', expect.any(AbortSignal));
    await user.keyboard('{Escape}');
    await waitFor(() => expect(screen.queryByRole('dialog', { name: exactUser.name })).not.toBeInTheDocument());
    expect(permissionsTrigger).toHaveFocus();
  });

  it('retains the source-version cursor and appends older versions on request', async () => {
    const user = userEvent.setup();
    const cursor = { created_at: '2026-08-25T07:00:00Z', id: 'source_version_1' };
    const currentVersion: SubscriptionSourceVersion = {
      id: 'source_version_1',
      source_id: 'source_local',
      format: 'sing-box-json',
      normalized_nodes: [],
      diagnostics: [],
      sha256: 'a'.repeat(64),
      fetched_at: '2026-08-26T07:00:00Z',
      created_at: '2026-08-26T07:00:00Z',
    };
    const olderVersion: SubscriptionSourceVersion = {
      ...currentVersion,
      id: 'source_version_older',
      fetched_at: '2026-08-24T07:00:00Z',
      created_at: '2026-08-24T07:00:00Z',
    };
    const listSubscriptionSourceVersions = vi.fn(
      async (_sourceID: string, filter: SubscriptionListFilter = {}) =>
        filter.beforeID === cursor.id
          ? { items: [olderVersion] }
          : { items: [currentVersion], next: cursor },
    );
    const client = createMockApiClient({ listSubscriptionSourceVersions });
    render(<ApiClientProvider client={client}><SubscriptionsPage /></ApiClientProvider>);

    await user.click(screen.getByRole('tab', { name: 'Sources' }));
    const sourcePanel = (await screen.findByRole('heading', { name: 'Sources' })).closest('section');
    expect(sourcePanel).not.toBeNull();
    await user.click(within(sourcePanel!).getByRole('button', { name: 'Versions' }));
    const versionSheet = await screen.findByRole('dialog', { name: 'Operator additions' });
    await user.click(within(versionSheet).getByRole('button', { name: 'Load older versions' }));

    expect(await within(versionSheet).findByText('source_version_older')).toBeInTheDocument();
    expect(listSubscriptionSourceVersions).toHaveBeenCalledWith('source_local', {
      beforeID: cursor.id,
      beforeTime: cursor.created_at,
      limit: 100,
    });
    expect(within(versionSheet).queryByRole('button', { name: 'Load older versions' })).not.toBeInTheDocument();
  });

  it('loads exact token and source-version details through their detail contracts', async () => {
    const user = userEvent.setup();
    const sourceVersion: SubscriptionSourceVersion = {
      id: 'source_version_detail',
      source_id: 'source_local',
      format: 'sing-box-json',
      raw_body: '{"outbounds":[]}',
      normalized_nodes: [{ tag: 'detail-node', type: 'socks' }],
      diagnostics: [],
      sha256: 'b'.repeat(64),
      fetched_at: '2026-08-26T07:00:00Z',
      created_at: '2026-08-26T07:00:00Z',
    };
    const getSubscriptionToken = vi.fn().mockResolvedValue(testSubscriptionTokens[0]);
    const getSubscriptionSourceVersion = vi.fn().mockResolvedValue(sourceVersion);
    const client = createMockApiClient({
      getSubscriptionSourceVersion,
      getSubscriptionToken,
      listSubscriptionSourceVersions: vi.fn().mockResolvedValue({
        items: [{ ...sourceVersion, raw_body: undefined }],
      }),
    });
    render(<ApiClientProvider client={client}><SubscriptionsPage /></ApiClientProvider>);

    await user.click(screen.getByRole('tab', { name: 'Tokens' }));
    const tokenPanel = (await screen.findByRole('heading', { name: 'Tokens' })).closest('section');
    expect(tokenPanel).not.toBeNull();
    await user.click(within(tokenPanel!).getByRole('button', { name: 'Inspect token phone' }));
    const tokenSheet = await screen.findByRole('dialog', { name: 'phone' });
    expect(tokenSheet).toBeInTheDocument();
    expect(tokenPanel).not.toContainElement(tokenSheet);
    expect(getSubscriptionToken).toHaveBeenCalledWith('token_primary');
    await user.click(within(tokenSheet).getByRole('button', { name: 'Close' }));

    await user.click(screen.getByRole('tab', { name: 'Sources' }));
    const sourcePanel = screen.getByRole('heading', { name: 'Sources' }).closest('section');
    expect(sourcePanel).not.toBeNull();
    await user.click(within(sourcePanel!).getByRole('button', { name: 'Versions' }));
    const versionSheet = await screen.findByRole('dialog', { name: 'Operator additions' });
    await user.click(within(versionSheet).getByRole('button', { name: 'Inspect' }));
    const versionDialog = await screen.findByRole('dialog', { name: 'source_version_detail' });
    expect(await within(versionDialog).findByText('{"outbounds":[]}')).toBeInTheDocument();
    expect(sourcePanel).not.toContainElement(versionDialog);
    expect(getSubscriptionSourceVersion).toHaveBeenCalledWith(
      'source_local',
      'source_version_detail',
    );
  });
});
