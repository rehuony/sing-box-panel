import type { RJSFSchema } from '@rjsf/utils';

import { useState } from 'react';
import { getSchemaType } from '@rjsf/utils';
import { useTranslation } from 'react-i18next';

import type { ReviewedSchemaResolution } from '@/schemas/resolve-reviewed-schema';

import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';

import type { CanonicalDraft } from './use-canonical-configuration';

import { SchemaSectionForm } from './schema-section-form';
import { resolvedSchema, schemaProperties, uiSchemaFromPanel } from './schema-ui';

interface ConfigurationSectionEditorProps {
  name: string;
  disabled: boolean;
  schema: RJSFSchema;
  draft: CanonicalDraft;
  resolution: ReviewedSchemaResolution;
  onChange: (change: (draft: CanonicalDraft) => CanonicalDraft) => void;
}

const experimentalGroupOrder = ['clash_api', 'v2ray_api', 'cache_file', 'debug'];

/** Group native fields for navigation without changing the stored schema or data. */
export function ConfigurationSectionEditor({
  name, schema, draft, disabled, resolution, onChange,
}: ConfigurationSectionEditorProps) {
  const { t } = useTranslation();
  const [actionContainer, setActionContainer] = useState<HTMLDivElement | null>(null);
  const properties = schemaProperties(schema, resolution.schema);
  const grouped = name === 'dns'
    ? ['servers', 'rules']
    : name === 'route'
      ? ['rules', 'rule_set']
      : name === 'experimental'
        ? [...experimentalGroupOrder, ...Object.keys(properties).filter(key => !experimentalGroupOrder.includes(key))]
        : [];
  const groups = grouped.filter((key) => properties[key] !== undefined);
  const remaining = Object.fromEntries(Object.entries(properties).filter(([key]) => !groups.includes(key)));
  const record = draft[name] !== null && typeof draft[name] === 'object' && !Array.isArray(draft[name]) ? draft[name] as Record<string, unknown> : {};

  function form(value: RJSFSchema, data: unknown, pointer: string, arrayLayout: 'default' | 'standalone' = 'default') {
    return <SchemaSectionForm arrayActionContainer={actionContainer} arrayLayout={arrayLayout} basePointer={pointer} data={data} disabled={disabled} onChange={onChange} resolution={resolution} schema={value} uiSchema={{ ...uiSchemaFromPanel(value, [], resolution.schema, data), 'ui:title': '', 'ui:description': '' }} />;
  }
  if (groups.length === 0) {
    const arrayLayout = getSchemaType(resolvedSchema(schema, resolution.schema)) === 'array' ? 'standalone' : 'default';
    return form(schema, draft[name], `/${name}`, arrayLayout);
  }
  const settingsSchema = {
    ...resolvedSchema(schema, resolution.schema),
    properties: remaining,
    required: schema.required?.filter((key) => Object.hasOwn(remaining, key)),
  };
  return (
    <Tabs className='configuration-section-tabs' defaultValue={groups[0]}>
      <div className='configuration-section-toolbar'>
        <TabsList aria-label={t(`configuration.general.labels.${name}`, { defaultValue: name })} className='configuration-section-tabs__nav' variant='line'>
          {groups.map((key) => <TabsTrigger key={key} value={key}>{t(name === 'dns' && key === 'rules' ? 'configuration.general.dnsRules' : `configuration.fields.${key}`, { defaultValue: key })}</TabsTrigger>)}
          {Object.keys(remaining).length > 0 ? <TabsTrigger value='settings'>{t(`configuration.general.${name === 'dns' ? 'dnsSettings' : 'routeSettings'}`)}</TabsTrigger> : null}
        </TabsList>
        <div className='configuration-section-tabs__actions' ref={setActionContainer} />
      </div>
      {groups.map((key) => <TabsContent key={key} value={key}>{form(properties[key], record[key], `/${name}/${key}`)}</TabsContent>)}
      {Object.keys(remaining).length > 0 ? <TabsContent value='settings'>{form(settingsSchema, record, `/${name}`)}</TabsContent> : null}
    </Tabs>
  );
}
