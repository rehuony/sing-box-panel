import type { DashboardContext } from '@/api/api-client';

import './context-rail.css';

export interface ContextRailProps {
  context: DashboardContext;
}

function compactDigest(digest: string): string {
  return digest.length > 14 ? `${digest.slice(0, 11)}…` : digest;
}

export function ContextRail({ context }: ContextRailProps) {
  const running = context.running;
  const applied = context.applied;
  const hasWarning = context.capability.warning !== null;

  return (
    <section className="context-rail" aria-labelledby="context-rail-title">
      <div className="context-rail__heading">
        <div>
          <p className="eyebrow">Version rail / active context</p>
          <h2 id="context-rail-title">Exact-version rail</h2>
        </div>
        <span
          className={`support-pill support-pill--${context.capability.level}`}
        >
          <span aria-hidden="true" className="support-pill__mark" />
          {context.capability.label}
        </span>
      </div>

      <ol className="context-rail__track">
        <li className="is-selected">
          <span className="context-rail__node" aria-hidden="true" />
          <span className="context-rail__label">Selected</span>
          <strong>{context.view.exactVersion}</strong>
        </li>
        <li className={running === null ? 'is-stopped' : 'is-running'}>
          <span className="context-rail__node" aria-hidden="true" />
          <span className="context-rail__label">Running</span>
          <strong>
            {running === null
              ? 'Stopped'
              : `${running.exactVersion} · ${compactDigest(running.digest)}`}
          </strong>
        </li>
        <li className={context.canonical.hasUnappliedChanges ? 'is-warning' : ''}>
          <span className="context-rail__node" aria-hidden="true" />
          <span className="context-rail__label">Canonical</span>
          <strong>Revision #{context.canonical.revision}</strong>
        </li>
        <li>
          <span className="context-rail__node" aria-hidden="true" />
          <span className="context-rail__label">Applied</span>
          <strong>
            {applied === null
              ? 'Nothing applied'
              : `${applied.bundle} · Revision #${applied.revision}`}
          </strong>
        </li>
      </ol>

      {hasWarning ? (
        <div className="context-rail__warning" role="status">
          <svg aria-hidden="true" viewBox="0 0 24 24">
            <path d="M12 3.5 21 20H3L12 3.5Z" />
            <path d="M12 9v5m0 3v.1" />
          </svg>
          <div>
            <strong>Capability attention required</strong>
            <p>{context.capability.warning}</p>
          </div>
        </div>
      ) : null}
    </section>
  );
}
