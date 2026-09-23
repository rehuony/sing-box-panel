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
  it('previews on a brief hover without stealing focus and allows reading inside the tooltip', async () => {
    const { user, trigger } = setup();
    await user.hover(trigger);
    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument();
    const tooltip = await screen.findByRole('tooltip');
    expect(tooltip).toHaveTextContent(description);
    expect(tooltip).not.toHaveFocus();
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    await user.hover(tooltip);
    await act(() => new Promise(resolve => setTimeout(resolve, 250)));
    expect(tooltip).toBeVisible();
    await user.hover(screen.getByRole('button', { name: 'Outside' }));
    await waitFor(() => expect(screen.queryByRole('tooltip')).not.toBeInTheDocument());
  });

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

  it('does not dismiss a hover preview on click and closes after leaving', async () => {
    const { user, trigger } = setup();
    await user.hover(trigger);
    expect(await screen.findByRole('tooltip')).toBeVisible();
    await user.click(trigger);
    expect(screen.getByRole('tooltip')).toBeVisible();
    await user.hover(screen.getByRole('button', { name: 'Outside' }));
    await waitFor(() => expect(screen.queryByRole('tooltip')).not.toBeInTheDocument());
  });

  it('replaces clicked help when hovering a neighboring field instead of stacking windows', async () => {
    const { user, trigger } = setup();
    await user.click(trigger);
    expect(screen.getByRole('tooltip')).toHaveTextContent(description);
    await user.hover(screen.getByRole('button', { name: 'Help for port' }));
    await waitFor(() => {
      expect(screen.getAllByRole('tooltip')).toHaveLength(1);
      expect(screen.getByRole('tooltip')).toHaveTextContent(portDescription);
    });
    expect(screen.queryByText(description)).not.toBeInTheDocument();
    await user.hover(screen.getByRole('button', { name: 'Action' }));
    await waitFor(() => {
      expect(screen.queryByRole('tooltip')).not.toBeInTheDocument();
      expect(screen.getByText('Action help')).toBeVisible();
    });
  });

  it('does not reopen the previous hint when a pending hover expires after switching fields', async () => {
    const { user, trigger } = setup();
    await user.hover(trigger);
    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument();
    await user.click(trigger);
    expect(screen.getByRole('tooltip')).toHaveTextContent(description);
    await user.hover(screen.getByRole('button', { name: 'Help for port' }));
    await act(() => new Promise(resolve => setTimeout(resolve, 300)));
    expect(screen.getAllByRole('tooltip')).toHaveLength(1);
    expect(screen.getByRole('tooltip')).toHaveTextContent(portDescription);
    expect(screen.queryByText(description)).not.toBeInTheDocument();
  });

  it.each(['{Enter}', ' '])('supports keyboard focus, %s and Escape while keeping focus on the trigger', async key => {
    const { user, trigger } = setup();
    await user.tab();
    expect(trigger).toHaveFocus();
    expect(screen.getByRole('tooltip')).toHaveTextContent(description);
    expect(trigger).toHaveAccessibleDescription(description);
    await user.keyboard(key);
    expect(screen.getByRole('tooltip')).toBeVisible();
    await user.keyboard('{Escape}');
    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument();
    expect(trigger).toHaveFocus();
    await user.keyboard(key);
    expect(screen.getByRole('tooltip')).toBeVisible();
    await user.tab();
    await waitFor(() => expect(screen.getByRole('tooltip')).toHaveTextContent(portDescription));
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
