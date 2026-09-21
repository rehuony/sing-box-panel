import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useSearchParams } from 'react-router-dom';

import { useHashTab } from '@/hooks/use-hash-tab';
import { WorkspaceToolbar } from '@/components/workspace-toolbar';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';

import { CoreLogsPanel } from './core-logs-panel';
import { PanelLogsPanel } from './panel-logs-panel';
import './observability-page.css';

export function ObservabilityPage() {
  const { t } = useTranslation();
  const [params] = useSearchParams();
  const [toolbarTarget, setToolbarTarget] = useState<HTMLDivElement | null>(null);
  const [tab, setTab] = useHashTab('logs-', ['core', 'panel'] as const, params.get('tab') === 'panel' ? 'panel' : 'core', ['tab']);
  return (
    <section className='observability-page panel-page'>
      <h1 className='sr-only'>{t('productLogs.title')}</h1>
      <Tabs
        className='product-logs'
        value={tab}
        onValueChange={setTab}
      >
        <WorkspaceToolbar>
          <TabsList aria-label={t('productLogs.title')}>
            <TabsTrigger value='core'>{t('productLogs.core')}</TabsTrigger>
            <TabsTrigger value='panel'>{t('productLogs.panel')}</TabsTrigger>
          </TabsList>
          <div className='workspace-toolbar__actions' ref={setToolbarTarget} />
        </WorkspaceToolbar>
        <TabsContent value='core'>
          <CoreLogsPanel active={tab === 'core'} toolbarTarget={toolbarTarget} />
        </TabsContent>
        <TabsContent value='panel'>
          <PanelLogsPanel active={tab === 'panel'} toolbarTarget={toolbarTarget} />
        </TabsContent>
      </Tabs>
    </section>
  );
}
