import { useTranslation } from 'react-i18next';

import { LogEntriesView } from './log-entries-view';
import { RuntimeStatesView } from './runtime-states-view';
import { useNodeFilter, usePanelLogView } from './use-panel-log-filters';

/**
 * The panel's log window in two views: the log, and every node's runtime
 * states. The view and the node filter stay in the address, so they can be
 * shared and cleared.
 */
export function PanelLogsPage() {
  const { t } = useTranslation();
  const view = usePanelLogView();
  const node = useNodeFilter();
  return (
    <section className='flex min-h-0 min-w-0 flex-1 flex-col gap-[var(--workspace-content-gap)]'>
      <h1 className='sr-only'>{t('nav.logs')}</h1>
      {view === 'states' ? <RuntimeStatesView node={node} /> : <LogEntriesView node={node} />}
    </section>
  );
}
