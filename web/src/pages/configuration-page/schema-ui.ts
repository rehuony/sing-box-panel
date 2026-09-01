import type { RJSFSchema, UiSchema } from '@rjsf/utils';

interface PanelMetadata {
  order?: number;
  widget?: string;
  section?: string;
  collection?: string;
}

const discriminatorKeys = ['type', 'action', 'provider'] as const;

function objectSchema(value: unknown): RJSFSchema | undefined {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
    ? value as RJSFSchema
    : undefined;
}

function pointerValue(root: RJSFSchema, pointer: string): RJSFSchema | undefined {
  if (!pointer.startsWith('#/')) return undefined;
  let current: unknown = root;
  for (const encodedToken of pointer.slice(2).split('/')) {
    const token = encodedToken.replaceAll('~1', '/').replaceAll('~0', '~');
    if (current === null || typeof current !== 'object' || Array.isArray(current)) return undefined;
    current = (current as Record<string, unknown>)[token];
  }
  return objectSchema(current);
}

function mergeSchemas(base: RJSFSchema, extension: RJSFSchema): RJSFSchema {
  const properties = {
    ...(objectSchema(base.properties) ?? {}),
    ...(objectSchema(extension.properties) ?? {}),
  };
  const required = [...new Set([
    ...(Array.isArray(base.required) ? base.required : []),
    ...(Array.isArray(extension.required) ? extension.required : []),
  ])];
  return {
    ...base,
    ...extension,
    ...(Object.keys(properties).length === 0 ? {} : { properties }),
    ...(required.length === 0 ? {} : { required }),
  };
}

function resolveReference(
  schema: RJSFSchema,
  root: RJSFSchema,
  visited = new Set<string>(),
): RJSFSchema {
  const reference = typeof schema.$ref === 'string' ? schema.$ref : undefined;
  if (reference === undefined || visited.has(reference)) return schema;
  const target = pointerValue(root, reference);
  if (target === undefined) return schema;
  const nextVisited = new Set(visited).add(reference);
  const { $ref: _reference, ...siblings } = schema;
  return mergeSchemas(resolveReference(target, root, nextVisited), siblings);
}

function allowedValues(schema: RJSFSchema | undefined): unknown[] {
  if (schema === undefined) return [];
  if (schema.const !== undefined) return [schema.const];
  return Array.isArray(schema.enum) ? schema.enum : [];
}

function directProperties(schema: RJSFSchema): Record<string, RJSFSchema> {
  const value = schema.properties;
  if (value === undefined) return {};
  return Object.fromEntries(Object.entries(value).filter(
    (entry): entry is [string, RJSFSchema] => entry[1] !== false && entry[1] !== true,
  ));
}

function discriminatorValues(
  schema: RJSFSchema,
  root: RJSFSchema,
  key: string,
  visited = new Set<string>(),
): unknown[] {
  const result = [...allowedValues(directProperties(schema)[key])];
  if (typeof schema.$ref === 'string' && !visited.has(schema.$ref)) {
    const target = pointerValue(root, schema.$ref);
    if (target !== undefined) {
      result.push(...discriminatorValues(target, root, key, new Set(visited).add(schema.$ref)));
    }
  }
  for (const composition of [schema.allOf, schema.oneOf, schema.anyOf]) {
    if (!Array.isArray(composition)) continue;
    for (const value of composition) {
      const child = objectSchema(value);
      if (child !== undefined) result.push(...discriminatorValues(child, root, key, visited));
    }
  }
  return result;
}

function requiresProperty(
  schema: RJSFSchema,
  root: RJSFSchema,
  key: string,
  visited = new Set<string>(),
): boolean {
  if (Array.isArray(schema.required) && schema.required.includes(key)) return true;
  if (typeof schema.$ref === 'string' && !visited.has(schema.$ref)) {
    const target = pointerValue(root, schema.$ref);
    if (target !== undefined && requiresProperty(target, root, key, new Set(visited).add(schema.$ref))) {
      return true;
    }
  }
  if (Array.isArray(schema.allOf) && schema.allOf.some((value) => {
    const child = objectSchema(value);
    return child !== undefined && requiresProperty(child, root, key, visited);
  })) {
    return true;
  }
  for (const union of [schema.oneOf, schema.anyOf]) {
    if (!Array.isArray(union) || union.length === 0) continue;
    const branches = union.map(objectSchema).filter((branch): branch is RJSFSchema => branch !== undefined);
    if (branches.length > 0 && branches.every((branch) => requiresProperty(branch, root, key, visited))) {
      return true;
    }
  }
  return false;
}

function matchingUnionBranch(
  branches: unknown,
  root: RJSFSchema,
  data: unknown,
): RJSFSchema | undefined {
  if (!Array.isArray(branches) || data === null || typeof data !== 'object' || Array.isArray(data)) return undefined;
  const record = data as Record<string, unknown>;
  let best: { branch: RJSFSchema; score: number } | undefined;
  let defaultBranch: RJSFSchema | undefined;
  for (const value of branches) {
    const branch = objectSchema(value);
    if (branch === undefined) continue;
    let score = 0;
    let rejected = false;
    let optionalDiscriminator = false;
    for (const key of discriminatorKeys) {
      const allowed = discriminatorValues(branch, root, key);
      if (allowed.length === 0) continue;
      if (!Object.hasOwn(record, key)) {
        if (!requiresProperty(branch, root, key)) optionalDiscriminator = true;
        continue;
      }
      if (!allowed.some((candidate) => Object.is(candidate, record[key]))) {
        rejected = true;
        break;
      }
      score += 1;
    }
    if (!rejected && score === 0 && optionalDiscriminator && defaultBranch === undefined) {
      defaultBranch = branch;
    }
    if (!rejected && score > 0 && (best === undefined || score > best.score)) {
      best = { branch, score };
    }
  }
  return best?.branch ?? defaultBranch;
}

function unionProperties(branches: unknown, root: RJSFSchema): Record<string, RJSFSchema> {
  if (!Array.isArray(branches)) return {};
  return branches.reduce<Record<string, RJSFSchema>>((result, value) => {
    const branch = objectSchema(value);
    return branch === undefined ? result : { ...result, ...schemaProperties(branch, root) };
  }, {});
}

export function resolvedSchema(
  schema: RJSFSchema,
  root: RJSFSchema = schema,
  data?: unknown,
): RJSFSchema {
  let result = resolveReference(schema, root);
  if (Array.isArray(result.allOf)) {
    for (const part of result.allOf) {
      const child = objectSchema(part);
      if (child !== undefined) result = mergeSchemas(result, resolvedSchema(child, root, data));
    }
  }
  const union = Array.isArray(result.oneOf) ? result.oneOf : result.anyOf;
  const selected = matchingUnionBranch(union, root, data);
  if (selected !== undefined) {
    const { anyOf: _anyOf, oneOf: _oneOf, ...base } = result;
    result = mergeSchemas(base, resolvedSchema(selected, root, data));
  }
  return result;
}

export function panelMetadata(schema: RJSFSchema | undefined): PanelMetadata {
  const value = schema?.['x-panel'];
  return value !== null && typeof value === 'object' && !Array.isArray(value)
    ? value as PanelMetadata
    : {};
}

export function schemaProperties(
  schema: RJSFSchema,
  root: RJSFSchema = schema,
  data?: unknown,
): Record<string, RJSFSchema> {
  const resolved = resolvedSchema(schema, root, data);
  const direct = directProperties(resolved);
  if (data !== undefined) return direct;
  return {
    ...unionProperties(resolved.oneOf, root),
    ...unionProperties(resolved.anyOf, root),
    ...direct,
  };
}

export function schemaDiscriminatorValues(
  schema: RJSFSchema,
  root: RJSFSchema,
  key: string,
): string[] {
  const values = new Set<string>();
  for (const candidate of discriminatorValues(schema, root, key)) {
    if (typeof candidate === 'string') {
      values.add(candidate);
    }
  }
  return [...values];
}

export function collectionItemSchema(
  schema: RJSFSchema,
  root: RJSFSchema,
  data?: unknown,
): RJSFSchema | null {
  const resolved = resolvedSchema(schema, root, data);
  return resolved.items !== undefined && !Array.isArray(resolved.items)
    && resolved.items !== true && resolved.items !== false
    ? resolved.items
    : null;
}

export function selfContainedSchema(
  schema: RJSFSchema,
  root: RJSFSchema,
  data?: unknown,
): RJSFSchema {
  const resolved = resolvedSchema(schema, root, data);
  return {
    ...resolved,
    ...(root.$defs === undefined ? {} : { $defs: root.$defs }),
    ...(root.definitions === undefined ? {} : { definitions: root.definitions }),
  };
}

function patternSchema(schema: RJSFSchema, key: string): RJSFSchema | undefined {
  if (schema.patternProperties === undefined) return undefined;
  for (const [pattern, candidate] of Object.entries(schema.patternProperties)) {
    if (candidate === true || candidate === false) continue;
    try {
      if (new RegExp(pattern).test(key)) return candidate;
    } catch {
      // Invalid patterns are rejected by Schema generation; ignore defensively here.
    }
  }
  return undefined;
}

function dynamicPropertySchema(schema: RJSFSchema, key: string): RJSFSchema | true | undefined {
  const pattern = patternSchema(schema, key);
  if (pattern !== undefined) return pattern;
  if (schema.additionalProperties === true) return true;
  return objectSchema(schema.additionalProperties);
}

function sameJSON(left: unknown, right: unknown): boolean {
  return JSON.stringify(left) === JSON.stringify(right);
}

export function schemaDataIdentity(value: unknown): string | undefined {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) return undefined;
  const record = value as Record<string, unknown>;
  for (const key of ['id', 'tag', 'name']) {
    const candidate = record[key];
    if (typeof candidate === 'string' && candidate !== '') return `${key}:${candidate}`;
  }
  return undefined;
}

export function projectSchemaKnownData(
  schema: RJSFSchema,
  root: RJSFSchema,
  value: unknown,
): unknown {
  const resolved = resolvedSchema(schema, root, value);
  if (Array.isArray(value)) {
    const itemSchema = collectionItemSchema(resolved, root);
    if (itemSchema === null) return [];
    return value.map((item) => projectSchemaKnownData(itemSchema, root, item));
  }
  if (value === null || typeof value !== 'object') return value;
  const properties = schemaProperties(resolved, root, value);
  const record = value as Record<string, unknown>;
  return Object.fromEntries(Object.entries(record).flatMap(([key, child]) => {
    const property = properties[key];
    if (property !== undefined) {
      const projected = projectSchemaKnownData(property, root, child);
      return projected === undefined ? [] : [[key, projected]];
    }
    const dynamicSchema = dynamicPropertySchema(resolved, key);
    if (dynamicSchema === undefined) return [];
    const projected = dynamicSchema === true
      ? child
      : projectSchemaKnownData(dynamicSchema, root, child);
    return projected === undefined ? [] : [[key, projected]];
  }));
}

/**
 * Applies an edited schema projection to its canonical source without erasing
 * fields that the structured editor cannot see. Array members that retain the
 * same known projection (or stable identity) reuse their original value.
 */
export function mergeSchemaKnownData(
  schema: RJSFSchema,
  root: RJSFSchema,
  original: unknown,
  before: unknown,
  after: unknown,
): unknown {
  if (sameJSON(before, after)) return original;
  const resolved = resolvedSchema(schema, root, after);
  if (Array.isArray(before) && Array.isArray(after)) {
    const itemSchema = collectionItemSchema(resolved, root);
    if (itemSchema === null || !Array.isArray(original)) return after;
    const visible = original.map((raw) => ({
      known: projectSchemaKnownData(itemSchema, root, raw),
      raw,
    }));
    const used = new Set<number>();
    const desired = after.map((known, index) => {
      let match = visible.findIndex((candidate, candidateIndex) =>
        !used.has(candidateIndex) && sameJSON(candidate.known, known));
      if (match < 0) {
        const identity = schemaDataIdentity(known);
        if (identity !== undefined) {
          match = visible.findIndex((candidate, candidateIndex) =>
            !used.has(candidateIndex) && schemaDataIdentity(candidate.known) === identity);
        }
      }
      if (match < 0 && before.length === after.length && visible[index] !== undefined && !used.has(index)) {
        match = index;
      }
      if (match < 0) return known;
      used.add(match);
      const candidate = visible[match];
      return mergeSchemaKnownData(itemSchema, root, candidate.raw, candidate.known, known);
    });
    return desired;
  }
  if (
    before !== null && after !== null && original !== null
    && typeof before === 'object' && typeof after === 'object' && typeof original === 'object'
    && !Array.isArray(before) && !Array.isArray(after) && !Array.isArray(original)
  ) {
    const prior = before as Record<string, unknown>;
    const next = after as Record<string, unknown>;
    const source = original as Record<string, unknown>;
    const merged: Record<string, unknown> = { ...source };
    for (const key of Object.keys(prior)) {
      if (!Object.hasOwn(next, key)) delete merged[key];
    }
    const properties = schemaProperties(resolved, root, after);
    for (const [key, value] of Object.entries(next)) {
      const property = properties[key];
      if (property !== undefined) {
        merged[key] = mergeSchemaKnownData(property, root, source[key], prior[key], value);
        continue;
      }
      const dynamicSchema = dynamicPropertySchema(resolved, key);
      merged[key] = dynamicSchema === undefined || dynamicSchema === true
        ? value
        : mergeSchemaKnownData(dynamicSchema, root, source[key], prior[key], value);
    }
    return merged;
  }
  return after;
}

export function uiSchemaFromPanel(
  schema: RJSFSchema,
  readonlyPaths: string[] = [],
  root: RJSFSchema = schema,
  data?: unknown,
): UiSchema {
  const result: UiSchema = {};
  const resolved = resolvedSchema(schema, root, data);
  const metadata = panelMetadata(resolved);
  if (metadata.widget !== undefined) result['ui:widget'] = metadata.widget;
  const record = data !== null && typeof data === 'object' && !Array.isArray(data)
    ? data as Record<string, unknown>
    : {};
  for (const [key, property] of Object.entries(schemaProperties(resolved, root, data))) {
    const child = uiSchemaFromPanel(property, readonlyPaths.map((path) =>
      path.startsWith(`${key}.`) ? path.slice(key.length + 1) : path,
    ).filter((path) => path !== key), root, record[key]);
    if (readonlyPaths.includes(key)) child['ui:readonly'] = true;
    result[key] = child;
  }
  return result;
}
