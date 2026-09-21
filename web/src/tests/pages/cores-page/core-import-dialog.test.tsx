import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';

import '@/i18n';
import { toast } from '@/components/ui/toast-manager';
import { CoreImportDialog } from '@/pages/cores-page/core-import-dialog';

function renderImport(onImport = vi.fn().mockResolvedValue(true)) {
  const onClose = vi.fn();
  render(<CoreImportDialog architecture='arm64' onImport={onImport} onClose={onClose} />);
  return { onImport, onClose };
}

describe('compact core import', () => {
  it('only asks for an archive and version, with formats matching the backend', () => {
    renderImport();
    expect(screen.getAllByRole('textbox')).toHaveLength(1);
    expect(screen.getByRole('textbox', { name: 'Version' })).toBeVisible();
    expect(screen.queryByLabelText('Source description')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Variant')).not.toBeInTheDocument();
    expect(screen.getByLabelText('Archive')).toHaveAttribute('accept', '.tar.gz,.tgz');
    expect(screen.getByRole('button', { name: 'Import' })).toBeDisabled();
  });

  it.each(['sing-box-1.14.0-linux-arm64-musl.tar.gz', 'sing-box-v1.14.0.tgz'])(
    'fills the version from %s and supplies hidden metadata', async (name) => {
      const user = userEvent.setup();
      const { onImport, onClose } = renderImport();
      const archive = new File(['archive'], name);
      await user.upload(screen.getByLabelText('Archive'), archive);
      expect(screen.getByRole('textbox', { name: 'Version' })).toHaveValue('1.14.0');
      expect(screen.getByText(name)).toHaveTextContent(name);
      await user.click(screen.getByRole('button', { name: 'Import' }));
      expect(onImport).toHaveBeenCalledWith({
        archive, exactVersion: '1.14.0', architecture: 'arm64', sourceDescription: name, variant: 'musl',
      });
      expect(onClose).toHaveBeenCalledOnce();
    },
  );

  it('clears stale version guesses on file replacement and accepts a manual version with Enter', async () => {
    const user = userEvent.setup();
    const { onImport } = renderImport();
    const input = screen.getByLabelText('Archive');
    await user.upload(input, new File(['first'], 'sing-box-1.14.0-linux-arm64-musl.tar.gz'));
    const archive = new File(['custom'], 'custom-core.tgz');
    await user.upload(input, archive);
    const version = screen.getByRole('textbox', { name: 'Version' });
    expect(version).toHaveValue('');
    expect(screen.getByRole('button', { name: 'Import' })).toBeDisabled();
    await user.type(version, '1.13.19{Enter}');
    expect(onImport).toHaveBeenCalledWith(expect.objectContaining({ archive, exactVersion: '1.13.19' }));
  });

  it.each(['arm64', 'arm64-glibc', 'amd64-musl'])(
    'rejects a recognized incompatible build %s', async (platform) => {
      const user = userEvent.setup();
      const { onImport } = renderImport();
      await user.upload(screen.getByLabelText('Archive'), new File(['archive'], `sing-box-1.14.0-linux-${platform}.tar.gz`));
      expect(screen.getByText('Choose a musl archive matching the server architecture.')).toBeVisible();
      expect(screen.getByRole('button', { name: 'Import' })).toBeDisabled();
      expect(onImport).not.toHaveBeenCalled();
    },
  );

  it('does not guess a stable version from a prerelease filename', async () => {
    const user = userEvent.setup();
    renderImport();
    await user.upload(screen.getByLabelText('Archive'), new File(['archive'], 'sing-box-1.14.0-beta.1-linux-arm64-musl.tar.gz'));
    expect(screen.getByRole('textbox', { name: 'Version' })).toHaveValue('');
    expect(screen.getByRole('button', { name: 'Import' })).toBeDisabled();
  });

  it('accepts a dropped archive, fills its version and submits that file', async () => {
    const user = userEvent.setup();
    const { onImport } = renderImport();
    const input = screen.getByLabelText('Archive');
    const archive = new File(['archive'], 'sing-box-1.14.0-linux-arm64-musl.tar.gz');
    const dataTransfer = { files: [archive], types: ['Files'], dropEffect: 'none' };
    fireEvent.dragOver(input, { dataTransfer });
    expect(screen.getByText('Release to select the archive')).toBeVisible();
    expect(dataTransfer.dropEffect).toBe('copy');
    fireEvent.drop(input, { dataTransfer });
    expect(screen.getByText(archive.name)).toBeVisible();
    expect(screen.getByRole('textbox', { name: 'Version' })).toHaveValue('1.14.0');
    await user.click(screen.getByRole('button', { name: 'Import' }));
    expect(onImport).toHaveBeenCalledWith(expect.objectContaining({ archive, exactVersion: '1.14.0' }));
  });

  it('leaves the drop state when a drag exits and ignores non-file drags', () => {
    renderImport();
    const input = screen.getByLabelText('Archive');
    fireEvent.dragOver(input, { dataTransfer: { types: ['Files'] } });
    fireEvent.dragLeave(input);
    expect(screen.getByText('Drop your archive here')).toBeVisible();
    fireEvent.dragOver(input, { dataTransfer: { types: ['text/plain'] } });
    expect(screen.getByText('Drop your archive here')).toBeVisible();
  });

  it('rejects unsupported or multiple files and lets the user replace them', async () => {
    const user = userEvent.setup();
    renderImport();
    const input = screen.getByLabelText('Archive');
    const archive = new File(['archive'], 'sing-box-1.14.0.tgz');
    const addToast = vi.spyOn(toast, 'add');
    await user.upload(input, archive);
    fireEvent.drop(input, { dataTransfer: { files: [new File(['invalid'], 'core.zip')] } });
    await waitFor(() => expect(addToast).toHaveBeenLastCalledWith(expect.objectContaining({ type: 'error', title: 'Choose a .tar.gz or .tgz archive.' })));
    expect(input).toHaveAccessibleDescription(expect.stringContaining('Choose a .tar.gz or .tgz archive.'));
    expect(screen.getByRole('button', { name: 'Import' })).toBeDisabled();
    fireEvent.drop(input, { dataTransfer: { files: [archive, archive] } });
    await waitFor(() => expect(addToast).toHaveBeenLastCalledWith(expect.objectContaining({ type: 'error', title: 'Choose one archive at a time.' })));
    await user.upload(input, archive);
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Import' })).toBeEnabled();
    addToast.mockRestore();
  });

  it('locks submission while pending and preserves the selection after failure for retry', async () => {
    const user = userEvent.setup();
    let finish!: (success: boolean) => void;
    const { onImport, onClose } = renderImport(vi.fn().mockImplementation(() =>
      new Promise<boolean>((resolve) => {
        finish = resolve;
      }),
    ));
    await user.upload(screen.getByLabelText('Archive'), new File(['archive'], 'sing-box-1.14.0.tgz'));
    await user.click(screen.getByRole('button', { name: 'Import' }));
    expect(screen.getByRole('button', { name: 'Importing…' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled();
    expect(screen.getByRole('textbox', { name: 'Version' })).toBeDisabled();
    fireEvent.drop(screen.getByLabelText('Archive'), {
      dataTransfer: { files: [new File(['replacement'], 'sing-box-1.13.19.tgz')] },
    });
    expect(screen.getByRole('textbox', { name: 'Version' })).toHaveValue('1.14.0');
    await user.keyboard('{Escape}');
    expect(onClose).not.toHaveBeenCalled();
    finish(false);
    await waitFor(() => expect(screen.getByRole('button', { name: 'Import' })).toBeEnabled());
    expect(screen.getByRole('textbox', { name: 'Version' })).toHaveValue('1.14.0');
    expect(onImport).toHaveBeenCalledOnce();
    expect(onClose).not.toHaveBeenCalled();
  });
});
