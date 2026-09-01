import { Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';

import type { RuntimeHistoryPage } from '@/api/api-client';

import { Button } from '@/components/ui/button';
import { ErrorNotice } from '@/components/error-notice';

export interface ActivationHistoryProps {
  error: unknown;
  loading: boolean;
  onRefresh: () => Promise<void>;
  page: RuntimeHistoryPage | null;
}

function formatTimestamp(timestamp: string, locale: string): string {
  return new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'short' })
    .format(new Date(timestamp));
}

export function ActivationHistory({ error, loading, onRefresh, page }: ActivationHistoryProps) {
  const { i18n, t } = useTranslation();
  const items = page?.items.filter((item) => item.activation_bundle_id !== undefined) ?? [];

  return (
    <section aria-labelledby='activation-history-title'>
      <div className='section-heading'>
        <div>
          <h3 id='activation-history-title'>{t('configuration.deploy.activations.title')}</h3>
          <span>{t('configuration.deploy.activations.loaded', { count: items.length })}</span>
        </div>
        <Button
          aria-busy={loading}
          disabled={loading}
          onClick={() => void onRefresh()}
          type='button'
          variant='outline'
        >
          {loading
            ? t('configuration.deploy.activations.loading')
            : t('configuration.deploy.activations.refresh')}
        </Button>
      </div>

      {error === null
        ? null
        : <ErrorNotice error={error} title={t('configuration.deploy.activations.error')} />}
      {items.length === 0 && !loading
        ? <p className='source-note'>{t('configuration.deploy.activations.empty')}</p>
        : (
            <div className='revision-history__list'>
              {items.map((transition) => (
                <article className='revision-card' key={transition.id}>
                  <div>
                    <strong><code>{transition.activation_bundle_id}</code></strong>
                    <span className={`state-label state-label--${transition.state === 'running' ? 'success' : transition.state === 'failed' ? 'danger' : 'warning'}`}>
                      {t(`configuration.deploy.activations.state.${transition.state}`)}
                    </span>
                    <span>{formatTimestamp(transition.occurred_at, i18n.language)}</span>
                  </div>
                  <dl className='detail-list'>
                    <div>
                      <dt>{t('configuration.deploy.activations.reason')}</dt>
                      <dd><code>{transition.reason}</code></dd>
                    </div>
                    <div>
                      <dt>{t('configuration.deploy.activations.generation')}</dt>
                      <dd>{transition.generation === undefined ? '—' : new Intl.NumberFormat(i18n.language).format(transition.generation)}</dd>
                    </div>
                    <div>
                      <dt>{t('configuration.deploy.activations.task')}</dt>
                      <dd>{transition.task_id === undefined ? '—' : <code>{transition.task_id}</code>}</dd>
                    </div>
                  </dl>
                  {transition.task_id === undefined
                    ? null
                    : <Link className='text-link' to='/tasks'>{t('configuration.deploy.openTasks')}</Link>}
                </article>
              ))}
            </div>
          )}
      {page?.next === undefined
        ? null
        : <p className='source-note'>{t('configuration.deploy.activations.truncated')}</p>}
    </section>
  );
}
