import type { CSSProperties } from 'react';

import { LogOut } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { motion, useReducedMotion } from 'motion/react';
import { NavLink, Outlet, useLocation } from 'react-router-dom';
import { Suspense, useEffect, useLayoutEffect, useRef, useState } from 'react';

import '@/i18n';
import { Button } from '@/components/ui/button';
import { Spinner } from '@/components/ui/spinner';
import { PanelLogo } from '@/components/panel-logo';
import { Skeleton } from '@/components/ui/skeleton';
import { ErrorNotice } from '@/components/error-notice';
import { AnimatedIcon } from '@/components/animated-icon';
import { useSidebar } from '@/components/ui/sidebar-context';
import { useAuthSession } from '@/stores/auth-session.store';
import { useControlPlane } from '@/stores/control-plane.store';
import { useUnsavedChangesContext } from '@/stores/unsaved-changes.store';
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

import { LanguageMenu } from './language-menu';
import { TelemetryBanner } from './telemetry-banner';
import { ThemeCycleButton } from './theme-cycle-button';
import { TelemetryProvider } from './telemetry-provider';
import './app-shell.css';

const navigationItems = [
  { icon: 'dashboard', labelKey: 'nav.dashboard', to: '/' },
  { icon: 'cores', labelKey: 'nav.cores', to: '/cores' },
  { icon: 'subscriptions', labelKey: 'nav.subscriptions', to: '/subscriptions' },
  { icon: 'configuration', labelKey: 'nav.configuration', to: '/configuration' },
  { icon: 'panel', labelKey: 'nav.panel', to: '/panel' },
  { icon: 'observability', labelKey: 'nav.observability', to: '/observability' },
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
  const reducedMotion = useReducedMotion();
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
        {isNavigationItemActive(pathname, item.to)
          ? (
              <motion.span
                aria-hidden
                className='panel-navigation-selection'
                layoutId='panel-navigation-selection'
                transition={reducedMotion ? { duration: 0 } : { type: 'spring', bounce: 0, duration: 0.35 }}
              />
            )
          : null}
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
      <Card className='shell-state__card'>
        <CardHeader>
          <CardTitle>Sing-Box Panel</CardTitle>
          <ErrorNotice error={message} title={t('shell.error.description')} />
        </CardHeader>
        <CardFooter>
          <Button onClick={onRetry}>{t('shell.error.retry')}</Button>
        </CardFooter>
      </Card>
    </main>
  );
}

function WorkspaceLoadingState() {
  const { t } = useTranslation();
  return (
    <div className='shell-route-loading' aria-busy='true' aria-live='polite'>
      <Spinner />
      <span>{t('shell.loading.page')}</span>
    </div>
  );
}

export function AppShell() {
  const { t } = useTranslation();
  const { logout, session } = useAuthSession();
  const { confirmNavigation } = useUnsavedChangesContext();
  const controlPlane = useControlPlane();
  const location = useLocation();
  const mainRef = useRef<HTMLDivElement>(null);
  const scrollRef = useRef<HTMLDivElement>(null);
  const [logoutError, setLogoutError] = useState<unknown | null>(null);
  const [loggingOut, setLoggingOut] = useState(false);
  useEffect(() => {
    void import('@/pages/dashboard-page/dashboard-page');
  }, []);
  useEffect(() => {
    if (scrollRef.current) scrollRef.current.scrollTop = 0;
    mainRef.current?.focus({ preventScroll: true });
  }, [location.pathname]);
  useLayoutEffect(() => {
    const scroll = scrollRef.current;
    if (!scroll) return;
    const measure = () => scroll.style.setProperty('--shell-scrollbar-width', `${scroll.offsetWidth - scroll.clientWidth}px`);
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(scroll);
    return () => observer.disconnect();
  }, [controlPlane.status]);

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
        <a className='skip-link' href='#main-content' onClick={event => {
          event.preventDefault();
          mainRef.current?.focus();
        }}>
          {t('shell.skip')}
        </a>
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
              <ThemeCycleButton />
              <LanguageMenu />
              <Tooltip>
                <TooltipTrigger
                  render={(
                    <Button
                      aria-label={loggingOut ? t('account.signingOut') : t('account.signOut')}
                      className='panel-sign-out'
                      disabled={loggingOut}
                      onClick={() => confirmNavigation(() => void signOut())}
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
          <div className='panel-content-scroll' ref={scrollRef}>
            {logoutError === null
              ? null
              : (
                  <ErrorNotice
                    error={logoutError}
                    title={t('shell.error.logout')}
                  />
                )}
            <main id='main-content' ref={mainRef} tabIndex={-1}>
              <Suspense fallback={<WorkspaceLoadingState />}>
                <Outlet />
              </Suspense>
            </main>
          </div>
        </div>
      </SidebarProvider>
    </TelemetryProvider>
  );
}
