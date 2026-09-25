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
  pendingContract: ConfigurationSchemaContract | Promise<ConfigurationSchemaContract>,
  exactVersion: string,
): Promise<ReviewedSchemaResolution> {
  const manifestEntry = reviewedSchemaManifest[exactVersion];
  // Start the reviewed browser modules while the authenticated contract is in flight.
  const [contract, reviewed] = await Promise.all([
    pendingContract,
    loadReviewedSchema(exactVersion),
  ]);
  if (
    contract.exact_version !== exactVersion
    || manifestEntry === undefined
    || manifestEntry.schemaSHA256 !== contract.schema_sha256
  ) {
    throw new Error(i18n.t('configuration.schema.error.notReviewed'));
  }
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
