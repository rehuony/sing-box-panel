import type { RJSFSchema } from '@rjsf/utils';

import type { ConfigurationSchemaContract } from '@/api/api-client';

const files = import.meta.glob<RJSFSchema>('./configuration-schema-*.json', { eager: true, import: 'default' });

/**
 * Configuration schemas as the panel serves them: the reviewed 1.13 schema,
 * which a Go test keeps equal to the panel's, and one a 1.14 core printed.
 */
export const configurationSchemas: Readonly<Record<'1.13' | '1.14', RJSFSchema>> = {
  1.13: files['./configuration-schema-1.13.json'],
  1.14: files['./configuration-schema-1.14.json'],
};

/** The schema of the line exactVersion belongs to. */
export function configurationSchemaFor(exactVersion: string): RJSFSchema {
  return exactVersion.startsWith('1.13.') ? configurationSchemas['1.13'] : configurationSchemas['1.14'];
}

/** The configuration-schema response for exactVersion. */
export function configurationSchemaContract(exactVersion: string): ConfigurationSchemaContract {
  return { exact_version: exactVersion, schema_sha256: '0'.repeat(64), schema: configurationSchemaFor(exactVersion) as ConfigurationSchemaContract['schema'] };
}
