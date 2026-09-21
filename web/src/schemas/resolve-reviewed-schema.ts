import type { RJSFSchema, ValidatorType } from '@rjsf/utils';

import { createPrecompiledValidator } from '@rjsf/validator-ajv8';

import type { ConfigurationSchemaContract } from '@/api/api-client';

import i18n from '@/i18n';

import { loadReviewedSchema, reviewedSchemaManifest } from './generated';

export interface ReviewedSchemaResolution {
  schema: RJSFSchema;
  createValidator: (schema: RJSFSchema) => ValidatorType;
}

export async function resolveReviewedSchema(
  contract: ConfigurationSchemaContract,
  exactVersion: string,
): Promise<ReviewedSchemaResolution> {
  const manifestEntry = reviewedSchemaManifest[exactVersion];
  if (
    contract.exact_version !== exactVersion
    || manifestEntry === undefined
    || manifestEntry.schemaSHA256 !== contract.schema_sha256
  ) {
    throw new Error(i18n.t('configuration.schema.error.notReviewed'));
  }
  const reviewed = await loadReviewedSchema(exactVersion);
  if (reviewed === undefined) {
    throw new Error(i18n.t('configuration.schema.error.notReviewed'));
  }
  if (
    reviewed.exactVersion !== exactVersion
    || reviewed.schemaSHA256 !== contract.schema_sha256
  ) {
    throw new Error(i18n.t('configuration.schema.error.notReviewed'));
  }
  if (JSON.stringify(reviewed.schema) !== JSON.stringify(contract.schema)) {
    throw new Error(i18n.t('configuration.schema.error.contract'));
  }
  return {
    schema: reviewed.schema,
    createValidator(schema) {
      return createPrecompiledValidator(reviewed.validateFns as never, schema);
    },
  };
}

// Authoring without an installed core uses only the same reviewed, build-time
// schema and precompiled validators. Native validation still requires a core.
export async function resolveBundledReviewedSchema(exactVersion: string): Promise<ReviewedSchemaResolution> {
  const reviewed = await loadReviewedSchema(exactVersion);
  if (!reviewed) throw new Error(i18n.t('configuration.schema.error.notReviewed'));
  return {
    schema: reviewed.schema,
    createValidator(schema) {
      return createPrecompiledValidator(reviewed.validateFns as never, schema);
    },
  };
}
