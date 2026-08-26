import { Navigate, Outlet, Route, Routes, useLocation } from 'react-router-dom';

import { CoresPage } from '@/pages/cores-page';
import { LoginPage } from '@/pages/login-page';
import { TasksPage } from '@/pages/tasks-page';
import { AppShell } from '@/components/app-shell';
import { NotFoundPage } from '@/pages/not-found-page';
import { DashboardPage } from '@/pages/dashboard-page';
import { useAuthSession } from '@/stores/auth-session.store';
import { ConfigurationPage } from '@/pages/configuration-page';
import { ObservabilityPage } from '@/pages/observability-page';
import { SubscriptionsPage } from '@/pages/subscriptions-page';
import { ControlPlaneProvider } from '@/stores/control-plane-provider';

function ProtectedRoute() {
  const { status } = useAuthSession();
  const location = useLocation();

  if (status === 'checking') {
    return (
      <main className='loading-screen' aria-busy='true' aria-live='polite'>
        <span aria-hidden='true' className='loading-screen__mark' />
        <p>Checking panel session…</p>
      </main>
    );
  }

  if (status === 'anonymous') {
    return <Navigate replace state={{ from: location.pathname }} to='/login' />;
  }

  return (
    <ControlPlaneProvider>
      <Outlet />
    </ControlPlaneProvider>
  );
}

export function AppRoutes() {
  return (
    <Routes>
      <Route element={<ProtectedRoute />}>
        <Route element={<AppShell />}>
          <Route element={<DashboardPage />} index />
          <Route element={<ConfigurationPage />} path='configuration' />
          <Route element={<CoresPage />} path='cores' />
          <Route element={<SubscriptionsPage />} path='subscriptions' />
          <Route element={<ObservabilityPage />} path='observability' />
          <Route element={<TasksPage />} path='tasks' />
          <Route element={<NotFoundPage />} path='*' />
        </Route>
      </Route>
      <Route element={<LoginPage />} path='/login' />
    </Routes>
  );
}
