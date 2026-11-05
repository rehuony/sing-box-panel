import type { CSSProperties } from 'react';

import { LogOut } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@shared/components/ui/button';
import { Spinner } from '@shared/components/ui/spinner';
import { motion, useReducedMotion } from 'motion/react';
import { Suspense, useEffect, useRef, useState } from 'react';

import '@/i18n';
import { ScrollArea } from '@shared/components/ui/scroll-area';
import { NavLink, Outlet, useLocation } from 'react-router-dom';
import { useSidebar } from '@shared/components/ui/sidebar-context';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@shared/components/ui/tooltip';
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
} from '@shared/components/ui/sidebar';

import type { PagePath } from '@/routes/page-loaders';

import { PanelLogo } from '@/components/panel-logo';
import { preloadPage } from '@/routes/page-loaders';
import { ErrorNotice } from '@/components/error-notice';
import { AnimatedIcon } from '@/components/animated-icon';
import { LoadingState } from '@/components/loading-state';
import { useAuthSession } from '@/stores/auth-session.store';
import { useUnsavedChangesContext } from '@/stores/unsaved-changes.store';

import { LanguageMenu } from './language-menu';
import { ThemeCycleButton } from './theme-cycle-button';

const navigationItems = [
  { icon: 'nodes', labelKey: 'nav.nodes', page: 'nodes', to: '/nodes' },
  { icon: 'subscriptions', labelKey: 'nav.subscriptions', page: 'subscriptions', to: '/subscriptions' },
  { icon: 'logs', labelKey: 'nav.logs', page: 'logs', to: '/logs' },
  { icon: 'settings', labelKey: 'nav.settings', page: 'settings', to: '/settings' },
] as const satisfies readonly { icon: string; labelKey: string; page: PagePath; to: string }[];

function isNavigationItemActive(pathname: string, to: string): boolean {
  return pathname === to || pathname.startsWith(`${to}/`);
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
  const active = isNavigationItemActive(pathname, item.to);

  return (
    <SidebarMenuItem>
      <SidebarMenuButton
        className='relative isolate gap-3 overflow-visible px-3 font-medium text-[var(--color-text-secondary)] transition-[background-color,color] duration-[var(--dur-short)] ease-out hover:bg-[var(--color-surface-raised)] data-active:bg-transparent data-active:text-foreground [&_svg]:size-[18px]'
        isActive={active}
        onBlur={() => setIconActive(false)}
        onClick={() => setOpenMobile(false)}
        onFocus={() => {
          setIconActive(true);
          preloadPage(item.page);
        }}
        onPointerEnter={() => {
          setIconActive(true);
          preloadPage(item.page);
        }}
        onPointerDown={() => preloadPage(item.page)}
        onPointerLeave={() => setIconActive(false)}
        render={<NavLink to={item.to} />}
        size='lg'
        tooltip={t(item.labelKey)}
      >
        {active
          ? (
              <motion.span
                aria-hidden
                className='pointer-events-none absolute inset-0 -z-10 rounded-[inherit] bg-accent'
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

function WorkspaceLoadingState() {
  const { t } = useTranslation();
  return <LoadingState label={t('shell.loading.page')} fullScreen={false} />;
}

export function AppShell() {
  const { t } = useTranslation();
  const { logout, session } = useAuthSession();
  const { confirmNavigation } = useUnsavedChangesContext();
  const location = useLocation();
  const mainRef = useRef<HTMLDivElement>(null);
  const scrollRef = useRef<HTMLDivElement>(null);
  const [logoutError, setLogoutError] = useState<unknown | null>(null);
  const [loggingOut, setLoggingOut] = useState(false);
  useEffect(() => {
    preloadPage('nodes');
  }, []);
  useEffect(() => {
    if (scrollRef.current) scrollRef.current.scrollTop = 0;
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

  return (
    <SidebarProvider
      className='h-dvh min-h-0 overflow-hidden'
      open
      style={{ '--sidebar-width': 'var(--shell-sidebar-width)' } as CSSProperties}
    >
      <a
        className='fixed top-3 left-4 z-[var(--z-modal)] -translate-y-[160%] rounded-[var(--radius-control)] bg-primary px-4 py-3 text-sm font-medium text-primary-foreground transition-transform duration-[var(--dur-short)] ease-[var(--ease-out)] focus:translate-y-0'
        href='#main-content'
        onClick={(event) => {
          event.preventDefault();
          mainRef.current?.focus();
        }}
      >
        {t('shell.skip')}
      </a>
      <Sidebar className='p-[var(--shell-edge)]' collapsible='offcanvas' variant='floating'>
        <SidebarHeader className='min-h-19 justify-center p-4'>
          <PanelLogo />
        </SidebarHeader>
        <SidebarContent className='p-2'>
          <SidebarGroup className='p-0'>
            <SidebarGroupContent>
              <SidebarMenu className='gap-1'>
                {navigationItems.map(item => (
                  <ShellNavigationItem item={item} key={item.to} pathname={location.pathname} />
                ))}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        </SidebarContent>
        <SidebarFooter className='flex-row items-center gap-2 p-3'>
          <ThemeCycleButton />
          <LanguageMenu />
          <Tooltip>
            <TooltipTrigger
              render={(
                <Button
                  aria-label={loggingOut ? t('account.signingOut') : t('account.signOut')}
                  className='ml-auto'
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
        </SidebarFooter>
      </Sidebar>

      <ScrollArea
        className='h-dvh min-w-0 flex-1'
        viewportClassName='flex flex-col px-4 py-[var(--shell-edge)] md:pr-[var(--shell-edge)] md:pl-0'
        viewportRef={scrollRef}
      >
        {logoutError === null ? null : <ErrorNotice error={logoutError} title={t('shell.error.logout')} />}
        <main className='@container flex min-w-0 flex-1 flex-col outline-none' id='main-content' ref={mainRef} tabIndex={-1}>
          <Suspense fallback={<WorkspaceLoadingState />}>
            <Outlet />
          </Suspense>
        </main>
      </ScrollArea>
    </SidebarProvider>
  );
}
