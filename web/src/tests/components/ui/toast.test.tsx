import { Toast } from '@base-ui/react/toast';
import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import { act, render, screen, waitFor } from '@testing-library/react';

import '@/i18n';
import { Toaster } from '@/components/ui/toast';

describe('toast feedback', () => {
  it.each(['success', 'error'])('automatically dismisses %s feedback after three seconds', async (type) => {
    vi.useFakeTimers();
    try {
      const manager = Toast.createToastManager();
      const view = render(<Toaster toastManager={manager} />);
      await act(async () => {
        manager.add({ title: 'Timed feedback', type });
      });
      expect(screen.getByText('Timed feedback')).toBeInTheDocument();
      await act(async () => {
        await vi.advanceTimersByTimeAsync(2999);
      });
      expect(screen.getByText('Timed feedback')).toBeInTheDocument();
      await act(async () => {
        await vi.advanceTimersByTimeAsync(1);
      });
      expect(screen.queryByText('Timed feedback')).not.toBeInTheDocument();
      view.unmount();
    } finally {
      vi.useRealTimers();
    }
  });

  it.each(['success', 'info', 'warning', 'error'])('keeps %s feedback dismissible and exposes its semantic type', async (type) => {
    const manager = Toast.createToastManager();
    render(<Toaster toastManager={manager} />);
    act(() => {
      manager.add({ title: 'Result', description: 'Operation details', type, timeout: 0 });
    });
    const title = await screen.findByText('Result');
    const root = title.closest('[data-slot="toast"]');
    expect(root).toHaveAttribute('data-type', type);
    expect(root?.querySelector('[data-slot="toast-icon"] svg')).toBeInTheDocument();
    expect(screen.getByText('Operation details')).toBeInTheDocument();
    await userEvent.hover(screen.getByRole('region', { name: 'Notifications' }));
    await userEvent.click(await screen.findByRole('button', { name: 'Close' }));
    await waitFor(() => expect(screen.queryByText('Result')).not.toBeInTheDocument());
  });

  it('updates the same loading notification when an operation completes', async () => {
    const manager = Toast.createToastManager();
    render(<Toaster toastManager={manager} />);
    let id = '';
    act(() => {
      id = manager.add({ title: 'Working', type: 'loading', timeout: 0 });
    });
    expect((await screen.findByText('Working')).closest('[data-slot="toast"]')).toHaveAttribute('data-type', 'loading');
    act(() => {
      manager.update(id, { title: 'Finished', type: 'success' });
    });
    expect((await screen.findByText('Finished')).closest('[data-slot="toast"]')).toHaveAttribute('data-type', 'success');
    expect(screen.queryByText('Working')).not.toBeInTheDocument();
    expect(document.querySelectorAll('[data-slot="toast"]')).toHaveLength(1);
  });
});
