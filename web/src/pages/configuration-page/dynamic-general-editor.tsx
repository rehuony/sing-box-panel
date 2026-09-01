import type { RJSFSchema } from '@rjsf/utils';

import { useTranslation } from 'react-i18next';

import type { ReviewedSchemaResolution } from '@/schemas/resolve-reviewed-schema';

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';

import type { CanonicalDraft } from './use-canonical-configuration';

import { SchemaSectionForm } from './schema-section-form';
import { panelMetadata, schemaProperties, uiSchemaFromPanel } from './schema-ui';

interface DynamicGeneralEditorProps {
  disabled?: boolean;
  draft: CanonicalDraft;
  resolution: ReviewedSchemaResolution;
  onChange: (change: (draft: CanonicalDraft) => CanonicalDraft) => void;
}

function label(schema: RJSFSchema, language: string, fallback: string): string {
  const metadata = schema['x-panel'];
  if (metadata !== null && typeof metadata === 'object' && !Array.isArray(metadata)) {
    const labels = (metadata as { label?: Record<string, string> }).label;
    return labels?.[language] ?? labels?.en ?? fallback;
  }
  return fallback;
}

export function DynamicGeneralEditor({
  disabled = false, draft, onChange, resolution,
}: DynamicGeneralEditorProps) {
  const { i18n, t } = useTranslation();
  const sections = Object.entries(schemaProperties(resolution.schema, resolution.schema))
    .filter(([, schema]) => panelMetadata(schema).section === 'general')
    .sort(([, left], [, right]) => (panelMetadata(left).order ?? 0) - (panelMetadata(right).order ?? 0));

  return (
    <div className='configuration-section-grid'>
      {sections.map(([name, schema]) => (
        <Card className='configuration-form-card' key={name}>
          <CardHeader>
            <CardTitle>{label(schema, i18n.language, name)}</CardTitle>
          </CardHeader>
          <CardContent>
            <SchemaSectionForm
              basePointer={`/${name}`}
              data={draft[name]}
              disabled={disabled}
              onChange={onChange}
              resolution={resolution}
              schema={schema}
              uiSchema={uiSchemaFromPanel(
                schema,
                [],
                resolution.schema,
                draft[name],
              )}
            />
          </CardContent>
        </Card>
      ))}
      {sections.length === 0
        ? (
            <p className='configuration-empty-copy'>{t('configuration.schema.noGeneralFields')}</p>
          )
        : null}
    </div>
  );
}
