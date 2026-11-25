import type { MouseEvent } from 'react';
import type { FieldControl } from '@shared/components/glyph-field';

import { useTranslation } from 'react-i18next';
import { useEffect, useRef, useState } from 'react';
import { appendEntries, CommandLine, echo, logLine, settleEntry, TerminalBuffer, WindowMarks } from '@shared/components/terminal';

import type { ConsoleStatus, Session } from '@/api/generated';

import { ConsoleFrame } from '@/components/console-frame';
import { ConsoleRequestError, enroll, getStatus, logout, unenroll } from '@/api/console-client';

import type { TrafficRates } from './status-format';
import type { BufferEntry, StatusBlock } from './buffer-entries';

import { LinkClock } from './link-clock';
import { statusEvents } from './status-events';
import { StatusReadout } from './status-readout';
import { freezeStatus, shellPrompt, statusBlock } from './buffer-entries';
import { connectionTone, nodeWords, trafficRates } from './status-format';

/** How often the page refreshes the node's status. */
const REFRESH_INTERVAL_MS = 5_000;
/** An enrollment code echoes only its start; the rest stays out of the terminal. */
const CODE_ECHO_LENGTH = 12;
/** The command the terminal opens on, as if it had just been typed. */
const STATUS_COMMAND = 'status';
/** The host the prompt names before the node reports its own. */
const DEFAULT_HOST = 'sing-box-node';
const CODE_PROMPT = { kind: 'code' } as const;
const CONFIRM_PROMPT = { kind: 'confirm' } as const;

type EnrollCommand = 'enroll' | 're-enroll';

/** What the input line is waiting for. */
type Mode
  = | { kind: 'command' }
    | { kind: 'code'; command: EnrollCommand }
    | { kind: 'confirm'; address: string }
    | { kind: 'running' };

const helpKeys = {
  'clear': 'clear',
  'enroll': 'enroll',
  'exit': 'exit',
  'help': 'help',
  're-enroll': 'reenroll',
  'status': 'status',
  'unenroll': 'unenroll',
} as const;

/** The commands the node takes in its current state, as help lists them. */
function commandsFor(status: ConsoleStatus | null): (keyof typeof helpKeys)[] {
  const enrolled = Boolean(status?.enrollment);
  return [...(enrolled ? ['re-enroll', 'unenroll'] as const : ['enroll'] as const), 'status', 'clear', 'help', 'exit'];
}

function signedOutBy(error: unknown): boolean {
  return error instanceof ConsoleRequestError && error.status === 401;
}

function enrollError(error: unknown): 'failed' | 'invalid' | 'rejected' | 'unreachable' {
  if (!(error instanceof ConsoleRequestError)) return 'failed';
  if (error.code === 'enrollment_rejected') return 'rejected';
  if (error.code === 'enrollment_failed') return 'unreachable';
  if (error.code === 'enrollment_code_invalid' || error.status === 400 || error.status === 413) return 'invalid';
  return 'failed';
}

function finePointer(): boolean {
  return window.matchMedia('(pointer: fine)').matches;
}

interface StatusPageProps {
  session: Session;
  field: FieldControl;
  onSignedOut: () => void;
  /** The status read while signing in, if any; the page opens on it without loading. */
  initialStatus: ConsoleStatus | null;
}

/** The signed-in console: a terminal that shows the node's status and takes its enrollment commands. */
export function StatusPage({ field, initialStatus, session, onSignedOut }: StatusPageProps) {
  const { i18n, t } = useTranslation();
  const locale = i18n.resolvedLanguage ?? 'en';
  const clock = (value: string) => new Intl.DateTimeFormat(locale, { hour: '2-digit', hourCycle: 'h23', minute: '2-digit', second: '2-digit' })
    .format(Date.parse(value));

  const [status, setStatus] = useState<ConsoleStatus | null>(initialStatus);
  const host = status?.node.hostname ?? DEFAULT_HOST;
  const [rates, setRates] = useState<TrafficRates | null>(null);
  const [now, setNow] = useState(Date.now);
  const [stale, setStale] = useState(false);
  const [mode, setMode] = useState<Mode>({ kind: 'command' });
  const [signingOut, setSigningOut] = useState(false);

  const statusRef = useRef(initialStatus);
  // The last reading with live counters, which the next one's rates count from.
  const trafficRef = useRef(initialStatus?.traffic?.live ? initialStatus.traffic : null);
  const lastErrorRef = useRef<string | undefined>(undefined);
  const staleRef = useRef(false);
  const inputRef = useRef<HTMLInputElement>(null);
  // The terminal opens on the status command's output, as if it had just been run.
  const [entries, setEntries] = useState<BufferEntry[]>(() => [
    echo(shellPrompt(initialStatus?.node.hostname ?? DEFAULT_HOST), STATUS_COMMAND),
    statusBlock(),
    ...(initialStatus ? statusEvents(null, initialStatus, { clock, lastError: lastErrorRef, t }) : []),
  ]);

  function print(...added: BufferEntry[]) {
    setEntries(current => appendEntries(current, added));
  }

  function settle(id: number) {
    setEntries(current => settleEntry(current, id));
  }

  function accept(next: ConsoleStatus) {
    const events = statusEvents(statusRef.current, next, { clock, lastError: lastErrorRef, t });
    statusRef.current = next;
    // Rates come from consecutive readings of the counters, not from refreshes.
    const previous = trafficRef.current;
    if (!next.traffic?.live) {
      trafficRef.current = null;
      setRates(null);
    } else if (next.traffic.live.sampled_at !== previous?.live?.sampled_at) {
      trafficRef.current = next.traffic;
      setRates(trafficRates(previous, next.traffic));
    }
    setStatus(next);
    setNow(Date.now());
    if (events.length > 0) print(...events);
  }

  async function refresh(source: 'command' | 'first' | 'interval') {
    try {
      const next = await getStatus();
      accept(next);
      if (staleRef.current) {
        staleRef.current = false;
        setStale(false);
        print(logLine('success', t('terminal.log.recovered'), { stamp: true }));
      }
      // Each refresh ripples once through the field behind the terminal.
      if (source !== 'first') field.pulse('refresh');
    } catch (error) {
      // A session that ended, or a console turned off, ends this page.
      if (signedOutBy(error)) {
        onSignedOut();
        return;
      }
      if (!staleRef.current || source === 'command') print(logLine('warning', t('console.stale'), { stamp: true }));
      staleRef.current = true;
      setStale(true);
    }
  }

  const refreshRef = useRef(refresh);
  refreshRef.current = refresh;
  const hadStatusRef = useRef(initialStatus !== null);
  useEffect(() => {
    if (!hadStatusRef.current) {
      hadStatusRef.current = true;
      void refreshRef.current('first');
    }
    const timer = window.setInterval(() => void refreshRef.current('interval'), REFRESH_INTERVAL_MS);
    return () => window.clearInterval(timer);
  }, []);

  useEffect(() => {
    if (status) field.setWords(nodeWords(status));
  }, [field, status]);

  // A terminal takes typing at once where a keyboard is at hand; a phone keeps its keyboard down until tapped.
  useEffect(() => {
    if (finePointer()) inputRef.current?.focus({ preventScroll: true });
  }, [mode.kind]);

  async function signOut() {
    if (signingOut) return;
    setSigningOut(true);
    try {
      await logout(session.csrfToken);
    } finally {
      onSignedOut();
    }
  }

  function startEnroll(command: EnrollCommand) {
    const current = statusRef.current;
    if (current === null) {
      print(logLine('warning', t('terminal.log.waiting')));
    } else if (command === 'enroll' && current.enrollment) {
      print(logLine('warning', t('terminal.log.alreadyEnrolled', { address: current.enrollment.panel_url })));
    } else if (command === 're-enroll' && !current.enrollment) {
      print(logLine('warning', t('terminal.log.notEnrolledYet')));
    } else {
      if (current.enrollment) print(logLine('warning', t('terminal.log.reenrollWarning', { address: current.enrollment.panel_url })));
      print(logLine('muted', t('terminal.log.codeHint')));
      setMode({ command, kind: 'code' });
    }
  }

  function startUnenroll() {
    const current = statusRef.current;
    if (current === null) {
      print(logLine('warning', t('terminal.log.waiting')));
    } else if (!current.enrollment) {
      print(logLine('warning', t('terminal.log.notEnrolledYet')));
    } else {
      const address = current.enrollment.panel_url;
      print(logLine('danger', t('terminal.log.unenrollWarning', { address })));
      setMode({ address, kind: 'confirm' });
    }
  }

  async function redeem(input: string) {
    const code = input.trim();
    // An empty line backs out, which a phone without Escape needs.
    if (code === '') {
      print(echo(CODE_PROMPT, ''), logLine('muted', t('terminal.log.cancelled')));
      setMode({ kind: 'command' });
      return;
    }
    print(echo(CODE_PROMPT, code.length > CODE_ECHO_LENGTH ? `${code.slice(0, CODE_ECHO_LENGTH)}…` : code));
    const pending = logLine('muted', t('terminal.log.enrolling'), { pending: true });
    print(pending);
    setMode({ kind: 'running' });
    try {
      const next = await enroll(code, session.csrfToken);
      settle(pending.id);
      if (next.enrollment) {
        print(logLine('success', t('terminal.log.enrolled', { address: next.enrollment.panel_url, nodeId: next.enrollment.node_id })));
      }
      accept(next);
    } catch (error) {
      settle(pending.id);
      if (signedOutBy(error)) {
        onSignedOut();
        return;
      }
      print(logLine('danger', t(`enroll.error.${enrollError(error)}`)));
    }
    setMode({ kind: 'command' });
  }

  async function answer(input: string, address: string) {
    const reply = input.trim();
    print(echo(CONFIRM_PROMPT, reply));
    if (!/^y(?:es)?$/i.test(reply)) {
      print(logLine('muted', t('terminal.log.cancelled')));
      setMode({ kind: 'command' });
      return;
    }
    const pending = logLine('muted', t('terminal.log.unenrolling'), { pending: true });
    print(pending);
    setMode({ kind: 'running' });
    try {
      const next = await unenroll(session.csrfToken);
      settle(pending.id);
      print(logLine('success', t('terminal.log.unenrolled', { address })));
      accept(next);
    } catch (error) {
      settle(pending.id);
      if (signedOutBy(error)) {
        onSignedOut();
        return;
      }
      print(logLine('danger', t('terminal.log.unenrollFailed')));
    }
    setMode({ kind: 'command' });
  }

  function execute(input: string) {
    const entry = input.trim();
    print(echo(shellPrompt(host), entry));
    if (entry === '') return;
    const [name] = entry.split(/\s+/);
    switch (name) {
      case 'help':
        print(...commandsFor(statusRef.current).map(command => logLine('muted', t(`terminal.help.${helpKeys[command]}`), { term: command })));
        break;
      case 'clear':
        setEntries([]);
        break;
      case 'status':
        // The new output follows the node from now on; the earlier ones keep what they showed.
        setEntries(current => appendEntries(freezeStatus(current, { now, rates, status }), [statusBlock()]));
        void refresh('command');
        break;
      case 'exit':
        void signOut();
        break;
      case 'enroll':
      case 're-enroll':
        startEnroll(name);
        break;
      case 'unenroll':
        startUnenroll();
        break;
      default:
        print(logLine('danger', t('terminal.log.notFound', { command: name })));
    }
  }

  function submit(input: string) {
    switch (mode.kind) {
      case 'command':
        execute(input);
        break;
      case 'code':
        void redeem(input);
        break;
      case 'confirm':
        void answer(input, mode.address);
        break;
      case 'running':
        break;
    }
  }

  function cancel() {
    if (mode.kind !== 'code' && mode.kind !== 'confirm') return;
    print(echo(mode.kind === 'code' ? CODE_PROMPT : CONFIRM_PROMPT, '^C'), logLine('muted', t('terminal.log.cancelled')));
    setMode({ kind: 'command' });
  }

  // Clicking the terminal puts the cursor on its input line, unless the click selected text to copy.
  function focusPrompt(event: MouseEvent<HTMLElement>) {
    if (!finePointer() || (event.target as Element).closest('button, a, input')) return;
    if (window.getSelection()?.isCollapsed === false) return;
    inputRef.current?.focus({ preventScroll: true });
  }

  const tone = status ? connectionTone[status.connection.state] : null;
  const commands = commandsFor(status).map(name => ({ description: t(`terminal.help.${helpKeys[name]}`), name }));

  let input = null;
  if (mode.kind !== 'running') {
    const prompt = mode.kind === 'command' ? shellPrompt(host) : mode.kind === 'code' ? CODE_PROMPT : CONFIRM_PROMPT;
    input = (
      <CommandLine
        // Commands take no arguments, so only the first word completes.
        completions={(_words, index) => index === 0 ? commands : []}
        inputRef={inputRef}
        label={t(`terminal.input.${mode.kind}`)}
        placeholder={prompt.kind === 'shell' ? 'help' : undefined}
        prompt={prompt}
        onCancel={cancel}
        onSubmit={submit}
      />
    );
  }

  return (
    <ConsoleFrame busy={signingOut} onSignOut={() => void signOut()}>
      <h1 className='sr-only'>{t('console.title')}</h1>
      {/*
        The window is the console's one surface; the sign-in prompt morphs into it. It keeps a desktop
        terminal's 3:2 at the largest size the space below the first row allows; a phone gives it a fixed height.
      */}
      <section
        aria-label={host}
        className='status-terminal flex aspect-[3/2] w-[min(100%,64rem,calc((100svh-9rem)*1.5))] flex-col max-sm:aspect-auto max-sm:h-[min(32rem,calc(100svh-9rem))] max-sm:w-full overflow-hidden rounded-[var(--radius-card)] border border-border bg-card font-[family-name:var(--font-data)] text-compact text-foreground shadow-[var(--shadow-card)] [view-transition-name:console-window]'
        onClick={focusPrompt}
        ref={field.anchor}
      >
        <header className='relative flex h-10 shrink-0 items-center px-4'>
          <WindowMarks />
          <p className='pointer-events-none absolute inset-x-24 truncate text-center font-[family-name:var(--font-body)] font-medium'>
            {`console@${host}:~`}
          </p>
          <p className='ms-auto flex shrink-0 items-center gap-2 text-muted-foreground'>
            {stale ? <span className='text-[var(--color-warning-text)]'>{t('terminal.stale')}</span> : null}
            {status && tone
              ? (
                  <>
                    <span aria-hidden='true' className={`size-2 rounded-full ${tone.dot}`} />
                    <LinkClock connection={status.connection} ticking={!stale} />
                  </>
                )
              : null}
          </p>
        </header>
        <TerminalBuffer
          entries={entries}
          renderBlock={(block: StatusBlock) => (
            <StatusReadout {...(block.frozen ?? { now, rates, status })} stale={!block.frozen && stale} />
          )}
        >
          {input}
        </TerminalBuffer>
      </section>
    </ConsoleFrame>
  );
}
