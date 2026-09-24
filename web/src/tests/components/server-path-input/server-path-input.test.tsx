import { useState } from 'react';
import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';

import type { FilesystemMode, FilesystemPage } from '@/api/api-client';

import '@/i18n';
import { ApiRequestError } from '@/api/api-client';
import { ApiClientProvider } from '@/api/api-client-context';
import { ServerPathInput } from '@/components/server-path-input';
import { createMockApiClient } from '@/tests/api/mock-api-client';
import { createDemoFilesystemApi } from '@/api/demo/demo-filesystem';
import { Dialog, DialogContent, DialogTitle } from '@/components/ui/dialog';

function Harness({ mode = 'file', initial = '', readOnly = false }: { mode?: FilesystemMode; initial?: string; readOnly?: boolean }) {
  const [value, setValue] = useState(initial);
  return <ServerPathInput aria-label='Certificate path' value={value} mode={mode} readOnly={readOnly} onValueChange={setValue} />;
}

function setup(mode: FilesystemMode = 'file', initial = '') {
  const api = createMockApiClient(createDemoFilesystemApi());
  api.resolveFilesystemPath = vi.fn(api.resolveFilesystemPath);
  api.listFilesystemEntries = vi.fn(api.listFilesystemEntries);
  render(<ApiClientProvider client={api}><Harness mode={mode} initial={initial} /></ApiClientProvider>);
  return { api, user: userEvent.setup() };
}

async function openPicker(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole('button', { name: 'Browse server path: Certificate path' }));
  await waitFor(() => expect(screen.queryByText('Loading directory…')).not.toBeInTheDocument());
  return within(screen.getByRole('dialog'));
}

describe('server path input', () => {
  it('browses and revalidates a file before replacing the field, and returns focus', async () => {
    const { api, user } = setup('file', '/etc/sing-box/certificate.pem');
    const dialog = await openPicker(user);
    expect(dialog.queryByRole('button', { name: 'Cancel' })).not.toBeInTheDocument();
    expect(dialog.getByRole('button', { name: 'Confirm' })).toBeDisabled();
    expect(dialog.queryByText('Select a path on the panel server.')).not.toBeInTheDocument();
    await user.click(dialog.getByRole('button', { name: 'private.key' }));
    await user.click(dialog.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(screen.getByLabelText('Certificate path')).toHaveValue('/etc/sing-box/private.key'));
    expect(api.resolveFilesystemPath).toHaveBeenCalledWith({ path: '/etc/sing-box/private.key', mode: 'file' }, expect.any(AbortSignal));
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(screen.getByRole('button', { name: 'Browse server path: Certificate path' })).toHaveFocus();
  });

  it.each(['file', 'directory', 'output-file', 'socket'] as const)('retains typed input when the %s picker closes without confirming', async mode => {
    const { api, user } = setup(mode, '/unchanged.pem');
    const dialog = await openPicker(user);
    await user.dblClick(dialog.getByRole('button', { name: 'var/' }));
    await dialog.findByRole('button', { name: 'lib/' });
    await user.click(dialog.getByRole('button', { name: 'Close' }));
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(screen.getByLabelText('Certificate path')).toHaveValue('/unchanged.pem');
    expect(api.resolveFilesystemPath).not.toHaveBeenCalled();
    expect(screen.getByRole('button', { name: 'Browse server path: Certificate path' })).toHaveFocus();
    await user.type(screen.getByLabelText('Certificate path'), '.new');
    expect(screen.getByLabelText('Certificate path')).toHaveValue('/unchanged.pem.new');
  });

  it('renders disabled browsing for a read-only input', () => {
    render(<Harness readOnly initial='/keep' />);
    expect(screen.getByRole('button', { name: 'Browse server path: Certificate path' })).toBeDisabled();
    expect(screen.getByLabelText('Certificate path')).toHaveAttribute('readonly');
  });

  it('selects an existing directory and navigates with the keyboard', async () => {
    const { user } = setup('directory');
    const dialog = await openPicker(user);
    const folder = dialog.getByRole('button', { name: 'logs/' });
    folder.focus();
    await user.keyboard('{Enter}');
    await waitFor(() => expect(dialog.getByRole('navigation', { name: 'Directory breadcrumb' })).toHaveAttribute('title', '/var/lib/sing-box-panel/runtime/logs'));
    await user.click(dialog.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(screen.getByLabelText('Certificate path')).toHaveValue('/var/lib/sing-box-panel/runtime/logs'));
  });

  it('selects a directory without entering it and waits for explicit confirmation', async () => {
    const { api, user } = setup('directory');
    const dialog = await openPicker(user);
    const folder = dialog.getByRole('button', { name: 'rules/' });
    await user.click(folder);
    expect(folder).toHaveAttribute('aria-pressed', 'true');
    expect(dialog.queryByRole('button', { name: 'Select directory: rules' })).not.toBeInTheDocument();
    expect(api.listFilesystemEntries).toHaveBeenCalledTimes(1);
    expect(api.resolveFilesystemPath).not.toHaveBeenCalled();
    expect(screen.getByLabelText('Certificate path')).toHaveValue('');
    await user.click(dialog.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(screen.getByLabelText('Certificate path')).toHaveValue('/var/lib/sing-box-panel/runtime/rules'));
    expect(api.resolveFilesystemPath).toHaveBeenCalledWith({ path: '/var/lib/sing-box-panel/runtime/rules', mode: 'directory' }, expect.any(AbortSignal));
  });

  it('confirms the root directory when no child is selected', async () => {
    const { api, user } = setup('directory', '/');
    const dialog = await openPicker(user);
    await user.click(dialog.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(api.resolveFilesystemPath).toHaveBeenCalledWith({ path: '/', mode: 'directory' }, expect.any(AbortSignal));
  });

  it('blocks row confirmation while editing a directory path', async () => {
    const { api, user } = setup('directory');
    const dialog = await openPicker(user);
    await user.click(dialog.getByRole('button', { name: 'Edit path' }));
    expect(dialog.getByRole('button', { name: 'logs/' })).toBeDisabled();
    expect(dialog.getByRole('button', { name: 'Confirm' })).toBeDisabled();
    await user.keyboard('{Escape}');
    expect(dialog.getByRole('button', { name: 'Confirm' })).toBeEnabled();
    expect(api.resolveFilesystemPath).not.toHaveBeenCalled();
  });

  it('moves through selectable entries with arrow, Home and End keys', async () => {
    const { user } = setup('file');
    const dialog = await openPicker(user);
    const first = dialog.getByRole('button', { name: 'logs/' });
    const second = dialog.getByRole('button', { name: 'rules/' });
    const last = dialog.getByRole('button', { name: 'cache.db' });
    first.focus();
    await user.keyboard('{ArrowDown}');
    expect(second).toHaveFocus();
    await user.keyboard('{End}');
    expect(last).toHaveFocus();
    await user.keyboard('{ArrowUp}');
    expect(second).toHaveFocus();
    await user.keyboard('{Home}');
    expect(first).toHaveFocus();
  });

  it('accepts new output paths in the form field without a filename area in the picker', async () => {
    const { api, user } = setup('output-file');
    const path = '/var/lib/sing-box-panel/runtime/logs/新 日志.log';
    await user.type(screen.getByLabelText('Certificate path'), path);
    const dialog = await openPicker(user);
    expect(dialog.queryByText('File name')).not.toBeInTheDocument();
    expect(dialog.queryByText('Choose a directory and enter a file name without /. This selects a path without creating a file.')).not.toBeInTheDocument();
    expect(dialog.getAllByRole('textbox')).toEqual([dialog.getByLabelText('Filter names in this directory')]);
    expect(dialog.getByRole('navigation', { name: 'Directory breadcrumb' })).toHaveAttribute('title', '/var/lib/sing-box-panel/runtime/logs');
    expect(dialog.getByRole('button', { name: 'Confirm' })).toBeDisabled();
    await user.click(dialog.getByRole('button', { name: 'Close' }));
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(screen.getByLabelText('Certificate path')).toHaveValue(path);
    expect(api.resolveFilesystemPath).not.toHaveBeenCalled();
  });

  it('filters names and exposes hidden files', async () => {
    const { api, user } = setup();
    const dialog = await openPicker(user);
    expect(dialog.queryByRole('button', { name: '.hidden' })).not.toBeInTheDocument();
    await user.click(dialog.getByRole('button', { name: 'Show hidden' }));
    await waitFor(() => expect(dialog.getByRole('button', { name: '.hidden' })).toBeVisible());
    await user.type(dialog.getByLabelText('Filter names in this directory'), 'cache');
    await waitFor(() => {
      expect(dialog.queryByRole('button', { name: 'logs/' })).not.toBeInTheDocument();
      expect(dialog.getByRole('button', { name: 'cache.db' })).toBeVisible();
    });
    expect(api.listFilesystemEntries).toHaveBeenLastCalledWith(expect.objectContaining({ search: 'cache', show_hidden: true, offset: 0 }), expect.any(AbortSignal));
  });

  it('requests the next page in the resolved directory and updates the visible entries', async () => {
    const { api, user } = setup('file', '/etc/sing-box/certificate.pem');
    const page: FilesystemPage = {
      path: '/etc/sing-box', parent: '/etc', requested_path: '/etc/sing-box/certificate.pem', fallback: false,
      items: [{ name: 'first.pem', path: '/etc/sing-box/first.pem', kind: 'file', available: true, symlink: false }],
      total: 11, limit: 10, offset: 0,
    };
    api.listFilesystemEntries.mockResolvedValueOnce(page).mockResolvedValueOnce({ ...page, offset: 10, items: [
      { name: 'last.pem', path: '/etc/sing-box/last.pem', kind: 'file', available: true, symlink: false },
    ] });
    const dialog = await openPicker(user);
    await user.click(dialog.getByRole('button', { name: 'Next page' }));
    expect(await dialog.findByRole('button', { name: 'last.pem' })).toBeVisible();
    expect(dialog.queryByRole('button', { name: 'first.pem' })).not.toBeInTheDocument();
    expect(api.listFilesystemEntries).toHaveBeenLastCalledWith(expect.objectContaining({ path: '/etc/sing-box', offset: 10 }), expect.any(AbortSignal));
    expect(dialog.getByRole('spinbutton', { name: 'Current page' })).toHaveValue(2);
  });

  it('keeps a pending navigation when filtering, toggling hidden files or refreshing', async () => {
    const { api, user } = setup('file', '/etc/sing-box/certificate.pem');
    const dialog = await openPicker(user);
    api.listFilesystemEntries.mockImplementation(() => new Promise(() => {}));
    await user.click(dialog.getByRole('button', { name: 'Edit path' }));
    fireEvent.change(dialog.getByLabelText('Location'), { target: { value: '/new-directory' } });
    await user.click(dialog.getByRole('button', { name: 'Go' }));
    await user.type(dialog.getByLabelText('Filter names in this directory'), 'cert');
    await waitFor(() => expect(api.listFilesystemEntries).toHaveBeenLastCalledWith(
      expect.objectContaining({ path: '/new-directory', search: 'cert' }), expect.any(AbortSignal),
    ));
    await user.click(dialog.getByRole('button', { name: 'Show hidden' }));
    await user.click(dialog.getByRole('button', { name: 'Refresh directory' }));
    expect(api.listFilesystemEntries).toHaveBeenLastCalledWith(
      expect.objectContaining({ path: '/new-directory', search: 'cert', show_hidden: true }), expect.any(AbortSignal),
    );
    expect(dialog.getByRole('combobox', { name: 'Items per page' })).toBeDisabled();
    await user.click(dialog.getByRole('button', { name: 'Close' }));
    expect(api.listFilesystemEntries.mock.calls.at(-1)![1]?.aborted).toBe(true);
  });

  it('allows retry after a permission error without changing the draft', async () => {
    const { api, user } = setup('file', '/etc/sing-box/certificate.pem');
    api.listFilesystemEntries.mockRejectedValueOnce(new ApiRequestError('denied', { code: 'filesystem_forbidden', status: 403 }));
    const dialog = await openPicker(user);
    expect(dialog.getByRole('alert')).toHaveTextContent('permission');
    expect(screen.getByLabelText('Certificate path')).toHaveValue('/etc/sing-box/certificate.pem');
    await user.click(dialog.getByRole('button', { name: 'Refresh directory' }));
    expect(await dialog.findByRole('button', { name: 'private.key' })).toBeVisible();
    expect(dialog.queryByRole('alert')).not.toBeInTheDocument();
  });

  it.each([
    ['file', '/etc/sing-box/certificate.pem', 'private.key'],
    ['socket', '/var/lib/sing-box-panel/runtime', 'protect.sock'],
    ['output-file', '/var/lib/sing-box-panel/runtime', 'cache.db'],
    ['directory', '/var/lib/sing-box-panel/runtime', 'logs/'],
  ] as const)('keeps the dialog and draft after a %s fails validation and allows retry', async (mode, initial, button) => {
    const { api, user } = setup(mode, initial);
    api.resolveFilesystemPath.mockRejectedValueOnce(new ApiRequestError('gone', { code: 'filesystem_not_found', status: 404 }));
    const dialog = await openPicker(user);
    await user.click(dialog.getByRole('button', { name: button }));
    await user.click(dialog.getByRole('button', { name: 'Confirm' }));
    expect(await dialog.findByRole('alert')).toHaveTextContent('no longer exists');
    expect(screen.getByLabelText('Certificate path')).toHaveValue(initial);
    await user.click(dialog.getByRole('button', { name: button }));
    await user.click(dialog.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(api.resolveFilesystemPath).toHaveBeenCalledTimes(2);
  });

  it('ignores late directory responses and cancels pending confirmation when closed', async () => {
    const { api, user } = setup();
    let resolveOld!: (page: FilesystemPage) => void;
    api.listFilesystemEntries.mockImplementationOnce(() => new Promise(resolve => {
      resolveOld = resolve;
    }));
    await user.click(screen.getByRole('button', { name: 'Browse server path: Certificate path' }));
    const dialog = within(screen.getByRole('dialog'));
    await user.click(dialog.getByRole('button', { name: 'Edit path' }));
    fireEvent.change(dialog.getByLabelText('Location'), { target: { value: '/etc/sing-box' } });
    await user.click(dialog.getByRole('button', { name: 'Go' }));
    await dialog.findByRole('button', { name: 'private.key' });
    expect(api.listFilesystemEntries.mock.calls[0][1]?.aborted).toBe(true);
    await act(async () => resolveOld({ path: '/stale', parent: '/', requested_path: '/stale', fallback: false, items: [], total: 0, offset: 0, limit: 10 }));
    expect(dialog.getByRole('navigation', { name: 'Directory breadcrumb' })).toHaveAttribute('title', '/etc/sing-box');
    let resolveSelection!: (value: Awaited<ReturnType<typeof api.resolveFilesystemPath>>) => void;
    api.resolveFilesystemPath.mockImplementation(() => new Promise(resolve => {
      resolveSelection = resolve;
    }));
    await user.click(dialog.getByRole('button', { name: 'private.key' }));
    await user.click(dialog.getByRole('button', { name: 'Confirm' }));
    expect(dialog.getByRole('button', { name: 'private.key' })).toBeDisabled();
    expect(dialog.getByRole('button', { name: 'Confirm' })).toHaveAttribute('aria-busy', 'true');
    expect(dialog.getByRole('button', { name: 'Parent directory' })).toBeDisabled();
    expect(screen.getByLabelText('Certificate path')).toHaveValue('');
    await user.dblClick(dialog.getByRole('button', { name: 'private.key' }));
    expect(api.resolveFilesystemPath).toHaveBeenCalledTimes(1);
    await user.click(dialog.getByRole('button', { name: 'Close' }));
    expect(api.resolveFilesystemPath.mock.calls[0][1]?.aborted).toBe(true);
    await act(async () => resolveSelection({ path: '/late', parent: '/', kind: 'file', exists: true, symlink: false }));
    expect(screen.getByLabelText('Certificate path')).toHaveValue('');
  });

  it('escape closes only the nested picker and restores focus to its trigger', async () => {
    const api = createMockApiClient(createDemoFilesystemApi());
    render(
      <ApiClientProvider client={api}>
        <Dialog open>
          <DialogContent>
            <DialogTitle>Node editor</DialogTitle>
            <Harness />
          </DialogContent>
        </Dialog>
      </ApiClientProvider>,
    );
    const user = userEvent.setup();
    await user.click(screen.getByRole('button', { name: 'Browse server path: Certificate path' }));
    await screen.findByRole('dialog', { name: 'Select server file' });
    await user.keyboard('{Escape}');
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Select server file' })).not.toBeInTheDocument());
    expect(screen.getByRole('dialog', { name: 'Node editor' })).toBeVisible();
    expect(screen.getByRole('button', { name: 'Browse server path: Certificate path' })).toHaveFocus();
  });

  it('edits the path in place, cancels with Escape and returns to breadcrumbs after Enter', async () => {
    const { api, user } = setup('directory', '/etc/sing-box');
    const dialog = await openPicker(user);
    expect(dialog.queryByRole('textbox', { name: 'Location' })).not.toBeInTheDocument();
    await user.click(dialog.getByRole('button', { name: 'Edit path' }));
    expect(dialog.getByRole('button', { name: 'Confirm' })).toBeDisabled();
    expect(dialog.queryByRole('navigation', { name: 'Directory breadcrumb' })).not.toBeInTheDocument();
    expect(dialog.getByRole('textbox', { name: 'Location' })).toHaveFocus();
    await user.clear(dialog.getByRole('textbox', { name: 'Location' }));
    await user.type(dialog.getByRole('textbox', { name: 'Location' }), '/discard');
    await user.keyboard('{Escape}');
    expect(screen.getByRole('dialog')).toBeVisible();
    expect(dialog.getByRole('button', { name: 'Edit path' })).toHaveFocus();
    expect(api.listFilesystemEntries).toHaveBeenCalledTimes(1);
    await user.click(dialog.getByRole('button', { name: 'Edit path' }));
    expect(dialog.getByRole('textbox', { name: 'Location' })).toHaveValue('/etc/sing-box');
    await user.clear(dialog.getByRole('textbox', { name: 'Location' }));
    await user.type(dialog.getByRole('textbox', { name: 'Location' }), '/var{Enter}');
    expect(await dialog.findByRole('button', { name: 'lib/' })).toBeVisible();
    expect(dialog.queryByRole('textbox', { name: 'Location' })).not.toBeInTheDocument();
    expect(dialog.getByRole('navigation', { name: 'Directory breadcrumb' })).toHaveAttribute('title', '/var');
    expect(screen.getByLabelText('Certificate path')).toHaveValue('/etc/sing-box');
  });

  it.each([
    ['socket', 'protect.sock'],
    ['output-file', 'cache.db'],
  ] as const)('selects an existing %s and confirms it from the toolbar', async (mode, name) => {
    const { api, user } = setup(mode);
    const dialog = await openPicker(user);
    if (mode === 'socket') expect(dialog.getByRole('button', { name: 'cache.db' })).toBeDisabled();
    await user.click(dialog.getByRole('button', { name }));
    expect(api.resolveFilesystemPath).not.toHaveBeenCalled();
    expect(dialog.getByRole('button', { name })).toHaveAttribute('aria-pressed', 'true');
    await user.click(dialog.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(screen.getByLabelText('Certificate path')).toHaveValue(`/var/lib/sing-box-panel/runtime/${name}`));
    expect(api.resolveFilesystemPath).toHaveBeenCalledWith({ path: `/var/lib/sing-box-panel/runtime/${name}`, mode }, expect.any(AbortSignal));
  });

  it('confirms the visible file after filtering', async () => {
    const { api, user } = setup('file', '/etc/sing-box/certificate.pem');
    const dialog = await openPicker(user);
    await user.type(dialog.getByLabelText('Filter names in this directory'), 'certificate');
    await waitFor(() => {
      expect(dialog.queryByRole('button', { name: 'private.key' })).not.toBeInTheDocument();
      expect(dialog.getByRole('button', { name: 'certificate.pem' })).toBeVisible();
    });
    expect(api.resolveFilesystemPath).not.toHaveBeenCalled();
    await user.click(dialog.getByRole('button', { name: 'certificate.pem' }));
    await user.click(dialog.getByRole('button', { name: 'Confirm' }));
    expect(api.resolveFilesystemPath).toHaveBeenCalledWith({ path: '/etc/sing-box/certificate.pem', mode: 'file' }, expect.any(AbortSignal));
  });

  it('clears the selected output file when navigating to another directory', async () => {
    const { api, user } = setup('output-file', '/var/lib/sing-box-panel/runtime/cache.db');
    const dialog = await openPicker(user);
    expect(dialog.getByRole('button', { name: 'Confirm' })).toBeDisabled();
    await user.click(dialog.getByRole('button', { name: 'cache.db' }));
    expect(dialog.getByRole('button', { name: 'Confirm' })).toBeEnabled();
    await user.dblClick(dialog.getByRole('button', { name: 'logs/' }));
    await waitFor(() => expect(dialog.getByRole('navigation', { name: 'Directory breadcrumb' })).toHaveAttribute('title', '/var/lib/sing-box-panel/runtime/logs'));
    expect(dialog.getByRole('button', { name: 'Confirm' })).toBeDisabled();
    expect(screen.getByLabelText('Certificate path')).toHaveValue('/var/lib/sing-box-panel/runtime/cache.db');
    expect(api.resolveFilesystemPath).not.toHaveBeenCalled();
  });

  it.each(['breadcrumb', 'parent'])('returns from an unreadable directory using the %s', async navigation => {
    const { api, user } = setup('directory', '/etc');
    const dialog = await openPicker(user);
    api.listFilesystemEntries.mockRejectedValueOnce(new ApiRequestError('denied', { code: 'filesystem_forbidden', status: 403 }));
    await user.dblClick(dialog.getByRole('button', { name: 'sing-box/' }));
    expect(await dialog.findByRole('alert')).toHaveTextContent('permission');
    expect(dialog.getByRole('button', { name: 'Confirm' })).toBeDisabled();
    const back = navigation === 'parent'
      ? dialog.getByRole('button', { name: 'Parent directory' })
      : within(dialog.getByRole('navigation', { name: 'Directory breadcrumb' })).getByRole('button', { name: 'etc' });
    expect(back).toBeEnabled();
    await user.click(back);
    expect(await dialog.findByRole('button', { name: 'sing-box/' })).toBeVisible();
    expect(dialog.queryByRole('alert')).not.toBeInTheDocument();
    expect(api.resolveFilesystemPath).not.toHaveBeenCalled();
  });

  it('restores keyboard focus after entering and leaving a directory', async () => {
    const { api, user } = setup('file', '/etc');
    const dialog = await openPicker(user);
    const childPage = await createDemoFilesystemApi().listFilesystemEntries({ path: '/etc/sing-box', limit: 10 }, undefined);
    let resolvePage!: (page: FilesystemPage) => void;
    api.listFilesystemEntries.mockImplementationOnce(() => new Promise(resolve => {
      resolvePage = resolve;
    }));
    dialog.getByRole('button', { name: 'sing-box/' }).focus();
    await user.keyboard('{Enter}');
    expect(document.activeElement).toHaveAttribute('aria-busy', 'true');
    await act(async () => resolvePage(childPage));
    const first = await dialog.findByRole('button', { name: 'certificate.pem' });
    await waitFor(() => expect(first).toHaveFocus());
    await user.keyboard('{ArrowDown}');
    expect(dialog.getByRole('button', { name: 'private.key' })).toHaveFocus();
    await user.keyboard('{ArrowLeft}');
    await waitFor(() => expect(dialog.getByRole('button', { name: 'sing-box/' })).toHaveFocus());
  });

  it('focuses a usable control after keyboard navigation to an empty directory', async () => {
    const { user } = setup('directory');
    const dialog = await openPicker(user);
    dialog.getByRole('button', { name: 'logs/' }).focus();
    await user.keyboard('{Enter}');
    await waitFor(() => expect(dialog.getByRole('button', { name: 'Edit path' })).toHaveFocus());
    expect(dialog.getByRole('button', { name: 'Confirm' })).toBeEnabled();
  });

  it.each(['file', 'output-file'] as const)('does not confirm on double-clicking a %s and clears hidden selections after filtering', async mode => {
    const { api, user } = setup(mode, '/etc/sing-box');
    const dialog = await openPicker(user);
    await user.dblClick(dialog.getByRole('button', { name: 'private.key' }));
    expect(dialog.getByRole('button', { name: 'private.key' })).toHaveAttribute('aria-pressed', 'true');
    expect(api.resolveFilesystemPath).not.toHaveBeenCalled();
    expect(screen.getByLabelText('Certificate path')).toHaveValue('/etc/sing-box');
    await user.type(dialog.getByLabelText('Filter names in this directory'), 'certificate');
    await waitFor(() => {
      expect(dialog.queryByRole('button', { name: 'private.key' })).not.toBeInTheDocument();
      expect(dialog.getByRole('button', { name: 'certificate.pem' })).toBeVisible();
    });
    expect(dialog.getByRole('button', { name: 'Confirm' })).toBeDisabled();
    await user.click(dialog.getByRole('button', { name: 'Close' }));
    expect(api.resolveFilesystemPath).not.toHaveBeenCalled();
  });
});
