import type { RJSFSchema } from '@rjsf/utils';

import { useTranslation } from 'react-i18next';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@shared/components/ui/tabs';

import type { ReviewedSchemaResolution } from '@/schemas/resolve-reviewed-schema';

import { useHashTab } from '@/hooks/use-hash-tab';

import type { CanonicalDraft } from './use-canonical-configuration';

import { schemaProperties } from './schema-ui';
import { ManagedCollectionsEditor } from './managed-collections-editor';
import { ConfigurationSectionEditor } from './configuration-section-editor';

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
  const [section, setSection] = useHashTab('configuration-visual/', sections.map(([name]) => name), linkedInbound ? 'inbounds' : sections[0]?.[0] ?? 'log');
  if (sections.length === 0) return null;
  return (
    <Tabs className='grid h-full min-h-0 grid-cols-[208px_minmax(0,1fr)] items-start gap-6 @max-[40rem]:flex @max-[40rem]:flex-col @max-[40rem]:gap-4' data-configuration-general value={section} onValueChange={setSection} orientation='vertical'>
      <TabsList aria-label={t('configuration.general.modules')} className='h-full w-full justify-start gap-1 overflow-auto rounded-none border-r bg-transparent pt-2 pr-4 pb-0 pl-0 @max-[40rem]:h-auto @max-[40rem]:shrink-0 @max-[40rem]:flex-row @max-[40rem]:border-r-0 *:data-[slot=tabs-trigger]:min-h-10 *:data-[slot=tabs-trigger]:w-full *:data-[slot=tabs-trigger]:flex-none *:data-[slot=tabs-trigger]:justify-start *:data-[slot=tabs-trigger]:gap-3 *:data-[slot=tabs-trigger]:font-normal *:data-[slot=tabs-trigger]:after:hidden *:data-[slot=tabs-trigger]:data-active:bg-accent *:data-[slot=tabs-trigger]:data-active:text-primary @max-[40rem]:*:data-[slot=tabs-trigger]:w-auto'>
        {sections.map(([name, schema]) => (
          <TabsTrigger className='data-section-start:mt-3 data-section-start:before:absolute data-section-start:before:inset-x-2 data-section-start:before:-top-[7px] data-section-start:before:border-t data-section-start:before:content-[""] @max-[40rem]:data-section-start:mt-0 @max-[40rem]:data-section-start:before:hidden' data-section-start={name === 'ntp' || name === 'experimental' || undefined} key={name} value={name}>{label(schema, i18n.language, t(`configuration.general.labels.${name}`, { defaultValue: name }))}</TabsTrigger>
        ))}
      </TabsList>
      {sections.map(([name, schema]) => (
        <TabsContent className='h-full min-h-0 overflow-auto overscroll-contain pr-(--configuration-inset) [container:configuration-content/inline-size] @max-[40rem]:h-auto @max-[40rem]:w-full @max-[40rem]:flex-1' key={name} value={name}>
          {name === 'endpoints' || name === 'inbounds' || name === 'outbounds' || name === 'services'
            ? (
                <ManagedCollectionsEditor
                  disabled={disabled} draft={draft} onChange={onChange}
                  linkedTag={name === 'inbounds' ? linkedInbound : undefined}
                  resolution={resolution} selectedCollection={name}
                />
              )
            : (
                <ConfigurationSectionEditor
                  name={name} draft={draft} disabled={disabled} onChange={onChange}
                  resolution={resolution} schema={schema}
                />
              )}
        </TabsContent>
      ))}
    </Tabs>
  );
}
