import type { ReactNode } from 'react';
import type { RJSFSchema } from '@rjsf/utils';

import { useTranslation } from 'react-i18next';

import { InfoTooltip } from '@/components/info-tooltip';

import { readConfigurationFieldHelp } from '../configuration-field-help';

interface SchemaFieldHelpProps {
  label: string;
  schema: RJSFSchema;
  description?: ReactNode;
}

export function SchemaFieldHelp({ schema, label, description }: SchemaFieldHelpProps) {
  const { i18n, t } = useTranslation();
  const help = readConfigurationFieldHelp(schema);
  const content = i18n.language.startsWith('zh') && help ? help.description : description ?? schema.description;
  if (!content) return null;
  return (
    <InfoTooltip label={t('configuration.general.fieldHelp', { field: label })}>
      {content}
    </InfoTooltip>
  );
}
