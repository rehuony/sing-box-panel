import type { InlineConfig } from 'vite';

import { build } from 'vite';
import { dirname, join } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { afterEach, describe, expect, it } from 'vitest';
import { mkdtemp, rm, writeFile } from 'node:fs/promises';

import { reviewedSchemaManifest } from '@/schemas/generated';

import {
  compileConfigurationSchemaValidator,
  generateConfigurationSchemaArtifacts,
  parseConfigurationSchemaManifest,
} from '../../../plugins/configuration-schema';

const temporaryDirectories: string[] = [];

afterEach(async () => {
  await Promise.all(temporaryDirectories.splice(0).map((directory) => rm(directory, { force: true, recursive: true })));
});

describe('generated configuration validators', () => {
  it('accepts the minimal exact-version manifest', () => {
    const manifest = parseConfigurationSchemaManifest(JSON.stringify({
      schema_version: 1,
      entries: [{
        exact_version: '1.14.0',
        source: 'native',
        schema_file: 'schema.json',
        schema_sha256: 'b'.repeat(64),
      }],
    }));
    expect(manifest.entries).toEqual([{
      exact_version: '1.14.0',
      source: 'native',
      schema_file: 'schema.json',
      schema_sha256: 'b'.repeat(64),
    }]);
  });

  it('rejects a manifest entry missing a version-owned schema digest', () => {
    const manifest = {
      schema_version: 1,
      entries: [{
        exact_version: '1.14.0',
        source: 'native',
        schema_file: 'schema.json',
        schema_sha256: 'b'.repeat(64),
      }],
    };
    delete (manifest.entries[0] as Partial<typeof manifest.entries[0]>).schema_sha256;
    expect(() => parseConfigurationSchemaManifest(JSON.stringify(manifest))).toThrow('manifest entry');
  });

  it('re-exports committed schema assets through the offline support-tool boundary', async () => {
    const webRoot = join(dirname(fileURLToPath(import.meta.url)), '..', '..', '..');
    await expect(generateConfigurationSchemaArtifacts(webRoot, { check: true })).resolves.toBeUndefined();
  }, 20_000);

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
    expect(validatorSource).toContain('import runtimeModule0 from');
    expect(validatorSource).not.toMatch(/runtime\d+\.default/);
    expect(validatorSource).not.toMatch(/\b(?:exports|require)\b/);

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
  }, 20_000);

  it.each(['1.14.0', '1.14.1', '1.14.2'])('executes the exact %s validator without panel-only fields', async (exactVersion) => {
    const entry = reviewedSchemaManifest[exactVersion];
    const loaded = await entry?.load();
    const validate = loaded?.validateFns['https://sing-box.sagernet.org/schema.json'];
    expect(Object.keys(loaded?.validateFns ?? {})).toEqual(['https://sing-box.sagernet.org/schema.json']);
    expect(validate).toBeTypeOf('function');
    expect(validate?.({})).toBe(true);
    expect(validate?.({ _panel: { id: 'inbound-1', enabled: true } })).toBe(false);
  }, 20_000);
});
