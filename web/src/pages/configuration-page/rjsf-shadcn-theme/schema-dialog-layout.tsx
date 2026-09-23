import type { ReactNode } from 'react';
import type { RJSFSchema } from '@rjsf/utils';

import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';

import { schemaDialogGroups } from './schema-dialog-groups';
import { SchemaDialogContext } from './schema-dialog-context';

// The same form stays mounted while tabs change: optional fields and nested drafts keep their state.
export function SchemaDialogLayout({ schema, root = schema, data, children, primaryContent, jsonPreview }: {
  schema: RJSFSchema;
  root?: RJSFSchema;
  data: unknown;
  children: ReactNode;
  primaryContent?: ReactNode;
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
        <TabsList hidden={available.length === 1} aria-label={t('configuration.dialog.sections')} className='schema-dialog-tabs__nav'>
          {available.map((group) => <TabsTrigger key={group} value={group}>{t(`configuration.dialog.${group}`)}</TabsTrigger>)}
        </TabsList>
        <TabsContent className='schema-dialog-tabs__panel' value={active}>
          {primaryContent === undefined ? null : <div hidden={active !== primary}>{primaryContent}</div>}
          <div className='schema-dialog-tabs__form' hidden={active === 'json'}>{children}</div>
          {jsonPreview === undefined ? null : <div hidden={active !== 'json'}>{jsonPreview}</div>}
        </TabsContent>
      </Tabs>
    </SchemaDialogContext>
  );
}
