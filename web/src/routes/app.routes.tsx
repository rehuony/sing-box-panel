import { lazy, Suspense } from 'react';
import { useTranslation } from 'react-i18next';
import { Navigate, Outlet, Route, Routes, useLocation } from 'react-router-dom';

import { LoginPage } from '@/pages/login-page';
import { Button } from '@/components/ui/button';
import { AppShell } from '@/components/app-shell';
import { NotFoundPage } from '@/pages/not-found-page';
import { useAuthSession } from '@/stores/auth-session.store';
import { ControlPlaneProvider } from '@/stores/control-plane-provider';
import { PanelSettingsProvider } from '@/stores/panel-settings-provider';

const PanelSettingsPage = lazy(async () => {
  const page = await import('@/pages/panel-settings-page/panel-settings-page');
  return { default: page.PanelSettingsPage };
});
const ConfigurationPage = lazy(async () => {
  const page = await import('@/pages/configuration-page/configuration-page');
  return { default: page.ConfigurationPage };
});
const CoresPage = lazy(async () => {
  const page = await import('@/pages/cores-page/cores-page');
  return { default: page.CoresPage };
});
const DashboardPage = lazy(async () => {
  const page = await import('@/pages/dashboard-page/dashboard-page');
  return { default: page.DashboardPage };
});
const ObservabilityPage = lazy(async () => {
  const page = await import('@/pages/observability-page/observability-page');
  return { default: page.ObservabilityPage };
});
const SubscriptionsPage = lazy(async () => {
  const page = await import('@/pages/subscriptions-page/subscriptions-page');
  return { default: page.SubscriptionsPage };
});

function RouteLoadingState() {
  const { t } = useTranslation();
  return (
    <main className='loading-screen' aria-busy='true' aria-live='polite'>
      <span aria-hidden='true' className='loading-screen__mark' />
      <p>{t('shell.loading.page')}</p>
    </main>
  );
}

function ProtectedRoute() {
  const { t } = useTranslation();
  const { retrySession, status } = useAuthSession();
  const location = useLocation();

  if (status === 'checking') {
    return (
      <main className='loading-screen' aria-busy='true' aria-live='polite'>
        <span aria-hidden='true' className='loading-screen__mark' />
        <p>{t('login.checking')}</p>
      </main>
    );
  }

  if (status === 'anonymous') {
    return (
      <Navigate
        replace
        state={{ from: `${location.pathname}${location.search}${location.hash}` }}
        to='/login'
      />
    );
  }

  if (status === 'unavailable') {
    return (
      <main className='loading-screen'>
        <div className='load-error' role='alert'>
          <h1>{t('login.unavailable.title')}</h1>
          <p>{t('login.unavailable.description')}</p>
          <Button onClick={retrySession} type='button'>{t('login.unavailable.retry')}</Button>
        </div>
      </main>
    );
  }

  return (
    <ControlPlaneProvider>
      <PanelSettingsProvider><Outlet /></PanelSettingsProvider>
    </ControlPlaneProvider>
  );
}

export function AppRoutes() {
  return (
    <Suspense fallback={<RouteLoadingState />}>
      <Routes>
        <Route element={<ProtectedRoute />}>
          <Route element={<AppShell />}>
            <Route element={<DashboardPage />} index />
            <Route element={<ConfigurationPage />} path='configuration' />
            <Route element={<CoresPage />} path='cores' />
            <Route element={<PanelSettingsPage />} path='panel' />
            <Route element={<SubscriptionsPage />} path='subscriptions' />
            <Route element={<ObservabilityPage />} path='observability' />
            <Route element={<NotFoundPage />} path='*' />
          </Route>
        </Route>
        <Route element={<LoginPage />} path='/login' />
      </Routes>
    </Suspense>
  );
}
