import type { ReactNode } from 'react';
import type { RJSFSchema } from '@rjsf/utils';

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ScrollArea } from '@shared/components/ui/scroll-area';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@shared/components/ui/tabs';

import type { SchemaDialogGroup } from './schema-dialog-groups';

import { schemaDialogGroups } from './schema-dialog-groups';
import { SchemaDialogContext } from './schema-dialog-context';

// The same form stays mounted while tabs change: optional fields and nested drafts keep their state.
export function SchemaDialogLayout({ schema, root = schema, data, children, sectionContent, jsonPreview }: {
  schema: RJSFSchema;
  root?: RJSFSchema;
  data: unknown;
  children: ReactNode;
  sectionContent?: Partial<Record<SchemaDialogGroup, ReactNode>>;
  jsonPreview?: ReactNode;
}) {
  const { t } = useTranslation();
  const { primary, groups, groupForField, compact } = schemaDialogGroups(schema, root, data);
  const [selected, setSelected] = useState<string>(primary);
  const available = jsonPreview === undefined ? groups : [...groups, 'json'];
  const active = available.includes(selected) ? selected : primary;
  return (
    <SchemaDialogContext value={{ active, groupForField }}>
      <Tabs className='schema-dialog-tabs' orientation='vertical' data-compact={compact} value={active} onValueChange={(value) => setSelected(String(value))}>
        {/* The rail and the fields scroll on their own, each scrollbar in the padding beside its content. */}
        <ScrollArea className='schema-dialog-tabs__rail' contentClassName='schema-dialog-tabs__rail-content' hidden={available.length === 1}>
          <TabsList aria-label={t('configuration.dialog.sections')} className='schema-dialog-tabs__nav'>
            {available.map((group) => <TabsTrigger key={group} value={group}>{t(`configuration.dialog.${group}`)}</TabsTrigger>)}
          </TabsList>
        </ScrollArea>
        <TabsContent className='schema-dialog-tabs__panel' data-json={active === 'json'} value={active}>
          {/* Hidden rather than removed while the JSON shows, so the form keeps its state. */}
          <ScrollArea className='schema-dialog-tabs__scroll' contentClassName='schema-dialog-tabs__body' hidden={active === 'json'}>
            {groups.map((group) => sectionContent?.[group] === undefined
              ? null
              : (
                  <div key={group} hidden={active !== group}>{sectionContent[group]}</div>
                ))}
            <div className='schema-dialog-tabs__form'>{children}</div>
          </ScrollArea>
          {jsonPreview === undefined ? null : <div className='schema-dialog-tabs__json' hidden={active !== 'json'}>{jsonPreview}</div>}
        </TabsContent>
      </Tabs>
    </SchemaDialogContext>
  );
}
