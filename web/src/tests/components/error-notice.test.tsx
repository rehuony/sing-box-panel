import { StrictMode } from 'react';
import { describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';

import '@/i18n';
import { Toaster } from '@/components/ui/toast';
import { toast } from '@/components/ui/toast-manager';
import { ErrorNotice } from '@/components/error-notice';

function Notification({ message }: { message: string | null }) {
  return (
    <Toaster>
      <StrictMode>
        <main data-testid='content'>
          <p>Existing content</p>
          <ErrorNotice error={message === null ? null : new Error(message)} />
        </main>
      </StrictMode>
    </Toaster>
  );
}

describe('error notifications', () => {
  it('keeps errors outside page flow, deduplicates repeated failures and closes on recovery', async () => {
    const addToast = vi.spyOn(toast, 'add');
    const { rerender } = render(<Notification message='Connection failed' />);
    const notice = await screen.findByText('Connection failed', { selector: '[data-slot="toast-title"]' });
    expect(notice.closest('[data-slot="toast"]')).toHaveAttribute('data-type', 'error');
    expect(screen.getByTestId('content').children).toHaveLength(1);
    expect(screen.getByTestId('content')).not.toHaveTextContent('Connection failed');
    rerender(<Notification message='Connection failed' />);
    expect(addToast).toHaveBeenCalledTimes(1);
    rerender(<Notification message='Request failed' />);
    expect(await screen.findByText('Request failed', { selector: '[data-slot="toast-title"]' })).toBeInTheDocument();
    expect(document.querySelectorAll('[data-slot="toast"]')).toHaveLength(1);
    rerender(<Notification message={null} />);
    await waitFor(() => expect(screen.queryByText('Request failed', { selector: '[data-slot="toast-title"]' })).not.toBeInTheDocument());
    addToast.mockRestore();
  });

  it('retains accessible field descriptions without an in-flow error alert', () => {
    render(
      <>
        <input aria-label='Name' aria-invalid aria-describedby='name-error' />
        <ErrorNotice id='name-error' error='A name is required' />
      </>,
    );
    expect(screen.getByRole('textbox', { name: 'Name' })).toHaveAccessibleDescription('A name is required');
    expect(screen.getByText('A name is required')).toHaveClass('sr-only');
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });
});
