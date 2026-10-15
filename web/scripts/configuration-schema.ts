// SPDX-License-Identifier: GPL-3.0-or-later

import { tmpdir } from 'node:os';
import process from 'node:process';
import { promisify } from 'node:util';
import Ajv2020 from 'ajv/dist/2020.js';
import { createHash } from 'node:crypto';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import standaloneCode from 'ajv/dist/standalone/index.js';
import { execFile as execFileCallback } from 'node:child_process';
import createAjvInstance from '@rjsf/validator-ajv8/lib/createAjvInstance.js';
import { chmod, mkdir, mkdtemp, readdir, readFile, rename, rm, writeFile } from 'node:fs/promises';

const execFile = promisify(execFileCallback);

interface SchemaManifestEntry {
  schema_file: string;
  exact_version: string;
  schema_sha256: string;
  source: 'native' | 'reviewed-1.13';
}

interface SchemaManifest {
  schema_version: number;
  entries: SchemaManifestEntry[];
}

export interface ConfigurationSchemaGenerationOptions {
  check?: boolean;
}

export async function generateConfigurationSchemaArtifacts(
  webRoot: string,
  options: ConfigurationSchemaGenerationOptions = {},
): Promise<void> {
  const repositoryRoot = resolve(webRoot, '..');
  const outputRoot = join(webRoot, 'src', 'schemas', '.generated');
  const sourceRoot = await mkdtemp(join(tmpdir(), 'sing-box-panel-schema-web-'));
  try {
    await exportConfigurationSchemaAssets(repositoryRoot, sourceRoot);
    const manifestSource = await readFile(join(sourceRoot, 'manifest.json'), 'utf8');
    const manifest = parseConfigurationSchemaManifest(manifestSource);
    const generated = new Map<string, string>();
    generated.set('manifest.json', `${JSON.stringify(manifest, null, 2)}\n`);

    for (const entry of manifest.entries) {
      const outputSchemaFile = entry.schema_file;
      const validatorFile = `validator-${entry.schema_sha256}.ts`;
      if (!generated.has(outputSchemaFile)) {
        const schemaSource = await readFile(join(sourceRoot, outputSchemaFile), 'utf8');
        if (sha256(schemaSource) !== entry.schema_sha256) {
          throw new Error(`schema ${entry.exact_version} differs from its manifest digest`);
        }
        const schema = JSON.parse(schemaSource) as Record<string, unknown>;
        generated.set(outputSchemaFile, schemaSource);
        generated.set(validatorFile, compileConfigurationSchemaValidator(
          schema,
          `Schema SHA-256: ${entry.schema_sha256}`,
          validatorFile,
        ));
      }
    }

    const differences = await compareGeneratedFiles(outputRoot, generated);
    if (options.check && differences.length > 0) {
      throw new Error(
        `configuration schema browser artifacts are stale (${differences.join(', ')}); run pnpm run generate`,
      );
    }
    if (!options.check && differences.length > 0) {
      await syncGeneratedFiles(outputRoot, generated);
    }
  } finally {
    await rm(sourceRoot, { force: true, recursive: true });
  }
}

async function exportConfigurationSchemaAssets(repositoryRoot: string, outputRoot: string): Promise<void> {
  try {
    await execFile(
      'go',
      ['tool', 'singbox-support', 'export-web', '--out', outputRoot],
      {
        cwd: repositoryRoot,
        env: { ...process.env, GOWORK: 'off' },
      },
    );
  } catch (error) {
    throw new Error('export configuration schema browser assets with singbox-support', { cause: error });
  }
}

export function parseConfigurationSchemaManifest(source: string): SchemaManifest {
  const value: unknown = JSON.parse(source);
  if (
    !isRecord(value)
    || value.schema_version !== 1
    || !Array.isArray(value.entries)
  ) {
    throw new Error('configuration schema manifest has an unsupported or incomplete header');
  }
  const versions = new Set<string>();
  const entries: SchemaManifestEntry[] = [];
  for (const entry of value.entries) {
    if (
      !isRecord(entry)
      || typeof entry.exact_version !== 'string' || !/^\d+\.\d+\.\d+$/.test(entry.exact_version)
      || !isSha256(entry.schema_sha256)
      || entry.schema_file !== `schema-${entry.schema_sha256}.json`
      || (entry.source !== 'native' && entry.source !== 'reviewed-1.13')
    ) {
      throw new Error(`configuration schema manifest entry for ${entry.exact_version ?? 'unknown'} is incomplete`);
    }
    if (versions.has(entry.exact_version)) {
      throw new Error(`configuration schema manifest contains a duplicate entry for ${entry.exact_version}`);
    }
    versions.add(entry.exact_version);
    entries.push({
      source: entry.source,
      exact_version: entry.exact_version,
      schema_sha256: entry.schema_sha256,
      schema_file: entry.schema_file,
    });
  }
  return { schema_version: 1, entries };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}

interface AjvRuntimeImport {
  specifier: string;
  importName: string;
  moduleName: string;
  importedName: string;
}

export function compileConfigurationSchemaValidator(
  schema: Record<string, unknown>,
  sourceReference: string,
  validatorFile: string,
): string {
  const runtimeImports = new Map<string, AjvRuntimeImport>();
  const code = compileRootSchemaValidator(schema)
    .replace(/^"use strict";\s*/, '')
    .replace(/\bexports\.([A-Za-z_$][\w$]*)\s*=/g, 'validators.$1 =')
    .replace(/\bexports(?=\s*\[)/g, 'validators')
    .replace(
      /require\((["'])([^"']+)\1\)\.([A-Za-z_$][\w$]*)/g,
      (_match, _quote: string, specifier: string, importedName: string) => {
        const key = `${specifier}\0${importedName}`;
        let runtimeImport = runtimeImports.get(key);
        if (runtimeImport === undefined) {
          runtimeImport = {
            importName: `runtime${runtimeImports.size}`,
            importedName,
            moduleName: `runtimeModule${runtimeImports.size}`,
            specifier,
          };
          runtimeImports.set(key, runtimeImport);
        }
        return runtimeImport.importName;
      },
    );
  if (/\brequire\s*\(/.test(code) || /\bexports\b/.test(code)) {
    throw new Error(`generated validator ${validatorFile} still contains CommonJS bindings`);
  }
  const imports = [...runtimeImports.values()]
    .map(({ importedName, importName, moduleName, specifier }) => importedName === 'default'
      ? `import ${moduleName} from ${JSON.stringify(specifier)};\nconst ${importName} = ${moduleName}.default ?? ${moduleName};`
      : `import * as ${moduleName} from ${JSON.stringify(specifier)};\nconst ${importName} = ${moduleName}[${JSON.stringify(importedName)}] ?? ${moduleName}.default?.[${JSON.stringify(importedName)}];`)
    .join('\n');
  return `/* eslint-disable */
// @ts-nocheck -- Ajv standalone output is generated JavaScript stored beside typed modules.
// Code generated by pnpm generate; DO NOT EDIT.
// Source: ${sourceReference}
${imports}
const validators = {};
${code}
export default validators;
`;
}

function compileRootSchemaValidator(schema: Record<string, unknown>): string {
  const rootID = schema.$id;
  if (typeof rootID !== 'string' || rootID === '') {
    throw new Error('configuration schema has no stable root $id');
  }
  // The UI validates complete documents; compiling parser-discovered sub-schemas
  // separately duplicates the same large definition graph in the browser bundle.
  const ajv = createAjvInstance(
    undefined,
    undefined,
    {
      code: { lines: false, optimize: 2, source: true },
      inlineRefs: false,
      schemas: [schema],
      strict: false,
      verbose: false,
    },
    undefined,
    Ajv2020,
  );
  return standaloneCode(ajv);
}

async function compareGeneratedFiles(
  outputRoot: string,
  expected: ReadonlyMap<string, string>,
): Promise<string[]> {
  const actual = await listFiles(outputRoot);
  const differences: string[] = [];
  for (const [name, content] of expected) {
    try {
      if (await readFile(join(outputRoot, name), 'utf8') !== content) differences.push(name);
    } catch (error) {
      if (!isNodeError(error, 'ENOENT')) throw error;
      differences.push(name);
    }
  }
  for (const name of actual) {
    if (!expected.has(name)) differences.push(name);
  }
  return [...new Set(differences)].sort();
}

async function syncGeneratedFiles(
  outputRoot: string,
  expected: ReadonlyMap<string, string>,
): Promise<void> {
  const parent = dirname(outputRoot);
  await mkdir(outputRoot, { recursive: true });
  const nextRoot = await mkdtemp(join(parent, '.configuration-schema-next-'));
  await chmod(nextRoot, 0o755);
  try {
    // Keep the directory available to concurrent builds. Publish referenced
    // content before the manifest that exposes it to consumers.
    const assets = [...expected.keys()].filter(name => name !== 'manifest.json');
    for (let attempt = 0; attempt < 3; attempt++) {
      for (const [name, content] of expected) {
        await writeFile(join(nextRoot, name), content, { encoding: 'utf8', mode: 0o644 });
      }
      for (const name of [...assets, 'manifest.json']) {
        await rename(join(nextRoot, name), join(outputRoot, name));
      }
      for (const name of await listFiles(outputRoot)) {
        if (!expected.has(name)) await rm(join(outputRoot, name), { force: true, recursive: true });
      }
      // A different concurrent input can remove assets referenced by our manifest.
      // Verify the whole set after publication and retry it as one complete set.
      if ((await compareGeneratedFiles(outputRoot, expected)).length === 0) return;
    }
    throw new Error('concurrent configuration schema generation conflict; rerun pnpm generate after other generators finish');
  } finally {
    await rm(nextRoot, { force: true, recursive: true });
  }
}

async function listFiles(root: string): Promise<string[]> {
  try {
    return (await readdir(root)).sort();
  } catch (error) {
    if (isNodeError(error, 'ENOENT')) return [];
    throw error;
  }
}

function isNodeError(error: unknown, code: string): error is NodeJS.ErrnoException {
  return error instanceof Error && 'code' in error && error.code === code;
}

function sha256(value: string): string {
  return createHash('sha256').update(value).digest('hex');
}

function isSha256(value: unknown): value is string {
  return typeof value === 'string' && /^[a-f0-9]{64}$/.test(value);
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  const arguments_ = process.argv.slice(2);
  if (arguments_.length > 1 || (arguments_.length === 1 && arguments_[0] !== '--check')) {
    throw new Error('usage: configuration-schema.ts [--check]');
  }
  await generateConfigurationSchemaArtifacts(fileURLToPath(new URL('..', import.meta.url)), {
    check: arguments_[0] === '--check',
  });
}
