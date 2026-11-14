import { lazy, Suspense } from 'react';
import { useTranslation } from 'react-i18next';
import { Navigate, Outlet, Route, Routes, useLocation } from 'react-router-dom';

import { LoginPage } from '@/pages/login-page';
import { AppShell } from '@/components/app-shell';
import { NotFoundPage } from '@/pages/not-found-page';
import { LoadingState } from '@/components/loading-state';
import { NodeWorkspace } from '@/components/node-workspace';
import { useAuthSession } from '@/stores/auth-session.store';
import { PanelSettingsProvider } from '@/stores/panel-settings-provider';
import { ConfigurationSessionsProvider } from '@/stores/configuration-session-provider';

import { loadPage } from './page-loaders';

const FleetPage = lazy(() => loadPage('nodes'));
const NodeOverviewPage = lazy(() => loadPage('node-overview'));
const CoresPage = lazy(() => loadPage('node-cores'));
const ConfigurationPage = lazy(() => loadPage('node-configuration'));
const NodeLogsPage = lazy(() => loadPage('node-logs'));
const NodeSettingsPage = lazy(() => loadPage('node-settings'));
const SubscriptionsPage = lazy(() => loadPage('subscriptions'));
const PanelLogsPage = lazy(() => loadPage('logs'));
const PanelSettingsPage = lazy(() => loadPage('settings'));

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
    <ConfigurationSessionsProvider>
      <PanelSettingsProvider><Outlet /></PanelSettingsProvider>
    </ConfigurationSessionsProvider>
  );
}

export function AppRoutes() {
  return (
    <Suspense fallback={<RouteLoadingState />}>
      <Routes>
        <Route element={<ProtectedRoute />}>
          <Route element={<AppShell />}>
            <Route element={<Navigate replace to='/nodes' />} index />
            <Route element={<Navigate replace to='/nodes' />} path='overview' />
            <Route element={<FleetPage />} path='nodes' />
            <Route element={<NodeWorkspace />} path='nodes/:nodeId'>
              <Route element={<NodeOverviewPage />} index />
              <Route element={<CoresPage />} path='cores' />
              <Route element={<ConfigurationPage />} path='configuration' />
              <Route element={<NodeLogsPage />} path='logs' />
              <Route element={<NodeSettingsPage />} path='settings' />
              <Route element={<NotFoundPage />} path='*' />
            </Route>
            <Route element={<SubscriptionsPage />} path='subscriptions' />
            <Route element={<PanelLogsPage />} path='logs' />
            <Route element={<PanelSettingsPage />} path='settings' />
            <Route element={<NotFoundPage />} path='*' />
          </Route>
        </Route>
        <Route element={<LoginPage />} path='/login' />
      </Routes>
    </Suspense>
  );
}
