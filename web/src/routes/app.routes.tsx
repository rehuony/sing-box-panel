import { lazy, Suspense } from 'react';
import { useTranslation } from 'react-i18next';
import { Navigate, Outlet, Route, Routes, useLocation } from 'react-router-dom';

import { LoginPage } from '@/pages/login-page';
import { AppShell } from '@/components/app-shell';
import { NotFoundPage } from '@/pages/not-found-page';
import { LoadingState } from '@/components/loading-state';
import { useAuthSession } from '@/stores/auth-session.store';
import { ControlPlaneProvider } from '@/stores/control-plane-provider';
import { PanelSettingsProvider } from '@/stores/panel-settings-provider';

import { loadPage } from './page-loaders';

const PanelSettingsPage = lazy(() => loadPage('/panel'));
const ConfigurationPage = lazy(() => loadPage('/configuration'));
const CoresPage = lazy(() => loadPage('/cores'));
const DashboardPage = lazy(() => loadPage('/'));
const ObservabilityPage = lazy(() => loadPage('/observability'));
const SubscriptionsPage = lazy(() => loadPage('/subscriptions'));

function RouteLoadingState() {
  const { t } = useTranslation();
  return <LoadingState label={t('shell.loading.page')} />;
}

function ProtectedRoute() {
  const { status } = useAuthSession();
  const location = useLocation();

  if (status === 'checking') {
    return <LoginPage />;
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

  if (status === 'unavailable') return <LoginPage />;

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
