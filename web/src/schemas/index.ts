import type { RJSFSchema } from '@rjsf/utils';

import manifest from './.generated/manifest.json';

type ReviewedValidatorFunction = ((data: unknown) => boolean) & { errors?: unknown };
type ReviewedValidatorFunctions = Record<string, ReviewedValidatorFunction>;

export interface ReviewedSchemaEntry {
  schema: RJSFSchema;
  exactVersion: string;
  schemaSHA256: string;
  validateFns: ReviewedValidatorFunctions;
}

export interface ReviewedSchemaManifestEntry {
  exactVersion: string;
  schemaSHA256: string;
  load: () => Promise<ReviewedSchemaEntry>;
}

const schemas = import.meta.glob<RJSFSchema>('./.generated/schema-*.json', { import: 'default' });
const validators = import.meta.glob<ReviewedValidatorFunctions>('./.generated/validator-*.ts', { import: 'default' });

export const reviewedSchemaManifest: Readonly<Record<string, ReviewedSchemaManifestEntry>> = Object.fromEntries(
  manifest.entries.map(entry => {
    const loadSchema = schemas[`./.generated/${entry.schema_file}`];
    const loadValidator = validators[`./.generated/validator-${entry.schema_sha256}.ts`];
    if (!loadSchema || !loadValidator) {
      throw new Error(`configuration schema ${entry.exact_version} is missing generated browser assets`);
    }
    return [entry.exact_version, {
      exactVersion: entry.exact_version,
      schemaSHA256: entry.schema_sha256,
      async load() {
        const [schema, validateFns] = await Promise.all([loadSchema(), loadValidator()]);
        return { exactVersion: entry.exact_version, schemaSHA256: entry.schema_sha256, schema, validateFns };
      },
    }];
  }),
);

export function hasReviewedSchemaVersion(exactVersion: string): boolean {
  return Object.hasOwn(reviewedSchemaManifest, exactVersion);
}

export async function loadReviewedSchema(exactVersion: string): Promise<ReviewedSchemaEntry | undefined> {
  return hasReviewedSchemaVersion(exactVersion) ? reviewedSchemaManifest[exactVersion].load() : undefined;
}
