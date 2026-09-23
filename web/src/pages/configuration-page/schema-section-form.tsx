import type { ReactNode } from 'react';
import type { IChangeEvent } from '@rjsf/core';
import type { RJSFSchema, UiSchema, ValidationData, ValidatorType } from '@rjsf/utils';

import Form from '@rjsf/core';
import { useId, useMemo } from 'react';
import { getSchemaType } from '@rjsf/utils';

import type { ReviewedSchemaResolution } from '@/schemas/resolve-reviewed-schema';

import type { CanonicalDraft } from './use-canonical-configuration';

import { encodeCanonicalValue } from './use-canonical-configuration';
import { documentWithoutValue, documentWithValue, valueAtPointer } from './canonical-document';
import { panelRJSFFields, panelRJSFTemplates, panelRJSFWidgets, SchemaDialogLayout } from './rjsf-shadcn-theme';
import {
  isByteArraySchema,
  mergeSchemaKnownData,
  projectSchemaKnownData,
  resolvedSchema,
  schemaDataIdentity,
  selfContainedSchema,
  uiSchemaFromPanel,
} from './schema-ui';

interface SchemaSectionFormProps {
  data: unknown;
  disabled?: boolean;
  schema: RJSFSchema;
  basePointer: string;
  uiSchema?: UiSchema;
  dialogLayout?: boolean;
  protectedPaths?: string[];
  dialogJsonPreview?: ReactNode;
  dialogPrimaryContent?: ReactNode;
  resolution: ReviewedSchemaResolution;
  onTouched?: (paths: string[]) => void;
  arrayLayout?: 'default' | 'standalone';
  arrayActionContainer?: HTMLElement | null;
  onChange: (change: (draft: CanonicalDraft) => CanonicalDraft) => void;
}

function displayCopy(value: unknown): unknown {
  return value === undefined ? {} : JSON.parse(encodeCanonicalValue(value));
}

function escapePointerToken(token: string): string {
  return token.replaceAll('~', '~0').replaceAll('/', '~1');
}

function sameJSON(left: unknown, right: unknown): boolean {
  return JSON.stringify(left) === JSON.stringify(right);
}

function arrayMembersChanged(before: unknown[], after: unknown[]): boolean {
  const beforeIdentities = before.map(schemaDataIdentity);
  const afterIdentities = after.map(schemaDataIdentity);
  if (
    beforeIdentities.some((identity) => identity !== undefined)
    || afterIdentities.some((identity) => identity !== undefined)
  ) {
    return beforeIdentities.some((identity, index) => identity !== afterIdentities[index]);
  }

  const used = new Set<number>();
  for (const [index, value] of after.entries()) {
    const originalIndex = before.findIndex((candidate, candidateIndex) =>
      !used.has(candidateIndex) && sameJSON(candidate, value));
    if (originalIndex < 0) return false;
    used.add(originalIndex);
    if (originalIndex !== index) return true;
  }
  return false;
}

function changedPointers(before: unknown, after: unknown, pointer = ''): string[] {
  if (sameJSON(before, after)) return [];
  if (Array.isArray(before) && Array.isArray(after)) {
    if (before.length !== after.length || arrayMembersChanged(before, after)) return [pointer];
    return before.flatMap((value, index) => changedPointers(value, after[index], `${pointer}/${index}`));
  }
  if (
    before !== null && after !== null
    && typeof before === 'object' && typeof after === 'object'
    && !Array.isArray(before) && !Array.isArray(after)
  ) {
    const left = before as Record<string, unknown>;
    const right = after as Record<string, unknown>;
    const keys = new Set([...Object.keys(left), ...Object.keys(right)]);
    return [...keys].flatMap((key) => changedPointers(
      left[key], right[key], `${pointer}/${escapePointerToken(key)}`,
    ));
  }
  return [pointer];
}

function joinPointer(base: string, relative: string): string {
  if (relative === '') return base;
  return `${base}${relative}`;
}

function isProtected(pointer: string, protectedPaths: string[]): boolean {
  return protectedPaths.some((protectedPath) =>
    pointer === protectedPath || pointer.startsWith(`${protectedPath}/`),
  );
}

function pointerTokens(pointer: string): string[] {
  return pointer.split('/').slice(1).map((token) =>
    token.replaceAll('~1', '/').replaceAll('~0', '~'));
}

function rootValidationData(basePointer: string, value: unknown): unknown {
  const tokens = pointerTokens(basePointer);
  return tokens.reduceRight<unknown>((current, token) => /^\d+$/.test(token)
    ? [current]
    : { [token]: current }, value);
}

function valueMatchesSchema(schema: RJSFSchema, root: RJSFSchema, value: unknown): boolean {
  const resolved = resolvedSchema(schema, root, value);
  if (resolved.const !== undefined && !Object.is(resolved.const, value)) return false;
  if (Array.isArray(resolved.enum) && !resolved.enum.some((candidate) => Object.is(candidate, value))) return false;
  const types = Array.isArray(resolved.type) ? resolved.type : resolved.type === undefined ? [] : [resolved.type];
  if (types.length > 0) {
    const matchesType = types.some((type) => {
      switch (type) {
        case 'null': return value === null;
        case 'array': return Array.isArray(value);
        case 'object': return value !== null && typeof value === 'object' && !Array.isArray(value);
        case 'integer': return typeof value === 'number' && Number.isInteger(value);
        case 'number': return typeof value === 'number';
        case 'string': return typeof value === 'string';
        case 'boolean': return typeof value === 'boolean';
        default: return true;
      }
    });
    if (!matchesType) return false;
  }
  // Representation matching must inspect array members as well as the outer type:
  // a byte sequence and a list of strings/byte sequences are both JSON arrays.
  for (const keyword of ['anyOf', 'oneOf'] as const) {
    if (resolved[keyword] && !resolved[keyword].some((branch) => typeof branch === 'boolean'
      ? branch
      : valueMatchesSchema(branch, root, value))) {
      return false;
    }
  }
  if (Array.isArray(value) && resolved.items && typeof resolved.items === 'object' && !Array.isArray(resolved.items)) {
    // Prefer the ordinary list editor for an ambiguous empty array. RJSF otherwise
    // falls back to the first option (even if that is a string) when array scores tie.
    // This is only UI matching; the reviewed root validator still accepts empty bytes.
    if (value.length === 0 && isByteArraySchema(resolved, root)) return false;
    const itemSchema = resolved.items;
    if (!value.every((item) => valueMatchesSchema(itemSchema, root, item))) return false;
  }
  return true;
}

function sectionValidator(
  resolution: ReviewedSchemaResolution,
  basePointer: string,
): ValidatorType<unknown> {
  const rootValidator = resolution.createValidator(resolution.schema);
  return {
    isValid(schema, formData, rootSchema) {
      return valueMatchesSchema(schema, rootSchema, formData);
    },
    rawValidation(_schema, formData) {
      return rootValidator.rawValidation(
        resolution.schema,
        rootValidationData(basePointer, formData),
      );
    },
    validateFormData(formData): ValidationData<unknown> {
      const result = rootValidator.validateFormData(
        rootValidationData(basePointer, formData),
        resolution.schema,
      );
      return {
        errors: result.errors.map((error) => ({
          ...error,
          property: '',
          stack: error.message ?? error.stack,
        })),
        errorSchema: result.errorSchema,
      };
    },
  };
}

export function SchemaSectionForm({
  arrayActionContainer,
  arrayLayout = 'default',
  basePointer,
  data,
  disabled = false,
  dialogLayout = false,
  dialogPrimaryContent,
  dialogJsonPreview,
  onChange,
  onTouched,
  protectedPaths = [],
  resolution,
  schema,
  uiSchema,
}: SchemaSectionFormProps) {
  const formId = useId();
  const formSchema = useMemo(
    () => selfContainedSchema(schema, resolution.schema, data),
    [data, resolution.schema, schema],
  );
  const external = useMemo(
    () => displayCopy(projectSchemaKnownData(schema, resolution.schema, data)
      ?? (getSchemaType(resolvedSchema(schema, resolution.schema)) === 'array' ? [] : {})),
    [data, resolution.schema, schema],
  );
  const validator = useMemo(
    () => sectionValidator(resolution, basePointer),
    [basePointer, resolution],
  );

  function handleChange(event: IChangeEvent) {
    const next = event.formData;
    const losslessNext = mergeSchemaKnownData(
      schema,
      resolution.schema,
      data,
      external,
      next,
    );
    const changed = changedPointers(external, next)
      .map((pointer) => joinPointer(basePointer, pointer))
      .filter((pointer) => !isProtected(pointer, protectedPaths));
    if (changed.length === 0) return;
    onTouched?.(changed);
    onChange((draft) => {
      let updated = draft;
      for (const pointer of changed) {
        const relative = pointer.slice(basePointer.length);
        const value = relative === ''
          ? losslessNext
          : valueAtDisplayPointer(losslessNext, relative);
        // Materialize optional object parents only for an actual edit, never on render.
        if (value !== undefined) {
          const segments = pointer.split('/');
          for (let end = 2; end < segments.length; end += 1) {
            const parent = segments.slice(0, end).join('/');
            if (valueAtPointer(updated, parent) == null) {
              updated = documentWithValue(updated, parent, {}) as CanonicalDraft;
            }
          }
        }
        updated = value === undefined
          ? documentWithoutValue(updated, pointer) as CanonicalDraft
          : documentWithValue(updated, pointer, value) as CanonicalDraft;
      }
      return updated;
    });
  }

  const form = (
    <Form
      className='schema-form'
      disabled={disabled}
      experimental_defaultFormStateBehavior={{ emptyObjectFields: 'populateRequiredDefaults' }}
      fields={panelRJSFFields}
      formData={external}
      formContext={{ arrayActionContainer, arrayLayout }}
      idPrefix={`schema-${formId}`}
      liveValidate
      noHtml5Validate
      onChange={handleChange}
      schema={formSchema}
      showErrorList={false}
      templates={panelRJSFTemplates}
      uiSchema={{
        ...uiSchemaFromPanel(schema, [], resolution.schema, data),
        ...uiSchema,
        'ui:submitButtonOptions': { norender: true },
      }}
      validator={validator}
      widgets={panelRJSFWidgets}
    />
  );
  return dialogLayout
    ? (
        <SchemaDialogLayout schema={schema} root={resolution.schema} data={external}
          primaryContent={dialogPrimaryContent} jsonPreview={dialogJsonPreview}>
          {form}
        </SchemaDialogLayout>
      )
    : form;
}

function valueAtDisplayPointer(value: unknown, pointer: string): unknown {
  if (pointer === '') return value;
  let current = value;
  for (const encodedToken of pointer.slice(1).split('/')) {
    const token = encodedToken.replaceAll('~1', '/').replaceAll('~0', '~');
    if (Array.isArray(current)) {
      current = current[Number(token)];
    } else if (current !== null && typeof current === 'object') {
      current = (current as Record<string, unknown>)[token];
    } else {
      return undefined;
    }
  }
  return current;
}
