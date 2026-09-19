import type { RJSFSchema } from '@rjsf/utils';

import { useTranslation } from 'react-i18next';

import type { ReviewedSchemaResolution } from '@/schemas/resolve-reviewed-schema';

import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';

import type { CanonicalDraft } from './use-canonical-configuration';

import { SchemaSectionForm } from './schema-section-form';
import { schemaProperties, uiSchemaFromPanel } from './schema-ui';
import { ManagedCollectionsEditor } from './managed-collections-editor';

interface DynamicGeneralEditorProps {
  disabled?: boolean;
  draft: CanonicalDraft;
  linkedInbound?: string;
  resolution: ReviewedSchemaResolution;
  onChange: (change: (draft: CanonicalDraft) => CanonicalDraft) => void;
}

const sectionOrder = ['log', 'dns', 'inbounds', 'outbounds', 'route', 'endpoints', 'services', 'ntp', 'certificate', 'certificate_providers', 'http_clients', 'network_namespaces', 'experimental'];

function label(schema: RJSFSchema, language: string, fallback: string): string {
  const metadata = schema['x-panel'];
  if (metadata !== null && typeof metadata === 'object' && !Array.isArray(metadata)) {
    const labels = (metadata as { label?: Record<string, string> }).label;
    return labels?.[language] ?? labels?.en ?? fallback;
  }
  return fallback;
}

export function DynamicGeneralEditor({
  disabled = false, draft, linkedInbound, onChange, resolution,
}: DynamicGeneralEditorProps) {
  const { i18n, t } = useTranslation();
  const sections = Object.entries(schemaProperties(resolution.schema, resolution.schema))
    .filter(([name]) => name !== '$schema')
    .sort(([left], [right]) => {
      const position = (name: string) => {
        const index = sectionOrder.indexOf(name);
        return index < 0 ? sectionOrder.length : index;
      };
      return position(left) - position(right);
    });
  if (sections.length === 0) return null;
  return (
    <Tabs className='configuration-general' defaultValue={linkedInbound ? 'inbounds' : sections[0][0]} orientation='vertical'>
      <TabsList aria-label={t('configuration.general.modules')} className='configuration-general__nav'>
        {sections.map(([name, schema]) => (
          <TabsTrigger key={name} value={name}>{label(schema, i18n.language, t(`configuration.general.labels.${name}`, { defaultValue: name }))}</TabsTrigger>
        ))}
      </TabsList>
      {sections.map(([name, schema]) => (
        <TabsContent className='configuration-section-scroll' key={name} value={name}>
          {name === 'endpoints' || name === 'inbounds' || name === 'outbounds' || name === 'services'
            ? (
                <ManagedCollectionsEditor
                  disabled={disabled} draft={draft} onChange={onChange}
                  linkedTag={name === 'inbounds' ? linkedInbound : undefined}
                  resolution={resolution} selectedCollection={name}
                />
              )
            : (
                <SchemaSectionForm
                  basePointer={`/${name}`} data={draft[name]} disabled={disabled} onChange={onChange}
                  resolution={resolution} schema={schema}
                  uiSchema={{ ...uiSchemaFromPanel(schema, [], resolution.schema, draft[name]), 'ui:title': '', 'ui:description': '' }}
                />
              )}
        </TabsContent>
      ))}
    </Tabs>
  );
}
