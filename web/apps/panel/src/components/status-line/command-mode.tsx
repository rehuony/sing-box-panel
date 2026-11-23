import { X } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useEffect, useRef, useState } from 'react';
import { Button } from '@shared/components/ui/button';
import { CommandLine, TerminalBuffer } from '@shared/components/terminal';

import { commandHistory, useCommandSession } from '@/stores/command-session.store';

import type { ShellScopeState } from './use-shell-scope';

import { useCommandRunner } from './use-command-runner';

/**
 * The status line reading a command: the input line in place of the scope,
 * windows and readout, and one floating panel above the line that holds the
 * completion menu and the output. On a phone the panel and the line read as
 * one sheet at the window's foot.
 */
export function CommandMode({ id, shell }: { id: string; shell: ShellScopeState }) {
  const { t } = useTranslation();
  const { clear, entries, setOpen } = useCommandSession();
  const { cancel, completions, prompt, submit } = useCommandRunner(shell);
  const inputRef = useRef<HTMLInputElement>(null);
  const [menuContainer, setMenuContainer] = useState<HTMLDivElement | null>(null);
  const output = entries.length > 0;

  useEffect(() => {
    inputRef.current?.focus({ preventScroll: true });
  }, []);

  // The command action controls this element. It sets no position, so the
  // panel inside still floats against the status line.
  return (
    <div className='flex h-full min-w-0 flex-1 items-center' id={id}>
      <div
        // The panel shows while there is output or a menu; it never pushes the workspace.
        className='absolute inset-x-0 bottom-[calc(100%+var(--space-xs))] z-[var(--z-sticky)] hidden max-h-[40vh] flex-col overflow-hidden rounded-[var(--radius-card)] border bg-popover text-compact shadow-[var(--shadow-panel)] has-[[role=listbox]]:flex data-[output]:flex max-sm:bottom-full max-sm:max-h-[50dvh] max-sm:rounded-b-none max-sm:border-x-0 max-sm:border-b-0'
        data-output={output ? '' : undefined}
      >
        {output
          ? (
              <>
                <div className='flex h-8 flex-none items-center justify-between ps-3 pe-1 text-muted-foreground'>
                  <span>{t('command.output')}</span>
                  <Button aria-label={t('command.dismiss')} onClick={clear} size='icon-xs' variant='ghost'>
                    <X aria-hidden='true' />
                  </Button>
                </div>
                <TerminalBuffer entries={entries} viewportClassName='px-3 pb-2' />
              </>
            )
          : null}
        <div className='max-h-[min(16rem,40vh)] flex-none overflow-y-auto px-3 empty:hidden has-[[role=listbox]]:py-2' ref={setMenuContainer} />
      </div>
      <CommandLine
        className='min-w-0 flex-1 flex-nowrap overflow-hidden px-2'
        completions={completions}
        history={commandHistory}
        inputRef={inputRef}
        label={t(prompt.kind === 'confirm' ? 'command.confirm' : 'command.input')}
        menuContainer={menuContainer}
        placeholder={prompt.kind === 'shell' ? 'help' : undefined}
        prompt={prompt}
        onCancel={cancel}
        onExit={() => setOpen(false)}
        onSubmit={submit}
      />
    </div>
  );
}
