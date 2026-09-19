// SPDX-License-Identifier: GPL-3.0-or-later

import type { Plugin, ResolvedConfig } from 'vite';

import { tmpdir } from 'node:os';
import process from 'node:process';
import { promisify } from 'node:util';
import Ajv2020 from 'ajv/dist/2020.js';
import { createHash } from 'node:crypto';
import standaloneCode from 'ajv/dist/standalone/index.js';
import { execFile as execFileCallback } from 'node:child_process';
import { dirname, join, parse, relative, resolve, sep } from 'node:path';
import createAjvInstance from '@rjsf/validator-ajv8/lib/createAjvInstance.js';
import { chmod, mkdir, mkdtemp, readdir, readFile, rename, rm, stat, writeFile } from 'node:fs/promises';

const execFile = promisify(execFileCallback);

interface SchemaManifestEntry {
  schema_file: string;
  exact_version: string;
  schema_sha256: string;
}

interface SchemaManifest {
  schema_version: number;
  entries: SchemaManifestEntry[];
}

export interface ConfigurationSchemaPluginOptions {
  check?: boolean;
}

export function configurationSchemaPlugin(
  options: ConfigurationSchemaPluginOptions = {},
): Plugin {
  let config: ResolvedConfig;
  let generation: Promise<void> | undefined;
  return {
    name: 'sing-box-panel:configuration-schema',
    enforce: 'pre',
    configResolved(resolvedConfig) {
      config = resolvedConfig;
    },
    async buildStart() {
      generation ??= generateConfigurationSchemaArtifacts(config.root, {
        check: options.check ?? process.env.CI !== undefined,
      });
      await generation;
    },
  };
}

export async function generateConfigurationSchemaArtifacts(
  webRoot: string,
  options: ConfigurationSchemaPluginOptions = {},
): Promise<void> {
  const repositoryRoot = resolve(webRoot, '..');
  const outputRoot = join(webRoot, 'src', 'schemas', 'generated');
  const sourceRoot = await mkdtemp(join(tmpdir(), 'sing-box-panel-schema-web-'));
  try {
    await exportConfigurationSchemaAssets(repositoryRoot, sourceRoot);
    const manifestSource = await readFile(join(sourceRoot, 'manifest.json'), 'utf8');
    const manifest = parseConfigurationSchemaManifest(manifestSource);
    const generated = new Map<string, string>();
    generated.set('manifest.json', `${JSON.stringify(manifest, null, 2)}\n`);

    const indexEntries: string[] = [];
    for (const entry of manifest.entries) {
      const schemaPath = safeAssetPath(sourceRoot, entry.schema_file);
      const schemaSource = await readFile(schemaPath, 'utf8');
      if (sha256(schemaSource) !== entry.schema_sha256) {
        throw new Error(`schema ${entry.exact_version} differs from its manifest digest`);
      }
      const schema = JSON.parse(schemaSource) as Record<string, unknown>;
      const outputSchemaFile = normalizePath(entry.schema_file);
      const schemaDirectory = parse(outputSchemaFile).dir;
      const validatorBase = parse(outputSchemaFile).name.replace(/^schema-/, 'validator-');
      const validatorFile = normalizePath(join(schemaDirectory, `${validatorBase}.ts`));
      generated.set(outputSchemaFile, schemaSource);
      generated.set(validatorFile, compileValidator(entry, schema, validatorFile));
      indexEntries.push(renderIndexEntry(entry, outputSchemaFile, validatorFile));
    }
    generated.set('index.ts', renderIndex(indexEntries));

    const differences = await compareGeneratedFiles(outputRoot, generated);
    if (options.check && differences.length > 0) {
      throw new Error(
        `configuration schema browser artifacts are stale (${differences.join(', ')}); run a local Vite command to regenerate them`,
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
      || typeof entry.schema_file !== 'string' || entry.schema_file === ''
      || !isSha256(entry.schema_sha256)
    ) {
      throw new Error(`configuration schema manifest entry for ${entry.exact_version ?? 'unknown'} is incomplete`);
    }
    if (versions.has(entry.exact_version)) {
      throw new Error(`configuration schema manifest contains a duplicate entry for ${entry.exact_version}`);
    }
    versions.add(entry.exact_version);
    entries.push({
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

function compileValidator(
  entry: SchemaManifestEntry,
  schema: Record<string, unknown>,
  validatorFile: string,
): string {
  return compileConfigurationSchemaValidator(
    schema,
    `github.com/sagernet/sing-box@v${entry.exact_version}`,
    validatorFile,
  );
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
// Code generated by the configuration Schema Vite plugin; DO NOT EDIT.
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

function renderIndexEntry(
  entry: SchemaManifestEntry,
  schemaFile: string,
  validatorFile: string,
): string {
  const schemaImport = `./${schemaFile}`;
  const validatorImport = `./${validatorFile.replace(/\.ts$/, '')}`;
  return `  ${JSON.stringify(entry.exact_version)}: {
    exactVersion: ${JSON.stringify(entry.exact_version)},
    schemaSHA256: ${JSON.stringify(entry.schema_sha256)},
    async load() {
      const [schemaModule, validatorModule] = await Promise.all([
        import(${JSON.stringify(schemaImport)}),
        import(${JSON.stringify(validatorImport)}),
      ]);
      return {
        exactVersion: ${JSON.stringify(entry.exact_version)},
        schemaSHA256: ${JSON.stringify(entry.schema_sha256)},
        schema: schemaModule.default as unknown as RJSFSchema,
        validateFns: validatorModule.default as unknown as ReviewedValidatorFunctions,
      };
    },
  },`;
}

function renderIndex(entries: string[]): string {
  return `/* eslint-disable */
// Code generated by the configuration Schema Vite plugin; DO NOT EDIT.
import type { RJSFSchema } from '@rjsf/utils';

type ReviewedValidatorFunction = ((data: unknown) => boolean) & { errors?: unknown };
type ReviewedValidatorFunctions = Record<string, ReviewedValidatorFunction>;

export interface ReviewedSchemaEntry {
  exactVersion: string;
  schemaSHA256: string;
  schema: RJSFSchema;
  validateFns: ReviewedValidatorFunctions;
}

export interface ReviewedSchemaManifestEntry {
  exactVersion: string;
  schemaSHA256: string;
  load: () => Promise<ReviewedSchemaEntry>;
}

export const reviewedSchemaManifest: Readonly<Record<string, ReviewedSchemaManifestEntry>> = {
${entries.join('\n')}
};

export function hasReviewedSchemaVersion(exactVersion: string): boolean {
  return reviewedSchemaManifest[exactVersion] !== undefined;
}

export async function loadReviewedSchema(exactVersion: string): Promise<ReviewedSchemaEntry | undefined> {
  return reviewedSchemaManifest[exactVersion]?.load();
}
`;
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
    } catch {
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
  await mkdir(parent, { recursive: true });
  const nextRoot = await mkdtemp(join(parent, '.configuration-schema-next-'));
  await chmod(nextRoot, 0o755);
  const previousRoot = join(parent, `.configuration-schema-previous-${process.pid}-${Math.random().toString(16).slice(2)}`);
  let movedPrevious = false;
  try {
    for (const [name, content] of expected) {
      const target = join(nextRoot, name);
      await mkdir(dirname(target), { recursive: true });
      await writeFile(target, content, { encoding: 'utf8', mode: 0o644 });
    }
    try {
      await rename(outputRoot, previousRoot);
      movedPrevious = true;
    } catch (error) {
      if (!isNodeError(error, 'ENOENT')) throw error;
    }
    try {
      await rename(nextRoot, outputRoot);
    } catch (error) {
      if (movedPrevious) await rename(previousRoot, outputRoot);
      throw error;
    }
    if (movedPrevious) await rm(previousRoot, { force: true, recursive: true });
  } finally {
    await rm(nextRoot, { force: true, recursive: true });
  }
}

async function listFiles(root: string): Promise<string[]> {
  try {
    if (!(await stat(root)).isDirectory()) return [];
  } catch {
    return [];
  }
  const result: string[] = [];
  async function visit(directory: string): Promise<void> {
    for (const entry of await readdir(directory, { withFileTypes: true })) {
      const current = join(directory, entry.name);
      if (entry.isDirectory()) await visit(current);
      else if (entry.isFile()) result.push(normalizePath(relative(root, current)));
    }
  }
  await visit(root);
  return result.sort();
}

function isNodeError(error: unknown, code: string): error is NodeJS.ErrnoException {
  return error instanceof Error && 'code' in error && error.code === code;
}

function safeAssetPath(root: string, name: string): string {
  const normalized = normalizePath(name);
  if (normalized !== name || normalized.startsWith('../') || normalized.startsWith('/')) {
    throw new Error(`unsafe configuration schema asset path ${JSON.stringify(name)}`);
  }
  const target = resolve(root, name);
  if (!target.startsWith(`${resolve(root)}${sep}`)) {
    throw new Error(`configuration schema asset escapes its root: ${JSON.stringify(name)}`);
  }
  return target;
}

function normalizePath(value: string): string {
  return value.split(sep).join('/');
}

function sha256(value: string): string {
  return createHash('sha256').update(value).digest('hex');
}

function isSha256(value: unknown): value is string {
  return typeof value === 'string' && /^[a-f0-9]{64}$/.test(value);
}
