import type { LucideIcon } from 'lucide-react';
import type { supportedLanguages } from '@shared/i18n/language';

import { cn } from '@shared/lib/utils';
import { useTranslation } from 'react-i18next';
import { useLayoutEffect, useRef } from 'react';
import { Button } from '@shared/components/ui/button';
import { setAppLanguage } from '@shared/i18n/language';
import { Spinner } from '@shared/components/ui/spinner';
import { Link, NavLink, useNavigate } from 'react-router-dom';
import { LanguageMenu } from '@shared/components/language-menu';
import { ThemeCycleButton } from '@shared/components/theme-cycle-button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@shared/components/ui/tooltip';
import {
  ArrowLeft,
  Cpu,
  Ellipsis,
  FileJson,
  Gauge,
  LogOut,
  Rss,
  ScrollText,
  Server,
  Settings,
} from 'lucide-react';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@shared/components/ui/dropdown-menu';

import type { NodeStatus } from '@/utils/fleet-summary';
import type { NodeWindow, PanelWindow } from '@/utils/shell-windows';

import { useTheme } from '@/theme';
import { preloadPage } from '@/routes/page-loaders';
import { useFleetView } from '@/stores/fleet-view.store';
import { usePanelUpdate } from '@/hooks/use-panel-update';
import { usePhoneLayout } from '@/hooks/use-phone-layout';
import { useCommandSession } from '@/stores/command-session.store';
import { useFleetStreamState } from '@/components/app-shell/use-fleet-stream';
import {
  nodeWindowPages,
  nodeWindowPath,
  nodeWindows,
  panelWindowPages,
  panelWindowPath,
  panelWindows,
} from '@/utils/shell-windows';

import type { ShellScopeState } from './use-shell-scope';

import { useSignOut } from './use-sign-out';
import { CommandMode } from './command-mode';
import { BatchActions } from './batch-actions';

const windowIcons: Record<NodeWindow | PanelWindow, LucideIcon> = {
  configuration: FileJson,
  cores: Cpu,
  logs: ScrollText,
  nodes: Server,
  overview: Gauge,
  settings: Settings,
  subscriptions: Rss,
};

const windowNames: Record<NodeWindow | PanelWindow, string> = {
  configuration: 'node.tabs.configuration',
  cores: 'node.tabs.cores',
  logs: 'nav.logs',
  nodes: 'nav.nodes',
  overview: 'node.tabs.overview',
  settings: 'nav.settings',
  subscriptions: 'nav.subscriptions',
};

/** The keys that open the command line on this platform. */
function commandKeys(): string {
  return typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.userAgent) ? '⌘K' : 'Ctrl K';
}

function Divider() {
  return <span aria-hidden='true' className='h-5 w-px flex-none bg-border' />;
}

interface WindowEntry {
  path: string;
  window: NodeWindow | PanelWindow;
  page: Parameters<typeof preloadPage>[0];
}

function windowsOf({ scope }: ShellScopeState): WindowEntry[] {
  return scope.kind === 'node'
    ? nodeWindows.map(window => ({ page: nodeWindowPages[window], path: nodeWindowPath(scope.nodeId, window), window }))
    : panelWindows.map(window => ({ page: panelWindowPages[window], path: panelWindowPath(window), window }));
}

/** The line's first action: the prompt symbol, which opens the command line. */
function CommandAction({ className }: { className?: string }) {
  const { t } = useTranslation();
  const { open, setOpen } = useCommandSession();
  return (
    <Tooltip>
      <TooltipTrigger
        render={(
          <Button
            aria-expanded={open}
            aria-label={t('command.label')}
            className={cn('font-[family-name:var(--font-data)] text-sm text-[var(--color-highlight-yellow)]', className)}
            onClick={() => setOpen(!open)}
            size='icon-xs'
            variant='ghost'
          />
        )}
      >
        λ
      </TooltipTrigger>
      <TooltipContent>{t('command.shortcut', { keys: commandKeys() })}</TooltipContent>
    </Tooltip>
  );
}

/**
 * The way back to the fleet, the same in every scope; a node window's title
 * strip names the node and leads to its neighbours.
 */
function Scope() {
  const { t } = useTranslation();
  return (
    <Link
      className='inline-flex h-(--control-height-xs) flex-none items-center rounded-[var(--radius-control)] px-2 text-foreground outline-none hover:bg-[var(--color-hover)] focus-visible:ring-2 focus-visible:ring-ring'
      title={t('shell.scope.fleet')}
      to='/nodes'
    >
      fleet
    </Link>
  );
}

/** The scope's windows as numbered tabs; the current one carries the accent's soft fill. */
function Windows({ shell }: { shell: ShellScopeState }) {
  const { t } = useTranslation();
  const windows = windowsOf(shell);
  return (
    <nav aria-label={t('shell.bar.windows')} className='flex min-w-0 items-center gap-0.5 overflow-x-auto [scrollbar-width:none]'>
      {windows.map(({ page, path, window }, index) => (
        <NavLink
          aria-label={t('shell.window', { name: t(windowNames[window]), number: index + 1 })}
          className='inline-flex h-(--control-height-xs) flex-none items-center gap-[1ch] rounded-[var(--radius-control)] px-2 whitespace-nowrap text-muted-foreground outline-none hover:bg-[var(--color-hover)] hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring aria-[current=page]:bg-accent aria-[current=page]:text-foreground'
          end
          key={window}
          onFocus={() => preloadPage(page)}
          onPointerEnter={() => preloadPage(page)}
          to={path}
        >
          <span aria-hidden='true' className='text-[var(--color-text-faint)]'>{index + 1}</span>
          <span aria-hidden='true'>{window}</span>
        </NavLink>
      ))}
    </nav>
  );
}

/** How much of the fleet is online, and whether the stream that keeps it current is down. */
function FleetReadout({ items }: { items: readonly NodeStatus[] }) {
  const { t } = useTranslation();
  const down = useFleetStreamState(state => state.down);
  const online = items.filter(item => item.node.state === 'online').length;
  const total = items.length;
  // While the stream is down the dot alone says so, as a warning ring; its reason is in the tooltip and the name.
  let tone = 'bg-[var(--color-text-faint)]';
  if (down) tone = 'border-2 border-[var(--color-warning)]';
  else if (total > 0 && online === total) tone = 'bg-[var(--color-success)]';
  else if (online > 0) tone = 'bg-[var(--color-warning)]';
  const label = [t('shell.readout.label', { online, total }), down ? t('shell.readout.stale') : null].filter(Boolean).join('. ');
  return (
    <Tooltip>
      <TooltipTrigger
        render={<Link aria-label={label} className='inline-flex h-(--control-height-xs) flex-none items-center gap-2 rounded-[var(--radius-control)] px-2 whitespace-nowrap tabular-nums outline-none hover:bg-[var(--color-hover)] focus-visible:ring-2 focus-visible:ring-ring' to='/nodes' />}
      >
        <span aria-hidden='true' className={cn('size-2 rounded-full', tone)} />
        <span aria-hidden='true'>{t('shell.readout.online', { online, total })}</span>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}

/**
 * The panel's own update, where the line has room for it: a newer release to
 * install, an update under way, or a restart it waits for. It sits left of the
 * readout, in the line's free space, so nothing else moves when it appears.
 */
function PanelUpdateHint() {
  const { t } = useTranslation();
  const update = usePanelUpdate().data;
  if (!update) return null;
  let text: string | null = null;
  let tone = 'text-[var(--color-info-text)]';
  if (update.state === 'restart_required') {
    text = t('shell.update.restart');
    tone = 'text-[var(--color-warning-text)]';
  } else if (update.state !== 'idle') {
    text = t('shell.update.running');
  } else if (update.update_available && update.latest_version) {
    text = `↑ ${update.latest_version}`;
  }
  if (text === null) return null;
  const label = update.state === 'idle'
    ? t('shell.update.available', { version: update.latest_version })
    : text;
  return (
    <Tooltip>
      <TooltipTrigger
        render={<Link aria-label={label} className={cn('inline-flex h-(--control-height-xs) flex-none items-center gap-1.5 rounded-[var(--radius-control)] px-2 whitespace-nowrap outline-none hover:bg-[var(--color-hover)] focus-visible:ring-2 focus-visible:ring-ring', tone)} to='/settings' />}
      >
        {update.state === 'updating' || update.state === 'restarting' ? <Spinner className='size-3.5' /> : null}
        <span aria-hidden='true'>{text}</span>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}

function SignOutAction() {
  const { t } = useTranslation();
  const { signOut, signingOut } = useSignOut();
  const label = signingOut ? t('account.signingOut') : t('account.signOut');
  return (
    <Tooltip>
      <TooltipTrigger render={<Button aria-label={label} onClick={signOut} size='icon-xs' variant='ghost' />}>
        {signingOut ? <Spinner /> : <LogOut aria-hidden='true' />}
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}

/** With panes selected on the home screen, the selection takes the scope's and windows' place. */
function Selection({ compact = false, nodes }: { compact?: boolean; nodes: readonly NodeStatus[] }) {
  const { t } = useTranslation();
  const { clearSelection, select, visible } = useFleetView();
  const count = nodes.length;
  return (
    <div aria-label={t('shell.selection.count', { count })} className='flex min-w-0 items-center gap-1 overflow-x-auto [scrollbar-width:none]' role='group'>
      <span className='flex-none px-2 whitespace-nowrap text-foreground tabular-nums'>{t('shell.selection.count', { count })}</span>
      <Button className='font-[family-name:var(--font-body)]' onClick={() => select(visible)} size='xs' variant='ghost'>
        {t('shell.selection.selectAll')}
      </Button>
      <Button className='font-[family-name:var(--font-body)]' onClick={clearSelection} size='xs' variant='ghost'>
        {t('shell.selection.clear')}
      </Button>
      {compact ? null : <Divider />}
      <BatchActions compact={compact} nodes={nodes} />
    </div>
  );
}

/** The selected nodes while the home screen is open; elsewhere the selection waits. */
function useSelectedNodes({ items, scope }: ShellScopeState): NodeStatus[] {
  const selected = useFleetView(state => state.selected);
  if (scope.kind !== 'fleet' || scope.window !== 'nodes' || selected.size === 0) return [];
  return items.filter(item => selected.has(item.node.id));
}

/** The desktop line: command action, scope, windows, then the fleet readout and the session's actions. */
function DesktopLine({ shell }: { shell: ShellScopeState }) {
  const { cycleTheme, preference } = useTheme();
  const selection = useSelectedNodes(shell);
  return (
    <div className='flex h-full min-w-0 flex-1 items-center gap-1'>
      <CommandAction />
      <Divider />
      {selection.length > 0
        ? <Selection nodes={selection} />
        : (
            <>
              <Scope />
              <Divider />
              <Windows shell={shell} />
            </>
          )}
      <span className='flex-1' />
      <PanelUpdateHint />
      <FleetReadout items={shell.items} />
      <Divider />
      <ThemeCycleButton onCycle={cycleTheme} preference={preference} size='icon-xs' />
      <LanguageMenu size='icon-xs' />
      <SignOutAction />
    </div>
  );
}

/** The phone's "more" menu: theme, language and signing out. */
function MoreMenu({ className }: { className: string }) {
  const { i18n, t } = useTranslation();
  const { preference, setPreference } = useTheme();
  const { signOut } = useSignOut();
  return (
    <DropdownMenu>
      <DropdownMenuTrigger render={<button aria-label={t('shell.bar.more')} className={className} type='button' />}>
        <Ellipsis aria-hidden='true' className='size-5' />
      </DropdownMenuTrigger>
      <DropdownMenuContent align='end' side='top' sideOffset={8}>
        <DropdownMenuGroup>
          <DropdownMenuLabel>{t('theme.label')}</DropdownMenuLabel>
          <DropdownMenuRadioGroup onValueChange={value => setPreference(value as typeof preference)} value={preference}>
            {(['light', 'dark', 'system'] as const).map(value => (
              <DropdownMenuRadioItem closeOnClick key={value} value={value}>{t(`theme.preference.${value}`)}</DropdownMenuRadioItem>
            ))}
          </DropdownMenuRadioGroup>
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        <DropdownMenuGroup>
          <DropdownMenuLabel>{t('language.label')}</DropdownMenuLabel>
          <DropdownMenuRadioGroup onValueChange={value => void setAppLanguage(value as typeof supportedLanguages[number])} value={i18n.resolvedLanguage ?? 'en'}>
            <DropdownMenuRadioItem closeOnClick value='en'>{t('language.english')}</DropdownMenuRadioItem>
            <DropdownMenuRadioItem closeOnClick value='zh-CN'>{t('language.simplifiedChinese')}</DropdownMenuRadioItem>
          </DropdownMenuRadioGroup>
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        <DropdownMenuItem onClick={signOut}>
          <LogOut aria-hidden='true' />
          {t('account.signOut')}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

/**
 * The phone's tab bar: back to the fleet in a node, the windows as icons with
 * the current one named, the command line and more.
 */
function PhoneBar({ shell }: { shell: ShellScopeState }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const windows = windowsOf(shell);
  const selection = useSelectedNodes(shell);
  const item = 'flex h-full min-w-10 flex-1 flex-col items-center justify-center gap-0.5 rounded-[var(--radius-control)] text-muted-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset';
  return (
    <div className='flex h-full min-w-0 flex-1 items-stretch'>
      {shell.scope.kind === 'node'
        ? (
            <button aria-label={t('shell.scope.fleet')} className={item} onClick={() => void navigate('/nodes')} type='button'>
              <ArrowLeft aria-hidden='true' className='size-5' />
            </button>
          )
        : null}
      {selection.length > 0
        ? <div className='flex min-w-0 flex-[4] items-center justify-center'><Selection compact nodes={selection} /></div>
        : (
            <nav aria-label={t('shell.bar.windows')} className='flex min-w-0 flex-[5] items-stretch'>
              {windows.map(({ path, window }, index) => {
                const Icon = windowIcons[window];
                return (
                  <NavLink
                    aria-label={t('shell.window', { name: t(windowNames[window]), number: index + 1 })}
                    className={cn(item, 'group aria-[current=page]:flex-[1.6] aria-[current=page]:text-foreground')}
                    end
                    key={window}
                    to={path}
                  >
                    <Icon aria-hidden='true' className='size-5 group-aria-[current=page]:text-foreground' />
                    <span aria-hidden='true' className='hidden max-w-full truncate px-1 font-[family-name:var(--font-data)] text-2xs group-aria-[current=page]:block'>{window}</span>
                  </NavLink>
                );
              })}
            </nav>
          )}
      <CommandAction className={cn(item, 'size-auto text-base')} />
      <MoreMenu className={item} />
    </div>
  );
}

/**
 * The status line below the workspace: on a desktop one 40px line at the
 * shell's inset, on a phone a 56px tab bar along the window's foot.
 */
export function StatusLine({ shell }: { shell: ShellScopeState }) {
  const { t } = useTranslation();
  const open = useCommandSession(state => state.open);
  const phone = usePhoneLayout();
  const returnRef = useRef<HTMLElement | null>(null);

  // Closing the command line puts focus back where it was when the line opened.
  useLayoutEffect(() => {
    if (open) {
      returnRef.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
      return;
    }
    const target = returnRef.current;
    returnRef.current = null;
    if (target?.isConnected && (document.activeElement === document.body || document.activeElement === null)) {
      target.focus({ preventScroll: true });
    }
  }, [open]);

  return (
    <div
      aria-label={t('shell.bar.label')}
      className='relative flex h-[var(--shell-bar-height)] flex-none items-center rounded-[var(--radius-card)] border bg-card px-1.5 font-[family-name:var(--font-data)] text-compact shadow-[var(--shadow-panel)] max-sm:rounded-none max-sm:border-x-0 max-sm:border-b-0 max-sm:px-1 max-sm:shadow-none'
      role='region'
    >
      {open
        ? (
            <div className='flex h-full min-w-0 flex-1 items-center gap-1'>
              <CommandAction className='max-sm:size-11' />
              <Divider />
              <CommandMode shell={shell} />
            </div>
          )
        : phone ? <PhoneBar shell={shell} /> : <DesktopLine shell={shell} />}
    </div>
  );
}
