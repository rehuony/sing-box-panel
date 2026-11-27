import { useTranslation } from 'react-i18next';

import { LogEntriesView } from '@/components/panel-log';

/** The panel's own log, about no node; each node's events and runtime states are in its logs window. */
export function PanelLogsPage() {
  const { t } = useTranslation();
  return (
    <section className='flex min-h-0 min-w-0 flex-1 flex-col gap-[var(--workspace-content-gap)] [contain:size]'>
      <h1 className='sr-only'>{t('nav.logs')}</h1>
      <LogEntriesView scope={{ panel: true }} />
    </section>
  );
}
