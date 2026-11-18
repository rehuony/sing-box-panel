import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { Suspense, useEffect, useRef } from 'react';
import { Outlet, useLocation } from 'react-router-dom';
import { formatRate } from '@shared/lib/telemetry-format';
import { ScrollArea } from '@shared/components/ui/scroll-area';

import '@/i18n';
import { queries } from '@/api/queries';
import { fleetWords } from '@/utils/fleet-summary';
import { useBackdrop } from '@/components/backdrop';
import { preloadPage } from '@/routes/page-loaders';
import { useApiClient } from '@/api/api-client-context';
import { useFleetView } from '@/stores/fleet-view.store';
import { LoadingState } from '@/components/loading-state';
import { useAuthSession } from '@/stores/auth-session.store';
import { EnrollmentDialog } from '@/components/enrollment-dialog';
import { useReloadOnPanelVersion } from '@/hooks/use-panel-update';
import { useCommandSession } from '@/stores/command-session.store';
import { StatusLine, useShellScope } from '@/components/status-line';
import { useEnrollmentDialog } from '@/stores/enrollment-dialog.store';

import { useFleetStream } from './use-fleet-stream';

function WorkspaceLoadingState() {
  const { t } = useTranslation();
  return <LoadingState label={t('shell.loading.page')} fullScreen={false} />;
}

/** Hands the fleet's own values to the backdrop behind the shell. */
function useFleetWords() {
  const { i18n } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language ?? 'en';
  const client = useApiClient();
  const { setWords } = useBackdrop();
  const fleet = useQuery(queries.fleetStatus(client));
  useEffect(() => {
    setWords(fleetWords(fleet.data?.items ?? [], value => formatRate(value, locale)));
  }, [fleet.data, locale, setWords]);
}

/** Whether a key typed here belongs to what has focus: a field, an editor, a menu or a dialog. */
function typingTarget(target: EventTarget | null): boolean {
  if (!(target instanceof Element)) return false;
  return target.closest('input, textarea, select, [contenteditable]:not([contenteditable="false"]), .cm-editor, [role="dialog"], [role="alertdialog"], [role="menu"], [role="listbox"]') !== null;
}

/** A modal layer holds focus, so the command line cannot take it. */
function modalOpen(): boolean {
  return document.querySelector('[role="dialog"][aria-modal="true"], [role="alertdialog"]') !== null;
}

/** Ctrl or Cmd+K anywhere, or `:` outside fields, editors and dialogs, opens the command line. */
function useCommandShortcut() {
  useEffect(() => {
    function onKeyDown(event: KeyboardEvent) {
      const { open, setOpen } = useCommandSession.getState();
      const commandKey = event.key.toLowerCase() === 'k' && (event.metaKey || event.ctrlKey) && !event.altKey && !event.shiftKey;
      const colon = event.key === ':' && !event.metaKey && !event.ctrlKey && !event.altKey && !typingTarget(event.target);
      if (!(commandKey || colon) || modalOpen()) return;
      event.preventDefault();
      if (!open) setOpen(true);
      else if (commandKey) document.querySelector<HTMLInputElement>('[data-slot="terminal-input"]')?.focus();
    }
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, []);
}

/**
 * The command output, prompt and history, the home screen's filters and
 * selection and the enrollment dialog belong to one signed-in session: the shell leaves whenever the
 * session ends, by signing out or expiring, and takes them with it.
 */
function useSessionScopedState() {
  useEffect(() => () => {
    useCommandSession.getState().reset();
    useFleetView.getState().reset();
    useEnrollmentDialog.getState().reset();
  }, []);
}

/**
 * The signed-in shell: the workspace filling the window at the shell's inset
 * and the status line below it, which names the scope and its windows and
 * reads commands.
 */
export function AppShell() {
  const { t } = useTranslation();
  useFleetStream();
  useFleetWords();
  useCommandShortcut();
  useSessionScopedState();
  useReloadOnPanelVersion();
  const { session } = useAuthSession();
  const shell = useShellScope();
  const location = useLocation();
  const mainRef = useRef<HTMLDivElement>(null);
  const scrollRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    preloadPage('nodes');
  }, []);
  useEffect(() => {
    if (scrollRef.current) scrollRef.current.scrollTop = 0;
    mainRef.current?.focus({ preventScroll: true });
  }, [location.pathname]);

  if (session === null) return null;

  return (
    <div className='flex h-dvh min-h-0 flex-col gap-3 overflow-hidden p-[var(--shell-edge)] max-sm:gap-0 max-sm:p-0'>
      <a
        className='fixed top-3 left-4 z-[var(--z-modal)] -translate-y-[160%] rounded-[var(--radius-control)] border border-primary/50 bg-accent px-4 py-3 text-sm font-medium text-primary transition-transform duration-[var(--dur-short)] ease-[var(--ease-out)] focus:translate-y-0'
        href='#main-content'
        onClick={(event) => {
          event.preventDefault();
          mainRef.current?.focus();
        }}
      >
        {t('shell.skip')}
      </a>
      <ScrollArea
        className='min-h-0 min-w-0 flex-1'
        viewportClassName='flex flex-col max-sm:p-[var(--shell-edge)]'
        viewportRef={scrollRef}
      >
        <main className='@container flex min-w-0 flex-1 flex-col outline-none' id='main-content' ref={mainRef} tabIndex={-1}>
          <Suspense fallback={<WorkspaceLoadingState />}>
            <Outlet />
          </Suspense>
        </main>
      </ScrollArea>
      <StatusLine shell={shell} />
      <EnrollmentDialog />
    </div>
  );
}
