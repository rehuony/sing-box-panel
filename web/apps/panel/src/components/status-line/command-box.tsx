import { X } from 'lucide-react';
import { cn } from '@shared/lib/utils';
import { useTranslation } from 'react-i18next';
import { Button } from '@shared/components/ui/button';
import { ScrollArea } from '@shared/components/ui/scroll-area';
import { useEffect, useLayoutEffect, useRef, useState } from 'react';
import { CommandLine, PromptScope, TerminalBuffer } from '@shared/components/terminal';
import { Tooltip, TooltipContent, TooltipTrigger } from '@shared/components/ui/tooltip';

import { commandHistory, useCommandSession } from '@/stores/command-session.store';

import type { ShellScopeState } from './use-shell-scope';

import { useCommandRunner } from './use-command-runner';

/** Layers that take Escape for themselves while the box is open. */
const OTHER_LAYERS = '[role="dialog"], [role="alertdialog"], [role="menu"], [role="listbox"]';

/**
 * The command line as a box floating near the top of the window: the input
 * under a label naming its scope, and below it one box that holds the
 * completion list while it is open, otherwise the output while there is any.
 * A scrim behind it dims the window, so the page does not compete with the
 * line. Both stay in the document while closed, so they can fade out, and each
 * opening starts on an empty line while the output and history stay.
 */
export function CommandBox({ id, shell }: { id: string; shell: ShellScopeState }) {
  const { t } = useTranslation();
  const { clear, entries, open, setOpen } = useCommandSession();
  const { cancel, completions, prompt, submit } = useCommandRunner(shell);
  const boxRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const returnRef = useRef<HTMLElement | null>(null);
  const [menuContainer, setMenuContainer] = useState<HTMLDivElement | null>(null);
  const [opening, setOpening] = useState(0);
  const [wasOpen, setWasOpen] = useState(open);
  if (open !== wasOpen) {
    setWasOpen(open);
    if (open) setOpening(opening + 1);
  }
  const confirming = prompt.kind === 'confirm';

  // Opening takes focus into the line; closing puts it back where it was, unless the user moved it elsewhere.
  useLayoutEffect(() => {
    if (open) {
      returnRef.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
      inputRef.current?.focus({ preventScroll: true });
      return;
    }
    const target = returnRef.current;
    returnRef.current = null;
    const active = document.activeElement;
    const left = active === null || active === document.body || boxRef.current?.contains(active) === true;
    if (target?.isConnected && left) target.focus({ preventScroll: true });
  }, [open]);

  // A press outside the box lands on the scrim and closes it; the action that controls the box toggles it itself.
  useEffect(() => {
    if (!open) return;
    function onPointerDown(event: PointerEvent) {
      const target = event.target;
      if (!(target instanceof Element) || boxRef.current?.contains(target)) return;
      if (target.closest('[aria-controls]')?.getAttribute('aria-controls') === id) return;
      setOpen(false);
    }
    document.addEventListener('pointerdown', onPointerDown, true);
    return () => document.removeEventListener('pointerdown', onPointerDown, true);
  }, [id, open, setOpen]);

  // Escape the line left alone, as from the output or the status line, leaves a pending confirmation, then the box.
  useEffect(() => {
    if (!open) return;
    function onKeyDown(event: globalThis.KeyboardEvent) {
      if (event.key !== 'Escape' || event.defaultPrevented) return;
      const target = event.target instanceof Element ? event.target : null;
      if (target?.closest(OTHER_LAYERS) && !boxRef.current?.contains(target)) return;
      event.preventDefault();
      if (useCommandSession.getState().pending) cancel();
      else setOpen(false);
    }
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [cancel, open, setOpen]);

  function clearOutput() {
    clear();
    inputRef.current?.focus({ preventScroll: true });
  }

  return (
    <>
      <div
        aria-hidden='true'
        className='fixed inset-0 z-[var(--z-modal)] bg-[var(--color-scrim)] transition-[opacity,display] transition-discrete duration-150 ease-out starting:opacity-0 [&[hidden]]:opacity-0'
        hidden={!open}
      />
      <div
        aria-label={t('command.label')}
        // `[y/N]` takes the warning colour here, in the line and its echoes, as the box's border and label do.
        className='pointer-events-none fixed inset-x-0 top-[max(4.5rem,15dvh)] z-[calc(var(--z-modal)+1)] mx-auto flex w-[min(calc(100vw-2rem),clamp(36rem,40vw,60rem))] origin-top flex-col gap-1.5 font-[family-name:var(--font-data)] text-compact min-[120rem]:text-sm transition-[opacity,scale,display] transition-discrete duration-150 ease-out starting:scale-98 starting:opacity-0 max-sm:top-4 [&[hidden]]:scale-98 [&[hidden]]:opacity-0 [&_span[data-prompt=confirm]]:text-[var(--color-warning-text)]'
        hidden={!open}
        id={id}
        ref={boxRef}
        role='group'
      >
        <div
          className={cn(
            'pointer-events-auto relative flex h-13 flex-none items-center rounded-[var(--radius-card)] border border-[var(--color-border-strong)] bg-popover px-4 text-base shadow-[var(--shadow-panel)] min-[120rem]:h-15 min-[120rem]:px-5 min-[120rem]:text-xl',
            confirming && 'border-[var(--color-warning)]',
          )}
        >
          {/* The scope sits on the box's top border as a pill with the box's outline; the input's name carries it. */}
          <span
            aria-hidden='true'
            className={cn(
              'absolute start-3.5 top-0 flex h-5.5 max-w-[calc(100%-1.75rem)] -translate-y-1/2 items-center overflow-hidden rounded-[var(--radius-pill)] border border-[var(--color-border-strong)] bg-popover px-2.25 text-xs leading-4 min-[120rem]:start-4.5 min-[120rem]:h-6.5 min-[120rem]:px-2.5 min-[120rem]:text-compact',
              confirming && 'border-[var(--color-warning)] text-[var(--color-warning-text)]',
            )}
          >
            {prompt.kind === 'shell' ? <PromptScope prompt={prompt} /> : t('command.confirmScope')}
          </span>
          <CommandLine
            // The line reads at 16px, a step above the shared input's dense size, and at 20px on a large screen.
            className='min-w-0 flex-1 flex-nowrap sm:[&_[data-slot=terminal-input]]:text-base min-[120rem]:[&_[data-slot=terminal-input]]:text-xl min-[120rem]:[&_[data-slot=terminal-input]]:leading-7'
            completions={completions}
            history={commandHistory}
            inputRef={inputRef}
            key={opening}
            label={t(confirming ? 'command.confirm' : 'command.input')}
            menuContainer={menuContainer}
            placeholder={prompt.kind === 'shell' ? 'help' : undefined}
            prompt={prompt}
            promptScope={false}
            onCancel={cancel}
            onExit={() => setOpen(false)}
            onSubmit={submit}
          />
        </div>
        {/* The completion list while it is open, otherwise the output while there is any, never both. */}
        <div
          className='group pointer-events-auto hidden max-h-[min(20rem,50dvh)] min-h-0 flex-col overflow-hidden rounded-[var(--radius-card)] border bg-popover shadow-[var(--shadow-panel)] has-[[role=listbox]]:flex data-[output]:flex max-sm:max-h-[50dvh] min-[120rem]:max-h-[min(26rem,50dvh)] min-[120rem]:[&_[role=option]]:h-8'
          data-output={entries.length > 0 ? '' : undefined}
        >
          <ScrollArea className='hidden flex-1 flex-col has-[[role=listbox]]:flex' viewportClassName='min-h-0 flex-1'>
            <div ref={setMenuContainer} />
          </ScrollArea>
          {entries.length > 0
            ? (
                <div className='relative flex min-h-0 flex-1 flex-col group-has-[[role=listbox]]:hidden'>
                  <TerminalBuffer
                    entries={entries}
                    promptScope={false}
                    // Room at the end for the clear action, and a gap before each command's echo.
                    viewportClassName='py-2 ps-3 pe-[calc(var(--control-height-xs)+var(--space-xs))] [&_p[data-prompt=shell]:not(:first-child)]:mt-1.5'
                  />
                  <Tooltip>
                    <TooltipTrigger
                      render={<Button aria-label={t('command.clearOutput')} className='absolute end-1 top-1' onClick={clearOutput} size='icon-xs' variant='ghost' />}
                    >
                      <X aria-hidden='true' />
                    </TooltipTrigger>
                    <TooltipContent>{t('command.clearOutput')}</TooltipContent>
                  </Tooltip>
                </div>
              )
            : null}
        </div>
      </div>
    </>
  );
}
