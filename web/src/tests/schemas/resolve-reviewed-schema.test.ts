import { describe, expect, it, vi } from 'vitest';

import type { ConfigurationSchemaContract } from '@/api/api-client';

import { reviewedSchemaManifest } from '@/schemas/generated';
import { resolveReviewedSchema } from '@/schemas/resolve-reviewed-schema';

import { representativeSchemaVersions } from './schema-fixtures';

const unavailableSHA256 = '0'.repeat(64);

it('loads browser modules before the remote schema contract completes', async () => {
  const exactVersion = '1.14.1';
  const expected = await contract(exactVersion);
  const load = vi.spyOn(reviewedSchemaManifest[exactVersion], 'load');
  try {
    let finish!: (value: ConfigurationSchemaContract) => void;
    const pending = new Promise<ConfigurationSchemaContract>(resolve => {
      finish = resolve;
    });
    const resolution = resolveReviewedSchema(pending, exactVersion);
    expect(load).toHaveBeenCalledOnce();
    finish(expected);
    expect((await resolution).schema).toEqual(expected.schema);
  } finally {
    load.mockRestore();
  }
});

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

it.each(Object.keys(reviewedSchemaManifest))('loads the precompiled contract for exact version %s', async (exactVersion) => {
  const expected = await reviewedSchemaManifest[exactVersion].load();
  const resolution = await resolveReviewedSchema(await contract(exactVersion), exactVersion);

  expect(resolution.schema).toEqual(expected.schema);
  expect(resolution.createValidator(resolution.schema)).toBeDefined();
}, 30_000);

describe.each(representativeSchemaVersions())('resolveReviewedSchema %s', (exactVersion) => {
  it('fails closed for a version without a reviewed manifest entry', async () => {
    await expect(resolveReviewedSchema(await contract(exactVersion, { exact_version: '99.0.0' }), '99.0.0'))
      .rejects
      .toThrow('reviewed browser manifest');
  });

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
