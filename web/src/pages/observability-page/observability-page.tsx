import { useTranslation } from 'react-i18next';
import { useSearchParams } from 'react-router-dom';

import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';

import { CoreLogsPanel } from './core-logs-panel';
import { PanelLogsPanel } from './panel-logs-panel';
import './observability-page.css';

export function ObservabilityPage() {
  const { t } = useTranslation();
  const [params, setParams] = useSearchParams();
  const tab = params.get('tab') === 'panel' || params.has('task') ? 'panel' : 'core';
  return (
    <section className='observability-page'>
      <h1>{t('productLogs.title')}</h1>
      <Tabs
        className='product-logs'
        value={tab}
        onValueChange={(value) => {
          setParams((next) => {
            next.set('tab', value);
            next.delete('task');
            return next;
          });
        }}
      >
        <TabsList aria-label={t('productLogs.title')}>
          <TabsTrigger value='core'>{t('productLogs.core')}</TabsTrigger>
          <TabsTrigger value='panel'>{t('productLogs.panel')}</TabsTrigger>
        </TabsList>
        <TabsContent value='core'>
          <CoreLogsPanel />
        </TabsContent>
        <TabsContent value='panel'>
          <PanelLogsPanel />
        </TabsContent>
      </Tabs>
    </section>
  );
}
