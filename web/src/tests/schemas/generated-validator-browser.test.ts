import type { InlineConfig } from 'vite';

import { build } from 'vite';
import { dirname, join } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { mkdtemp, readdir, rm, writeFile } from 'node:fs/promises';
import { afterEach, beforeAll, describe, expect, it } from 'vitest';

import { reviewedSchemaManifest } from '@/schemas';
import manifest from '@/schemas/.generated/manifest.json';

import {
  compileConfigurationSchemaValidator,
} from '../../../scripts/configuration-schema';

const temporaryDirectories: string[] = [];

beforeAll(async () => {
  await Promise.all(Object.values(reviewedSchemaManifest).map(entry => entry.load()));
});

afterEach(async () => {
  await Promise.all(temporaryDirectories.splice(0).map((directory) => rm(directory, { force: true, recursive: true })));
});

describe('generated configuration validators', () => {
  it('shares one schema and validator per digest without merging exact versions', async () => {
    const digests = [...new Set(manifest.entries.map(entry => entry.schema_sha256))];
    const files = await readdir(new URL('../../schemas/.generated/', import.meta.url));
    expect(files.sort()).toEqual([
      'manifest.json',
      ...digests.flatMap(digest => [`schema-${digest}.json`, `validator-${digest}.ts`]),
    ].sort());

    for (const digest of digests) {
      const entries = manifest.entries.filter(entry => entry.schema_sha256 === digest);
      const shared = await reviewedSchemaManifest[entries[0].exact_version].load();
      for (const entry of entries) {
        const loaded = await reviewedSchemaManifest[entry.exact_version].load();
        expect(loaded.exactVersion).toBe(entry.exact_version);
        expect(loaded.schemaSHA256).toBe(digest);
        expect(loaded.schema).toBe(shared.schema);
        expect(loaded.validateFns).toBe(shared.validateFns);
      }
    }
  });

  it('executes Ajv runtime helpers after Vite bundles the generated ESM module', async () => {
    const testRoot = await mkdtemp(join(dirname(fileURLToPath(import.meta.url)), '.validator-runtime-'));
    temporaryDirectories.push(testRoot);
    const validatorSource = compileConfigurationSchemaValidator({
      $id: 'browserRuntimeFixture',
      type: 'object',
      properties: {
        items: { type: 'array', uniqueItems: true },
        name: { type: 'string', minLength: 2 },
      },
      required: ['items', 'name'],
    }, 'browser-runtime-fixture', 'validator-runtime-fixture.ts');

    const entry = join(testRoot, 'validator.mjs');
    const outputDirectory = join(testRoot, 'dist');
    await writeFile(entry, validatorSource, 'utf8');
    await build({
      configFile: false,
      logLevel: 'silent',
      root: testRoot,
      build: {
        emptyOutDir: true,
        lib: { entry, fileName: 'validator', formats: ['es'] },
        outDir: outputDirectory,
      },
    } satisfies InlineConfig);

    const bundledFile = join(outputDirectory, 'validator.js');
    const validatorModule = await import(`${pathToFileURL(bundledFile).href}?test=${Date.now()}`) as {
      default: Record<string, ((value: unknown) => boolean) & { errors?: unknown }>;
    };
    const validate = validatorModule.default.browserRuntimeFixture;
    expect(validate({ items: [1, 2], name: '🚀a' })).toBe(true);
    expect(validate({ items: [1, 1], name: 'a' })).toBe(false);
    const errors = validate.errors as Record<string, unknown>[] | undefined;
    expect(errors).toHaveLength(2);
    expect(errors?.every((error) => {
      const record = error as Record<string, unknown>;
      return record.schema === undefined && record.parentSchema === undefined && record.data === undefined;
    })).toBe(true);
  });

  it.each(Object.keys(reviewedSchemaManifest))('executes the exact %s validator without panel-only fields', async (exactVersion) => {
    const entry = reviewedSchemaManifest[exactVersion];
    const loaded = await entry.load();
    const validate = loaded.validateFns[loaded.schema.$id!];
    expect(Object.keys(loaded.validateFns)).toEqual([loaded.schema.$id]);
    expect(validate).toBeTypeOf('function');
    expect(validate({})).toBe(true);
    expect(validate({ _panel: { id: 'inbound-1', enabled: true } })).toBe(false);
  });
});
