import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';

import '@/i18n';
import { InfoTooltip } from '@/components/info-tooltip';
import { Dialog, DialogContent, DialogTitle } from '@/components/ui/dialog';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';

const label = 'Help for server';
const description = 'Enter a hostname or IP address.';
const portDescription = 'Enter a port between 1 and 65535.';

function setup() {
  const user = userEvent.setup({ skipHover: true });
  const submit = vi.fn(event => event.preventDefault());
  render(
    <TooltipProvider delay={200} closeDelay={100}>
      <form onSubmit={submit}>
        <InfoTooltip label={label}>{description}</InfoTooltip>
        <InfoTooltip label='Help for port'>{portDescription}</InfoTooltip>
        <Tooltip>
          <TooltipTrigger>Action</TooltipTrigger>
          <TooltipContent>Action help</TooltipContent>
        </Tooltip>
        <button type='button'>Outside</button>
      </form>
    </TooltipProvider>,
  );
  return { user, submit, trigger: screen.getByRole('button', { name: label }) };
}

describe('info tooltip', () => {
  it('opens immediately on click before the hover delay without submitting the form', async () => {
    const { user, trigger, submit } = setup();
    fireEvent.mouseEnter(trigger);
    fireEvent.mouseMove(trigger);
    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument();
    fireEvent.pointerDown(trigger, { pointerType: 'mouse' });
    fireEvent.click(trigger);
    expect(screen.getByRole('tooltip')).toHaveTextContent(description);
    expect(submit).not.toHaveBeenCalled();
    await user.click(trigger);
    expect(screen.getByRole('tooltip')).toBeVisible();
    await user.click(screen.getByRole('button', { name: 'Outside' }));
    await waitFor(() => expect(screen.queryByRole('tooltip')).not.toBeInTheDocument());
  });

  it('does not reopen a pending hover after clicking another field', async () => {
    vi.useFakeTimers();
    const { trigger } = setup();
    fireEvent.mouseEnter(trigger);
    fireEvent.mouseMove(trigger);
    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument();
    fireEvent.click(trigger);
    expect(screen.getByRole('tooltip')).toHaveTextContent(description);
    fireEvent.mouseLeave(trigger);
    const other = screen.getByRole('button', { name: 'Help for port' });
    fireEvent.mouseEnter(other);
    fireEvent.mouseMove(other);
    fireEvent.click(other);
    await act(() => vi.advanceTimersByTimeAsync(300));
    expect(screen.getAllByRole('tooltip')).toHaveLength(1);
    expect(screen.getByRole('tooltip')).toHaveTextContent(portDescription);
    expect(screen.queryByText(description)).not.toBeInTheDocument();
  });

  it('skips help icons in forward and backward Tab navigation', async () => {
    const { user, trigger } = setup();
    await user.tab();
    expect(screen.getByRole('button', { name: 'Action' })).toHaveFocus();
    expect(trigger).not.toHaveFocus();
    expect(screen.getByRole('button', { name: 'Help for port' })).not.toHaveFocus();
    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument();
    await user.tab();
    expect(screen.getByRole('button', { name: 'Outside' })).toHaveFocus();
    await user.tab({ shift: true });
    expect(screen.getByRole('button', { name: 'Action' })).toHaveFocus();
    await user.tab({ shift: true });
    expect(document.body).toHaveFocus();
  });

  it('opens on touch and dismisses on outside press', async () => {
    const { user, trigger } = setup();
    await user.pointer({ keys: '[TouchA]', target: trigger });
    expect(screen.getByRole('tooltip')).toHaveTextContent(description);
    await user.pointer({ keys: '[TouchA]', target: screen.getByRole('button', { name: 'Help for port' }) });
    expect(screen.getAllByRole('tooltip')).toHaveLength(1);
    expect(screen.getByRole('tooltip')).toHaveTextContent(portDescription);
    await user.pointer({ keys: '[TouchA]', target: screen.getByRole('button', { name: 'Outside' }) });
    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument();
  });

  it('dismisses help inside an editor dialog without closing the editor', async () => {
    const user = userEvent.setup();
    render(
      <TooltipProvider>
        <Dialog defaultOpen>
          <DialogContent>
            <DialogTitle>Edit server</DialogTitle>
            <InfoTooltip label={label}>{description}</InfoTooltip>
          </DialogContent>
        </Dialog>
      </TooltipProvider>,
    );
    await user.click(screen.getByRole('button', { name: label }));
    expect(screen.getByRole('tooltip')).toBeVisible();
    await user.keyboard('{Escape}');
    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument();
    expect(screen.getByRole('dialog', { name: 'Edit server' })).toBeVisible();
    expect(screen.getByRole('button', { name: label })).toHaveFocus();
  });
});
