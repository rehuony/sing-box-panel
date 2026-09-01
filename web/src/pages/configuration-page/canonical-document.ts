import {
  isLosslessNumber,
  parse as parseLosslessJSON,
  stringify as stringifyLosslessJSON,
} from 'lossless-json';

import type { CanonicalDocument, CanonicalSnapshot } from '@/api/api-client';

import i18n from '@/i18n';

function pointerTokens(pointer: string): string[] {
  if (!pointer.startsWith('/')) return [];
  return pointer
    .slice(1)
    .split('/')
    .map((token) => token.replaceAll('~1', '/').replaceAll('~0', '~'));
}

export function valueAtPointer(document: unknown, pointer: string): unknown {
  let current = document;
  for (const token of pointerTokens(pointer)) {
    if (current === null || typeof current !== 'object' || isLosslessNumber(current)) return undefined;
    if (Array.isArray(current)) {
      if (!/^(?:0|[1-9]\d*)$/.test(token)) return undefined;
      current = current[Number(token)];
      continue;
    }
    if (!Object.hasOwn(current, token)) return undefined;
    current = (current as Record<string, unknown>)[token];
  }
  return current;
}

export function parseCanonicalDocument(snapshot: CanonicalSnapshot): CanonicalDocument {
  const parsed = parseLosslessJSON(snapshot.document_json);
  if (parsed === null || Array.isArray(parsed) || typeof parsed !== 'object') {
    throw new Error(i18n.t('configuration.canonical.error.snapshotObject'));
  }
  return parsed as CanonicalDocument;
}

export function cloneCanonicalDocument(source: CanonicalDocument): CanonicalDocument {
  const encoded = stringifyLosslessJSON(source);
  if (encoded === undefined) throw new Error(i18n.t('configuration.canonical.error.encodeLossless'));
  const parsed = parseLosslessJSON(encoded);
  if (parsed === null || Array.isArray(parsed) || typeof parsed !== 'object') {
    throw new Error(i18n.t('configuration.canonical.error.cloneObject'));
  }
  return parsed as CanonicalDocument;
}

function arrayPointerIndex(token: string, pointer: string, length: number): number {
  if (!/^(?:0|[1-9]\d*)$/.test(token)) {
    throw new Error(i18n.t('configuration.canonical.error.arrayIndexNumeric', { pointer }));
  }
  const index = Number(token);
  if (!Number.isSafeInteger(index) || index > 100_000) {
    throw new Error(i18n.t('configuration.canonical.error.arrayIndexLimit', { pointer }));
  }
  if (index >= length) {
    throw new Error(i18n.t('configuration.canonical.error.arrayItemMissing', { pointer }));
  }
  return index;
}

function defineObjectValue(target: Record<string, unknown>, key: string, value: unknown) {
  Object.defineProperty(target, key, {
    configurable: true,
    enumerable: true,
    value,
    writable: true,
  });
}

export function documentWithValue(
  source: CanonicalDocument,
  pointer: string,
  value: unknown,
): CanonicalDocument {
  const copy = cloneCanonicalDocument(source);
  const tokens = pointerTokens(pointer);
  let current: unknown = copy;
  for (let index = 0; index < tokens.length; index += 1) {
    const token = tokens[index];
    const final = index === tokens.length - 1;
    if (Array.isArray(current)) {
      const arrayIndex = arrayPointerIndex(token, pointer, current.length);
      if (final) {
        current[arrayIndex] = value;
        break;
      }
      if (current[arrayIndex] === null || typeof current[arrayIndex] !== 'object') {
        throw new Error(i18n.t('configuration.canonical.error.nonContainer', { pointer }));
      }
      current = current[arrayIndex];
      continue;
    }
    if (current === null || typeof current !== 'object' || isLosslessNumber(current)) {
      throw new Error(i18n.t('configuration.canonical.error.nonContainer', { pointer }));
    }
    const object = current as Record<string, unknown>;
    if (final) {
      defineObjectValue(object, token, value);
      break;
    }
    if (!Object.hasOwn(object, token)) {
      throw new Error(i18n.t('configuration.canonical.error.parentMissing', { pointer }));
    }
    if (object[token] === null || typeof object[token] !== 'object') {
      throw new Error(i18n.t('configuration.canonical.error.nonContainer', { pointer }));
    }
    current = object[token];
  }
  return copy;
}

export function documentWithoutValue(source: CanonicalDocument, pointer: string): CanonicalDocument {
  const copy = cloneCanonicalDocument(source);
  const tokens = pointerTokens(pointer);
  let current: unknown = copy;
  for (let index = 0; index < tokens.length - 1; index += 1) {
    const token = tokens[index];
    if (Array.isArray(current)) {
      current = current[arrayPointerIndex(token, pointer, current.length)];
    } else if (current !== null && typeof current === 'object' && !isLosslessNumber(current)) {
      if (!Object.hasOwn(current, token)) return copy;
      current = (current as Record<string, unknown>)[token];
    } else {
      throw new Error(i18n.t('configuration.canonical.error.nonContainer', { pointer }));
    }
  }
  const final = tokens.at(-1);
  if (final === undefined) return copy;
  if (Array.isArray(current)) {
    const index = arrayPointerIndex(final, pointer, current.length);
    current.splice(index, 1);
  } else if (current !== null && typeof current === 'object' && !isLosslessNumber(current)) {
    delete (current as Record<string, unknown>)[final];
  } else {
    throw new Error(i18n.t('configuration.canonical.error.nonContainer', { pointer }));
  }
  return copy;
}
