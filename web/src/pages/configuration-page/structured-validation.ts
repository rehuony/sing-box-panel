import type { RJSFSchema } from '@rjsf/utils';

import { projectSchemaKnownData } from './schema-ui';

export function visibleStructuredConfiguration(
  schema: RJSFSchema,
  configuration: unknown,
): Record<string, unknown> {
  if (configuration === null || typeof configuration !== 'object' || Array.isArray(configuration)) return {};
  return projectSchemaKnownData(schema, schema, configuration) as Record<string, unknown>;
}
