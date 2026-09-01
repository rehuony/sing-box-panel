import type { CSSProperties } from 'react';

import { LogOut } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useEffect, useRef, useState } from 'react';
import { NavLink, Outlet, useLocation } from 'react-router-dom';

import '@/i18n';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Spinner } from '@/components/ui/spinner';
import { PanelLogo } from '@/components/panel-logo';
import { Skeleton } from '@/components/ui/skeleton';
import { useApiClient } from '@/api/api-client-context';
import { ErrorNotice } from '@/components/error-notice';
import { AnimatedIcon } from '@/components/animated-icon';
import { useSidebar } from '@/components/ui/sidebar-context';
import { useAuthSession } from '@/stores/auth-session.store';
import { useControlPlane } from '@/stores/control-plane.store';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
} from '@/components/ui/sidebar';

import { TelemetryBanner } from './telemetry-banner';
import { TelemetryProvider } from './telemetry-provider';
import './app-shell.css';

export const PANEL_STATUS_REFRESH_MS = 10_000;

const navigationItems = [
  { icon: 'dashboard', labelKey: 'nav.dashboard', to: '/' },
  { icon: 'configuration', labelKey: 'nav.configuration', to: '/configuration' },
  { icon: 'cores', labelKey: 'nav.cores', to: '/cores' },
  { icon: 'subscriptions', labelKey: 'nav.subscriptions', to: '/subscriptions' },
  { icon: 'observability', labelKey: 'nav.observability', to: '/observability' },
  { icon: 'tasks', labelKey: 'nav.tasks', to: '/tasks' },
] as const;

function isNavigationItemActive(pathname: string, to: string): boolean {
  return to === '/' ? pathname === '/' : pathname === to || pathname.startsWith(`${to}/`);
}

function ShellNavigationItem({
  item,
  pathname,
}: {
  item: typeof navigationItems[number];
  pathname: string;
}) {
  const { t } = useTranslation();
  const { setOpenMobile } = useSidebar();
  const [iconActive, setIconActive] = useState(false);

  return (
    <SidebarMenuItem>
      <SidebarMenuButton
        isActive={isNavigationItemActive(pathname, item.to)}
        onBlur={() => setIconActive(false)}
        onClick={() => setOpenMobile(false)}
        onFocus={() => setIconActive(true)}
        onPointerEnter={() => setIconActive(true)}
        onPointerLeave={() => setIconActive(false)}
        render={<NavLink end={item.to === '/'} to={item.to} />}
        size='lg'
        tooltip={t(item.labelKey)}
      >
        <AnimatedIcon active={iconActive} name={item.icon} size={18} />
        <span>{t(item.labelKey)}</span>
      </SidebarMenuButton>
    </SidebarMenuItem>
  );
}

function ShellNavigation({ pathname }: { pathname: string }) {
  return (
    <SidebarMenu>
      {navigationItems.map((item) => (
        <ShellNavigationItem item={item} key={item.to} pathname={pathname} />
      ))}
    </SidebarMenu>
  );
}

function ShellLoadingState() {
  const { t } = useTranslation();

  return (
    <main className='shell-state' aria-busy='true' aria-live='polite'>
      <Card className='shell-state__card'>
        <CardHeader>
          <PanelLogo />
          <CardTitle>{t('shell.loading.title')}</CardTitle>
          <CardDescription>{t('shell.loading.description')}</CardDescription>
        </CardHeader>
        <CardContent className='shell-state__skeletons'>
          <Skeleton />
          <Skeleton />
          <Skeleton />
        </CardContent>
      </Card>
    </main>
  );
}

function ShellErrorState({ message, onRetry }: { message: string; onRetry: () => void }) {
  const { t } = useTranslation();

  return (
    <main className='shell-state'>
      <Card className='shell-state__card' role='alert'>
        <CardHeader>
          <Badge variant='warning'>{t('shell.error.badge')}</Badge>
          <CardTitle>{t('shell.error.description')}</CardTitle>
          <CardDescription>{message}</CardDescription>
        </CardHeader>
        <CardFooter>
          <Button onClick={onRetry}>{t('shell.error.retry')}</Button>
        </CardFooter>
      </Card>
    </main>
  );
}

export function AppShell() {
  const { t } = useTranslation();
  const { logout, session } = useAuthSession();
  const client = useApiClient();
  const controlPlane = useControlPlane();
  const location = useLocation();
  const mainRef = useRef<HTMLDivElement>(null);
  const [logoutError, setLogoutError] = useState<unknown | null>(null);
  const [loggingOut, setLoggingOut] = useState(false);
  const [panelVersion, setPanelVersion] = useState<string | null>(null);
  const [panelStatus, setPanelStatus] = useState<'loading' | 'online' | 'unavailable'>('loading');

  useEffect(() => {
    let active = true;
    let controller: AbortController | null = null;
    let generation = 0;
    let timer: number | undefined;

    function scheduleRefresh() {
      if (!active) return;
      timer = window.setTimeout(() => void refresh(), PANEL_STATUS_REFRESH_MS);
    }

    async function refresh() {
      if (timer !== undefined) {
        window.clearTimeout(timer);
        timer = undefined;
      }
      controller?.abort();
      const requestController = new AbortController();
      controller = requestController;
      const requestGeneration = ++generation;

      try {
        const status = await client.getSystemStatus(requestController.signal);
        if (!active || requestGeneration !== generation) return;
        const version = status.panel_version.trim();
        setPanelVersion(version === '' ? null : version);
        setPanelStatus('online');
      } catch {
        if (!active || requestGeneration !== generation || requestController.signal.aborted) return;
        setPanelVersion(null);
        setPanelStatus('unavailable');
      } finally {
        if (active && requestGeneration === generation) {
          if (controller === requestController) controller = null;
          scheduleRefresh();
        }
      }
    }

    function refreshWhenVisible() {
      if (document.visibilityState === 'visible') void refresh();
    }

    document.addEventListener('visibilitychange', refreshWhenVisible);
    void refresh();
    return () => {
      active = false;
      generation += 1;
      if (timer !== undefined) window.clearTimeout(timer);
      controller?.abort();
      document.removeEventListener('visibilitychange', refreshWhenVisible);
    };
  }, [client]);

  useEffect(() => {
    mainRef.current?.scrollIntoView?.({ behavior: 'auto', block: 'start' });
    mainRef.current?.focus({ preventScroll: true });
  }, [location.pathname]);

  async function signOut() {
    setLoggingOut(true);
    setLogoutError(null);
    try {
      await logout();
    } catch (error) {
      setLogoutError(error);
    } finally {
      setLoggingOut(false);
    }
  }

  if (session === null) return null;
  if (controlPlane.status === 'loading') return <ShellLoadingState />;
  if (controlPlane.status === 'error') {
    return (
      <ShellErrorState
        message={controlPlane.message}
        onRetry={() => void controlPlane.refresh()}
      />
    );
  }

  return (
    <TelemetryProvider>
      <SidebarProvider
        className='panel-shell'
        open
        style={{ '--sidebar-width': 'var(--shell-sidebar-width)' } as CSSProperties}
      >
        <a className='skip-link' href='#main-content'>{t('shell.skip')}</a>
        <Sidebar className='panel-sidebar' collapsible='offcanvas' variant='floating'>
          <SidebarHeader className='panel-sidebar__header'>
            <PanelLogo />
          </SidebarHeader>
          <SidebarContent>
            <SidebarGroup>
              <SidebarGroupContent>
                <ShellNavigation pathname={location.pathname} />
              </SidebarGroupContent>
            </SidebarGroup>
          </SidebarContent>
          <SidebarFooter className='panel-sidebar__footer'>
            <div className='panel-status-row'>
              <div
                aria-label={t(`shell.panelStatus.${panelStatus}`)}
                className='panel-status'
                data-status={panelStatus}
              >
                <Badge variant={panelStatus === 'online' ? 'success' : panelStatus === 'unavailable' ? 'warning' : 'secondary'}>
                  <span aria-hidden='true' className='panel-status__dot' />
                  {t(`shell.panelStatus.${panelStatus}`)}
                </Badge>
                <span
                  className='panel-status__version'
                  title={panelVersion === null ? undefined : `sing-box-panel ${panelVersion}`}
                >
                  {panelVersion === null ? '—' : t('shell.version', { version: panelVersion })}
                </span>
              </div>
              <Tooltip>
                <TooltipTrigger
                  render={(
                    <Button
                      aria-label={loggingOut ? t('account.signingOut') : t('account.signOut')}
                      className='panel-sign-out'
                      disabled={loggingOut}
                      onClick={() => void signOut()}
                      size='icon-sm'
                      variant='ghost'
                    />
                  )}
                >
                  {loggingOut ? <Spinner /> : <LogOut aria-hidden='true' />}
                </TooltipTrigger>
                <TooltipContent>{loggingOut ? t('account.signingOut') : t('account.signOut')}</TooltipContent>
              </Tooltip>
            </div>
          </SidebarFooter>
        </Sidebar>

        <div className='panel-workspace'>
          <TelemetryBanner />
          <div className='panel-content-scroll'>
            {logoutError === null
              ? null
              : (
                  <ErrorNotice
                    error={logoutError}
                    title={t('shell.error.logout')}
                  />
                )}
            <main id='main-content' ref={mainRef} tabIndex={-1}>
              <Outlet />
            </main>
          </div>
        </div>
      </SidebarProvider>
    </TelemetryProvider>
  );
}
