import { describe, expect, it } from 'vitest';

import type { ConfigurationSchemaContract } from '@/api/api-client';

import { reviewedSchemaManifest } from '@/schemas/generated';
import { resolveReviewedSchema } from '@/schemas/resolve-reviewed-schema';

const exactVersion = '1.14.0';
const reviewed = reviewedSchemaManifest[exactVersion];
const unavailableSHA256 = '0'.repeat(64);

async function contract(
  overrides: Partial<ConfigurationSchemaContract> = {},
): Promise<ConfigurationSchemaContract> {
  const entry = await reviewed?.load();
  return {
    exact_version: exactVersion,
    schema_sha256: entry?.schemaSHA256 ?? unavailableSHA256,
    schema: entry?.schema ?? {},
    ...overrides,
  };
}

describe('resolveReviewedSchema', () => {
  it('fails closed while the reviewed manifest has no exact version', async () => {
    if (reviewed !== undefined) return;
    await expect(resolveReviewedSchema(await contract(), exactVersion))
      .rejects
      .toThrow('reviewed browser manifest');
  });

  it.skipIf(reviewed === undefined)('loads the precompiled contract for the exact version', async () => {
    const expected = await reviewed?.load();
    const resolution = await resolveReviewedSchema(await contract(), exactVersion);

    expect(resolution.schema).toEqual(expected?.schema);
    expect(resolution.createValidator(resolution.schema)).toBeDefined();
  }, 30_000);

  it('fails closed for an unreviewed digest', async () => {
    if (reviewed === undefined) return;
    await expect(resolveReviewedSchema(
      await contract({ schema_sha256: unavailableSHA256 }),
      exactVersion,
    )).rejects.toThrow('reviewed browser manifest');
  });

  it.skipIf(reviewed === undefined)('rejects an altered schema even with the reviewed digest', async () => {
    const exact = await contract();
    await expect(resolveReviewedSchema({
      ...exact,
      schema: { ...exact.schema, title: 'altered at runtime' },
    }, exactVersion)).rejects.toThrow('differs from the reviewed browser Schema');
  });

  it('rejects a response for another exact version', async () => {
    await expect(resolveReviewedSchema(
      await contract({ exact_version: '1.14.1' }),
      exactVersion,
    )).rejects.toThrow('reviewed browser manifest');
  });
});
