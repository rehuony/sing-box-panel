import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { createHash } from 'node:crypto';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { mkdir, mkdtemp, readdir, readFile, rm, stat, writeFile } from 'node:fs/promises';

import { deferred } from '@/tests/deferred';

import { generateConfigurationSchemaArtifacts } from '../../../scripts/configuration-schema';

const { exportAssets, afterRename } = vi.hoisted(() => ({
  exportAssets: vi.fn<(outputRoot: string) => Promise<void>>(),
  afterRename: vi.fn<(destination: string) => Promise<void>>(),
}));

vi.mock('node:child_process', () => ({
  execFile(_command: string, args: string[], _options: unknown, callback: (error: unknown, stdout?: string) => void) {
    exportAssets(args.at(-1)!).then(() => callback(null, ''), error => callback(error));
  },
}));

vi.mock('node:fs/promises', async importOriginal => {
  const actual = await importOriginal<typeof import('node:fs/promises')>();
  return {
    ...actual,
    async rename(source: string, destination: string) {
      await actual.rename(source, destination);
      await afterRename(destination);
    },
  };
});

let temporaryRoot: string;
let webRoot: string;
let outputRoot: string;
let schema: Record<string, unknown>;

beforeEach(async () => {
  temporaryRoot = await mkdtemp(join(tmpdir(), 'configuration-schema-generation-'));
  webRoot = join(temporaryRoot, 'web');
  outputRoot = join(webRoot, 'src', 'schemas', '.generated');
  schema = { $id: 'generationFixture', type: 'object' };
  afterRename.mockResolvedValue(undefined);
  exportAssets.mockImplementation(async root => {
    const raw = JSON.stringify(schema);
    const digest = createHash('sha256').update(raw).digest('hex');
    const schemaFile = `schema-${digest}.json`;
    await writeFile(join(root, schemaFile), raw);
    await writeFile(join(root, 'manifest.json'), JSON.stringify({
      schema_version: 1,
      entries: ['1.14.2', '1.14.3'].map(exactVersion => ({
        exact_version: exactVersion,
        schema_file: schemaFile,
        schema_sha256: digest,
        source: 'native',
      })),
    }));
  });
});

afterEach(async () => {
  await rm(temporaryRoot, { force: true, recursive: true });
});

async function concurrentGeneration() {
  const results = await Promise.allSettled(
    Array.from({ length: 4 }, () => generateConfigurationSchemaArtifacts(webRoot)),
  );
  expect(results.filter(result => result.status === 'rejected')).toEqual([]);
}

async function expectCurrentFileSet() {
  const digest = createHash('sha256').update(JSON.stringify(schema)).digest('hex');
  expect((await readdir(outputRoot)).sort()).toEqual([
    'manifest.json', `schema-${digest}.json`, `validator-${digest}.ts`,
  ]);
  await generateConfigurationSchemaArtifacts(webRoot, { check: true });
}

async function outputSnapshot() {
  return Promise.all((await readdir(outputRoot)).sort().map(async name => ({
    name,
    content: await readFile(join(outputRoot, name), 'utf8'),
    modified: (await stat(join(outputRoot, name))).mtimeMs,
  })));
}

it('allows concurrent generators to publish a complete cold-start output', async () => {
  await concurrentGeneration();
  await expectCurrentFileSet();
  const manifest = JSON.parse(await readFile(join(outputRoot, 'manifest.json'), 'utf8'));
  expect(manifest.entries.map((entry: { exact_version: string }) => entry.exact_version)).toEqual(['1.14.2', '1.14.3']);
});

it('keeps the output directory available during concurrent updates and removes obsolete assets', async () => {
  await generateConfigurationSchemaArtifacts(webRoot);
  const originalDirectory = await stat(outputRoot);
  await mkdir(join(outputRoot, 'obsolete-directory'));
  await writeFile(join(outputRoot, 'obsolete.ts'), 'obsolete');
  schema.properties = { enabled: { type: 'boolean' } };

  await concurrentGeneration();

  expect((await stat(outputRoot)).ino).toBe(originalDirectory.ino);
  await expectCurrentFileSet();
});

it('republishes complete content after a different input is published mid-generation', async () => {
  const originalSchema = schema;
  const digest = createHash('sha256').update(JSON.stringify(originalSchema)).digest('hex');
  const paused = deferred<void>();
  const resume = deferred<void>();
  let didPause = false;
  afterRename.mockImplementation(async destination => {
    if (!didPause && destination === join(outputRoot, `validator-${digest}.ts`)) {
      didPause = true;
      paused.resolve();
      await resume.promise;
    }
  });
  const originalGeneration = generateConfigurationSchemaArtifacts(webRoot);
  try {
    await Promise.race([
      paused.promise,
      originalGeneration.then(() => {
        throw new Error('generation did not reach the publication barrier');
      }),
    ]);
    schema = { ...originalSchema, properties: { enabled: { type: 'boolean' } } };
    await generateConfigurationSchemaArtifacts(webRoot);
    await expectCurrentFileSet();
    resume.resolve();
    await originalGeneration;
    schema = originalSchema;
    await expectCurrentFileSet();
  } finally {
    resume.resolve();
    await Promise.allSettled([originalGeneration]);
  }
});

it('reports persistent publication conflicts after three complete attempts', async () => {
  const digest = createHash('sha256').update(JSON.stringify(schema)).digest('hex');
  let publications = 0;
  afterRename.mockImplementation(async destination => {
    if (destination === join(outputRoot, 'manifest.json')) {
      publications++;
      await rm(join(outputRoot, `validator-${digest}.ts`));
    }
  });
  await expect(generateConfigurationSchemaArtifacts(webRoot)).rejects.toThrow('concurrent configuration schema generation conflict');
  expect(publications).toBe(3);
});

it('does not rewrite unchanged output and checks both current and stale output without writes', async () => {
  await generateConfigurationSchemaArtifacts(webRoot);
  const original = await outputSnapshot();
  await generateConfigurationSchemaArtifacts(webRoot);
  await generateConfigurationSchemaArtifacts(webRoot, { check: true });
  expect(await outputSnapshot()).toEqual(original);

  await writeFile(join(outputRoot, 'manifest.json'), 'stale');
  const stale = await outputSnapshot();
  await expect(generateConfigurationSchemaArtifacts(webRoot, { check: true })).rejects.toThrow('artifacts are stale');
  expect(await outputSnapshot()).toEqual(stale);
});

it('reports missing output in check mode without creating it', async () => {
  await expect(generateConfigurationSchemaArtifacts(webRoot, { check: true })).rejects.toThrow('artifacts are stale');
  await expect(stat(outputRoot)).rejects.toMatchObject({ code: 'ENOENT' });
});

it('preserves existing output when the backend export fails', async () => {
  await generateConfigurationSchemaArtifacts(webRoot);
  const original = await outputSnapshot();
  exportAssets.mockRejectedValue(new Error('fixture export failed'));

  await expect(generateConfigurationSchemaArtifacts(webRoot)).rejects.toThrow('export configuration schema');
  expect(await outputSnapshot()).toEqual(original);
});
