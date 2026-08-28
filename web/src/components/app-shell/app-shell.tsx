import { NavLink, Outlet } from 'react-router-dom';

import type { DashboardContext } from '@/api/api-client';

import { PanelLogo } from '@/components/panel-logo';
import { ContextRail } from '@/components/context-rail';
import { useAuthSession } from '@/stores/auth-session.store';
import { useControlPlane } from '@/stores/control-plane.store';

import './app-shell.css';

const navigationItems = [
  { label: 'Overview', to: '/' },
  { label: 'Configuration', to: '/configuration' },
  { label: 'Core versions', to: '/cores' },
  { label: 'Subscriptions', to: '/subscriptions' },
  { label: 'Observability', to: '/observability' },
  { label: 'Tasks', to: '/tasks' },
] as const;

export function AppShell() {
  const { logout, session } = useAuthSession();
  const controlPlane = useControlPlane();

  if (session === null) {
    return null;
  }

  if (controlPlane.status === 'loading') {
    return (
      <main className='loading-screen' aria-busy='true' aria-live='polite'>
        <span aria-hidden='true' className='loading-screen__mark' />
        <p>Reading exact runtime identity…</p>
      </main>
    );
  }

  if (controlPlane.status === 'error') {
    return (
      <main className='loading-screen'>
        <div className='load-error' role='alert'>
          <p className='eyebrow'>Context unavailable</p>
          <h1>The control plane did not answer.</h1>
          <p>{controlPlane.message}</p>
          <button
            className='button button--primary'
            onClick={() => void controlPlane.refresh()}
            type='button'
          >
            Try again
          </button>
        </div>
      </main>
    );
  }

  const serverContext = controlPlane.context;
  const context: DashboardContext = {
    ...serverContext,
    view: { exactVersion: controlPlane.viewVersion || 'Not selected' },
  };

  return (
    <div className='app-shell'>
      <a className='skip-link' href='#main-content'>
        Skip to main content
      </a>
      <header className='app-header'>
        <PanelLogo />
        <div className='app-header__actions'>
          <span className='environment-label'>
            <span aria-hidden='true' />
            {' '}
            Local control plane
          </span>
          <span className='session-name'>{session.displayName}</span>
          <button
            className='button button--quiet'
            onClick={() => void logout()}
            type='button'
          >
            Sign out
          </button>
        </div>
      </header>

      <div className='app-shell__layout'>
        <aside className='side-panel'>
          <nav aria-label='Primary navigation'>
            <p className='side-panel__label'>Control plane</p>
            <ul>
              {navigationItems.map((item) => (
                <li key={item.to}>
                  <NavLink
                    className={({ isActive }) =>
                      isActive ? 'side-link side-link--active' : 'side-link'
                    }
                    end={item.to === '/'}
                    to={item.to}
                  >
                    <span aria-hidden='true' className='side-link__index' />
                    {item.label}
                  </NavLink>
                </li>
              ))}
            </ul>
          </nav>

          <div className='side-panel__runtime'>
            <span className='side-panel__label'>Exact runtime</span>
            <strong className={context.running === null ? 'is-stopped' : ''}>
              <span aria-hidden='true' className='live-mark' />
              {context.running === null
                ? 'Core stopped'
                : context.running.exactVersion}
            </strong>
            <span>
              {context.running === null
                ? 'No running identity was reported'
                : context.running.artifactName}
            </span>
          </div>
        </aside>

        <div className='app-shell__content'>
          <ContextRail context={context} />
          <main id='main-content' tabIndex={-1}>
            <Outlet />
          </main>
        </div>
      </div>
    </div>
  );
}
