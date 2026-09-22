import { describe, expect, it } from 'vitest';

import type { ConfigurationSchemaContract } from '@/api/api-client';

import { reviewedSchemaManifest } from '@/schemas/generated';
import { resolveReviewedSchema } from '@/schemas/resolve-reviewed-schema';

const unavailableSHA256 = '0'.repeat(64);

async function contract(
  exactVersion: string,
  overrides: Partial<ConfigurationSchemaContract> = {},
): Promise<ConfigurationSchemaContract> {
  const entry = await reviewedSchemaManifest[exactVersion].load();
  return {
    exact_version: exactVersion,
    schema_sha256: entry.schemaSHA256,
    schema: entry.schema,
    ...overrides,
  };
}

describe.each(['1.14.0', '1.14.1'])('resolveReviewedSchema %s', (exactVersion) => {
  it('fails closed for a version without a reviewed manifest entry', async () => {
    await expect(resolveReviewedSchema(await contract(exactVersion, { exact_version: '1.14.2' }), '1.14.2'))
      .rejects
      .toThrow('reviewed browser manifest');
  });

  it('loads the precompiled contract for the exact version', async () => {
    const expected = await reviewedSchemaManifest[exactVersion].load();
    const resolution = await resolveReviewedSchema(await contract(exactVersion), exactVersion);

    expect(resolution.schema).toEqual(expected.schema);
    expect(resolution.createValidator(resolution.schema)).toBeDefined();
  }, 30_000);

  it('fails closed for an unreviewed digest', async () => {
    await expect(resolveReviewedSchema(
      await contract(exactVersion, { schema_sha256: unavailableSHA256 }),
      exactVersion,
    )).rejects.toThrow('reviewed browser manifest');
  });

  it('rejects an altered schema even with the reviewed digest', async () => {
    const exact = await contract(exactVersion);
    await expect(resolveReviewedSchema({
      ...exact,
      schema: { ...exact.schema, title: 'altered at runtime' },
    }, exactVersion)).rejects.toThrow('differs from the reviewed browser Schema');
  });

  it('rejects a response for another exact version', async () => {
    await expect(resolveReviewedSchema(
      await contract(exactVersion, { exact_version: exactVersion === '1.14.0' ? '1.14.1' : '1.14.0' }),
      exactVersion,
    )).rejects.toThrow('reviewed browser manifest');
  });
});
