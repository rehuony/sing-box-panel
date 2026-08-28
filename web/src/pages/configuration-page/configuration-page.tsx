import { ErrorNotice } from '@/components/error-notice';
import { PageHeading } from '@/components/page-heading';
import { useControlPlane } from '@/stores/control-plane.store';

import { StartupWorkflow } from './startup-workflow';
import { ConfigurationFields } from './configuration-fields';
import { ManagedCollectionEditor } from './managed-collection-editor';
import { useCanonicalConfiguration } from './use-canonical-configuration';
import './configuration-page.css';

function formatTimestamp(timestamp: string): string {
  return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' })
    .format(new Date(timestamp));
}

export function ConfigurationPage() {
  const controlPlane = useControlPlane();
  const canonical = useCanonicalConfiguration();
  const exactVersion = controlPlane.viewVersion;
  const currentRevisionID = canonical.state.status === 'ready' ? canonical.state.snapshot.id : '';

  return (
    <div className='page-stack configuration-page'>
      <PageHeading
        eyebrow='Configuration / global history'
        summary='One version-independent history is projected through the selected binary’s exact compiled adapter. Fields unsupported by that version stay saved and return when a supporting version is selected.'
        title='Manage one configuration across versions.'
      />

      {canonical.state.status === 'error'
        ? <ErrorNotice error={canonical.state.error} title='Configuration is unavailable' />
        : null}
      {canonical.saveError === null
        ? null
        : <ErrorNotice error={canonical.saveError} title='Configuration was not saved' />}
      {canonical.message === ''
        ? null
        : (
            <div className='notice notice--success' role='status'>
              <strong>Global history updated</strong>
              <p>{canonical.message}</p>
            </div>
          )}

      {canonical.state.status === 'loading'
        ? <div className='inline-loading' aria-busy='true'>Loading global configuration…</div>
        : null}

      {canonical.state.status === 'ready'
        ? (
            <>
              <section className='canonical-summary' aria-labelledby='canonical-summary-title'>
                <div>
                  <p className='eyebrow'>Current immutable revision</p>
                  <h2 id='canonical-summary-title'>
                    Revision #
                    {canonical.state.snapshot.sequence}
                  </h2>
                  <small>
                    {formatTimestamp(canonical.state.snapshot.created_at)}
                    {' '}
                    ·
                    {' '}
                    {canonical.state.snapshot.id}
                  </small>
                </div>
                <div className='inline-actions'>
                  <button className='button button--quiet' disabled={canonical.saving} onClick={canonical.reset} type='button'>Discard unsaved edits</button>
                  <button className='button button--primary' disabled={canonical.saving} onClick={() => void canonical.save()} type='button'>
                    {canonical.saving ? 'Saving…' : 'Save new revision'}
                  </button>
                </div>
              </section>

              <ConfigurationFields draft={canonical.state.draft} onChange={canonical.update} />
              <ManagedCollectionEditor collection='inbounds' draft={canonical.state.draft} onChange={canonical.update} />
              <ManagedCollectionEditor collection='outbounds' draft={canonical.state.draft} onChange={canonical.update} />

              <section className='revision-section' aria-labelledby='revision-history-title'>
                <div className='section-heading'>
                  <div>
                    <p className='eyebrow'>Immutable audit trail</p>
                    <h2 id='revision-history-title'>Recent global revisions</h2>
                  </div>
                  <span>
                    {canonical.revisions?.items.length ?? 0}
                    {' '}
                    loaded
                  </span>
                </div>
                {canonical.revisionError === null ? null : <ErrorNotice error={canonical.revisionError} title='Revision history is unavailable' />}
                <div className='artifact-list'>
                  {(canonical.revisions?.items ?? []).map((revision) => (
                    <article key={revision.id}>
                      <div>
                        <strong>
                          Revision #
                          {revision.sequence}
                        </strong>
                        <span>{formatTimestamp(revision.created_at)}</span>
                        <code>{revision.id}</code>
                      </div>
                      {revision.id === currentRevisionID
                        ? <span className='state-label state-label--success'>current</span>
                        : <button className='text-button' disabled={canonical.saving} onClick={() => void canonical.restore(revision.id)} type='button'>Restore as new revision</button>}
                    </article>
                  ))}
                </div>
              </section>
            </>
          )
        : null}

      <StartupWorkflow exactVersion={exactVersion} key={exactVersion} />
    </div>
  );
}
