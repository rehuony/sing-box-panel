import { X } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useSearchParams } from 'react-router-dom';
import { Button } from '@shared/components/ui/button';
import { Tabs, TabsList, TabsTrigger } from '@shared/components/ui/tabs';

import { StripDivider } from '@/components/title-strip';

import type { NodeFilter, PanelLogView } from './use-panel-log-filters';

/** The window's views as the strip's first tabs. Switching keeps the node filter. */
export function ViewTabs({ view }: { view: PanelLogView }) {
  const { t } = useTranslation();
  const [, setParams] = useSearchParams();
  return (
    <Tabs
      className='max-w-full min-w-0 flex-none'
      onValueChange={(value) => {
        setParams((current) => {
          const next = new URLSearchParams(current);
          if (value === 'states') next.set('view', 'states');
          else next.delete('view');
          return next;
        });
      }}
      value={view}
    >
      <TabsList aria-label={t('productLogs.tabs.label')} className='max-w-full overflow-x-auto [scrollbar-width:none]' variant='pill'>
        <TabsTrigger className='flex-none' value='logs'>{t('productLogs.tabs.logs')}</TabsTrigger>
        <TabsTrigger className='flex-none' value='states'>{t('productLogs.tabs.states')}</TabsTrigger>
      </TabsList>
    </Tabs>
  );
}

/** The node filter as a strip chip that clears it, followed by its divider. */
export function NodeFilterChip({ node, onClear }: { node: NodeFilter; onClear: () => void }) {
  const { t } = useTranslation();
  if (!node.id) return null;
  return (
    <>
      <Button
        aria-label={t('productLogs.clearNode', { name: node.name })}
        className='max-w-48 min-w-0 flex-none bg-accent text-foreground'
        onClick={() => {
          node.clear();
          onClear();
        }}
        size='sm'
        type='button'
        variant='flat'
      >
        <span className='truncate'>{node.name}</span>
        <X aria-hidden='true' data-icon='inline-end' />
      </Button>
      <StripDivider />
    </>
  );
}
