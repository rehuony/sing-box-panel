import type { ChangeEvent, ReactNode } from 'react';

import {

  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import {
  compareLosslessNumber,
  isLosslessNumber,
  parse as parseLosslessJSON,
  stringify as stringifyLosslessJSON,
} from 'lossless-json';

import type { CanonicalChange, CanonicalDocument, CanonicalSnapshot, CapabilityClassification, CapabilitySemanticFact, CapabilityStatus, CapabilityUIDescriptor } from '@/api/api-client';

import {
  ApiRequestError,

} from '@/api/api-client';
import { useApiClient } from '@/api/api-client-context';
import { describeRequestError, ErrorNotice } from '@/components/error-notice';

interface VersionedCanonicalFormProps {
  exactVersion: string;
  capability: CapabilityStatus;
  onSaved: () => Promise<void>;
}

type LoadState
  = | { status: 'loading'; snapshot: null; error: null }
    | { status: 'error'; snapshot: null; error: unknown }
    | { status: 'ready'; snapshot: CanonicalSnapshot; error: null };

const classificationLabels: Record<CapabilityClassification, string> = {
  behavior_changed: 'Behavior changed in this version',
  intentionally_unsupported: 'Intentionally unsupported',
  supported: 'Supported',
};

function pointerTokens(pointer: string): string[] {
  if (!pointer.startsWith('/')) return [];
  return pointer
    .slice(1)
    .split('/')
    .map((token) => token.replaceAll('~1', '/').replaceAll('~0', '~'));
}

function valueAtPointer(document: unknown, pointer: string): unknown {
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

function parseCanonicalDocument(snapshot: CanonicalSnapshot): CanonicalDocument {
  const parsed = parseLosslessJSON(snapshot.document_json);
  if (parsed === null || Array.isArray(parsed) || typeof parsed !== 'object') {
    throw new Error('The canonical snapshot is not one JSON object.');
  }
  return parsed as CanonicalDocument;
}

function cloneCanonicalDocument(source: CanonicalDocument): CanonicalDocument {
  const encoded = stringifyLosslessJSON(source);
  if (encoded === undefined) throw new Error('The canonical snapshot cannot be encoded losslessly.');
  const parsed = parseLosslessJSON(encoded);
  if (parsed === null || Array.isArray(parsed) || typeof parsed !== 'object') {
    throw new Error('The canonical snapshot clone is not one JSON object.');
  }
  return parsed as CanonicalDocument;
}

function arrayPointerIndex(token: string, pointer: string, length: number): number {
  if (!/^(?:0|[1-9]\d*)$/.test(token)) {
    throw new Error(`Canonical path ${pointer} uses a non-numeric array index.`);
  }
  const index = Number(token);
  if (!Number.isSafeInteger(index) || index > 100_000) {
    throw new Error(`Canonical path ${pointer} exceeds the safe array-index limit.`);
  }
  if (index >= length) {
    throw new Error(`Canonical path ${pointer} addresses an array item that does not exist.`);
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

function documentWithValue(
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
        throw new Error(`Canonical path ${pointer} crosses a non-container value.`);
      }
      current = current[arrayIndex];
      continue;
    }
    if (current === null || typeof current !== 'object' || isLosslessNumber(current)) {
      throw new Error(`Canonical path ${pointer} crosses a non-container value.`);
    }
    const object = current as Record<string, unknown>;
    if (final) {
      defineObjectValue(object, token, value);
      break;
    }
    if (!Object.hasOwn(object, token)) {
      throw new Error(`Canonical path ${pointer} has a missing parent container.`);
    }
    if (object[token] === null || typeof object[token] !== 'object') {
      throw new Error(`Canonical path ${pointer} crosses a non-container value.`);
    }
    current = object[token];
  }
  return copy;
}

function documentWithoutValue(source: CanonicalDocument, pointer: string): CanonicalDocument {
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
      throw new Error(`Canonical path ${pointer} crosses a non-container value.`);
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
    throw new Error(`Canonical path ${pointer} crosses a non-container value.`);
  }
  return copy;
}

function descriptorInputID(descriptor: CapabilityUIDescriptor): string {
  // Manifest IDs are validated before they reach this boundary. Preserve their
  // full identity: replacing dots would make valid IDs such as `route.mode`
  // and `route-mode` point at the same form control.
  return `capability-field-${encodeURIComponent(descriptor.id)}`;
}

function isVisible(descriptor: CapabilityUIDescriptor, document: CanonicalDocument): boolean {
  const condition = descriptor.visible_when;
  if (condition === undefined) return true;
  try {
    const actual = valueAtPointer(document, condition.canonical_path);
    const expected = parseLosslessJSON(condition.equals_json);
    if (isLosslessNumber(actual) && isLosslessNumber(expected)) {
      return compareLosslessNumber(actual, expected) === 0;
    }
    return Object.is(actual, expected);
  } catch {
    return false;
  }
}

function initialJSONDrafts(
  descriptors: CapabilityUIDescriptor[],
  facts: Map<string, CapabilitySemanticFact>,
  document: CanonicalDocument,
): Record<string, string> {
  return Object.fromEntries(
    descriptors
      .filter((descriptor) => descriptor.kind === 'json')
      .map((descriptor) => {
        const path = facts.get(descriptor.fact_id)?.canonical_path ?? '';
        const value = valueAtPointer(document, path);
        return [descriptor.id, value === undefined ? '' : stringifyLosslessJSON(value, null, 2) ?? ''];
      }),
  );
}

function initialNumberDrafts(
  descriptors: CapabilityUIDescriptor[],
  facts: Map<string, CapabilitySemanticFact>,
  document: CanonicalDocument,
): Record<string, string> {
  return Object.fromEntries(
    descriptors
      .filter((descriptor) => descriptor.kind === 'number')
      .map((descriptor) => {
        const path = facts.get(descriptor.fact_id)?.canonical_path ?? '';
        const value = valueAtPointer(document, path);
        return [descriptor.id, isLosslessNumber(value) ? value.toString() : typeof value === 'number' ? String(value) : ''];
      }),
  );
}

export function VersionedCanonicalForm({
  capability,
  exactVersion,
  onSaved,
}: VersionedCanonicalFormProps) {
  const client = useApiClient();
  const presentation = capability.presentation!;
  const descriptors = useMemo(
    () => presentation.ui
      .map((descriptor, index) => ({ descriptor, index }))
      .sort((left, right) =>
        (left.descriptor.order ?? 0) - (right.descriptor.order ?? 0)
        || left.index - right.index,
      )
      .map(({ descriptor }) => descriptor),
    [presentation],
  );
  const facts = useMemo(
    () => new Map(presentation.semantic_facts.map((fact) => [fact.id, fact])),
    [presentation],
  );
  const [load, setLoad] = useState<LoadState>({ status: 'loading', snapshot: null, error: null });
  const [draft, setDraft] = useState<CanonicalDocument | null>(null);
  const [jsonDrafts, setJSONDrafts] = useState<Record<string, string>>({});
  const [numberDrafts, setNumberDrafts] = useState<Record<string, string>>({});
  const [dirtyFields, setDirtyFields] = useState<Record<string, boolean>>({});
  const [acceptedCompatible, setAcceptedCompatible] = useState(false);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [isSaving, setIsSaving] = useState(false);
  const [message, setMessage] = useState('');
  const [conflict, setConflict] = useState(false);
  const errorSummaryRef = useRef<HTMLDivElement>(null);
  const compatible = capability.support_level === 'compatible_structured';
  const editable = !compatible || acceptedCompatible;

  const loadCanonical = useCallback(async (signal?: AbortSignal) => {
    setLoad({ status: 'loading', snapshot: null, error: null });
    try {
      const snapshot = await client.getCanonical(signal);
      if (signal?.aborted) return;
      const document = parseCanonicalDocument(snapshot);
      setLoad({ status: 'ready', snapshot, error: null });
      setDraft(document);
      setJSONDrafts(initialJSONDrafts(descriptors, facts, document));
      setNumberDrafts(initialNumberDrafts(descriptors, facts, document));
      setDirtyFields({});
      setErrors({});
      setConflict(false);
      setMessage('');
    } catch (error) {
      if (signal?.aborted || (error instanceof DOMException && error.name === 'AbortError')) return;
      setLoad({ status: 'error', snapshot: null, error });
    }
  }, [client, descriptors, facts]);

  useEffect(() => {
    const controller = new AbortController();
    void loadCanonical(controller.signal);
    return () => controller.abort();
  }, [capability.pin?.manifest_sha256, exactVersion, loadCanonical]);

  useEffect(() => {
    if (Object.keys(errors).length > 0) errorSummaryRef.current?.focus();
  }, [errors]);

  function updateValue(descriptorID: string, pointer: string, value: unknown) {
    if (draft === null) return;
    try {
      setDraft(documentWithValue(draft, pointer, value));
      setDirtyFields((current) => ({ ...current, [descriptorID]: true }));
      setErrors((current) => {
        const next = { ...current };
        delete next[descriptorID];
        return next;
      });
    } catch (error) {
      setErrors((current) => ({ ...current, [descriptorID]: describeRequestError(error) }));
    }
    setMessage('');
    setConflict(false);
  }

  function updateUnset(descriptorID: string, pointer: string) {
    if (draft === null) return;
    try {
      if (valueAtPointer(draft, pointer) !== undefined) {
        setDraft(documentWithoutValue(draft, pointer));
      }
      setDirtyFields((current) => ({ ...current, [descriptorID]: true }));
      setErrors((current) => {
        const next = { ...current };
        delete next[descriptorID];
        return next;
      });
    } catch (error) {
      setErrors((current) => ({ ...current, [descriptorID]: describeRequestError(error) }));
    }
    setMessage('');
    setConflict(false);
  }

  function controlFor(
    descriptor: CapabilityUIDescriptor,
    fact: CapabilitySemanticFact,
  ): ReactNode {
    if (draft === null) return null;
    const id = descriptorInputID(descriptor);
    const value = valueAtPointer(draft, fact.canonical_path);
    const describedBy = [descriptor.help ? `${id}-help` : '', errors[descriptor.id] ? `${id}-error` : '']
      .filter(Boolean)
      .join(' ') || undefined;
    const common = {
      'aria-describedby': describedBy,
      'aria-invalid': errors[descriptor.id] !== undefined,
      'disabled': !editable || isSaving,
      id,
    };

    let control: ReactNode;
    switch (descriptor.kind) {
      case 'text':
        control = <input {...common} onChange={(event) => updateValue(descriptor.id, fact.canonical_path, event.target.value)} type='text' value={typeof value === 'string' ? value : ''} />;
        break;
      case 'number':
        control = (
          <input {...common} inputMode='decimal' onChange={(event) => {
            const raw = event.target.value;
            setNumberDrafts((current) => ({ ...current, [descriptor.id]: raw }));
            setDirtyFields((current) => ({ ...current, [descriptor.id]: true }));
            setMessage('');
            setConflict(false);
            if (raw === '') {
              updateUnset(descriptor.id, fact.canonical_path);
              return;
            }
            try {
              const parsed = parseLosslessJSON(raw);
              if (isLosslessNumber(parsed)) updateValue(descriptor.id, fact.canonical_path, parsed);
            } catch {
              // Intermediate number input is validated when the form is saved.
            }
          }} type='text' value={numberDrafts[descriptor.id] ?? ''} />
        );
        break;
      case 'boolean':
        return (
          <div className='capability-field capability-field--boolean' key={descriptor.id}>
            <label className='check-field' htmlFor={id}>
              <input {...common} checked={value === true} onChange={(event) => updateValue(descriptor.id, fact.canonical_path, event.target.checked)} type='checkbox' />
              <span>
                <strong>{descriptor.label}</strong>
                {descriptor.help ? <small id={`${id}-help`}>{descriptor.help}</small> : null}
              </span>
            </label>
            <Classification fact={fact} />
            {errors[descriptor.id] ? <span className='field-error' id={`${id}-error`}>{errors[descriptor.id]}</span> : null}
          </div>
        );
      case 'select':
        control = (
          <select {...common} onChange={(event: ChangeEvent<HTMLSelectElement>) => updateValue(descriptor.id, fact.canonical_path, event.target.value)} value={typeof value === 'string' ? value : ''}>
            <option value=''>Select a value</option>
            {descriptor.options?.map((option) => (
              <option key={option.value} value={option.value}>{option.label}</option>
            ))}
          </select>
        );
        break;
      case 'json':
        control = (
          <textarea {...common} className='data-editor' onChange={(event) => {
            const raw = event.target.value;
            setJSONDrafts((current) => ({ ...current, [descriptor.id]: raw }));
            setDirtyFields((current) => ({ ...current, [descriptor.id]: true }));
            setMessage('');
            setConflict(false);
            if (raw === '') {
              updateUnset(descriptor.id, fact.canonical_path);
              return;
            }
            try {
              updateValue(descriptor.id, fact.canonical_path, parseLosslessJSON(raw));
            } catch {
              // Intermediate JSON input is validated when the form is saved.
            }
          }} rows={7} spellCheck={false} value={jsonDrafts[descriptor.id] ?? ''} />
        );
        break;
      default:
        return null;
    }
    return (
      <div className='capability-field field-group' key={descriptor.id}>
        <div className='capability-field__label'>
          <label htmlFor={id}>{descriptor.label}</label>
          <Classification fact={fact} />
        </div>
        {control}
        {descriptor.help ? <span id={`${id}-help`}>{descriptor.help}</span> : null}
        {errors[descriptor.id] ? <span className='field-error' id={`${id}-error`}>{errors[descriptor.id]}</span> : null}
      </div>
    );
  }

  async function save(event: React.FormEvent) {
    event.preventDefault();
    if (draft === null || load.status !== 'ready' || !editable) return;
    const baseDocument = parseCanonicalDocument(load.snapshot);
    let validatedDocument = cloneCanonicalDocument(baseDocument);
    const changes: CanonicalChange[] = [];
    const changedPaths = new Set<string>();
    const nextErrors: Record<string, string> = Object.fromEntries(
      Object.entries(errors).filter(([id]) => {
        if (id === '_form') return false;
        const descriptor = descriptors.find((candidate) => candidate.id === id);
        return descriptor !== undefined && isVisible(descriptor, draft);
      }),
    );
    for (const descriptor of descriptors) {
      if (descriptor.kind === 'group' || !isVisible(descriptor, draft)) continue;
      const fact = facts.get(descriptor.fact_id);
      if (fact === undefined) continue;
      if (!dirtyFields[descriptor.id]) continue;
      if (changedPaths.has(fact.canonical_path)) {
        nextErrors[descriptor.id] = `The pinned manifest maps more than one edited control to ${fact.canonical_path}.`;
        continue;
      }

      let change: CanonicalChange | undefined;
      let parsedValue: unknown;
      if (descriptor.kind === 'number') {
        const raw = numberDrafts[descriptor.id] ?? '';
        if (raw === '') {
          if (valueAtPointer(baseDocument, fact.canonical_path) !== undefined) {
            change = { op: 'unset', path: fact.canonical_path };
          }
        } else {
          try {
            parsedValue = parseLosslessJSON(raw);
            if (!isLosslessNumber(parsedValue)) throw new Error('not a JSON number');
            const valueJSON = stringifyLosslessJSON(parsedValue);
            if (valueJSON === undefined) throw new Error('number cannot be encoded');
            change = { op: 'set', path: fact.canonical_path, value_json: valueJSON };
          } catch {
            nextErrors[descriptor.id] = 'Enter one valid JSON number.';
          }
        }
      } else if (descriptor.kind === 'json') {
        const raw = jsonDrafts[descriptor.id] ?? '';
        if (raw === '') {
          if (valueAtPointer(baseDocument, fact.canonical_path) !== undefined) {
            change = { op: 'unset', path: fact.canonical_path };
          }
        } else {
          try {
            parsedValue = parseLosslessJSON(raw);
            const valueJSON = stringifyLosslessJSON(parsedValue);
            if (valueJSON === undefined) throw new Error('JSON value cannot be encoded');
            change = { op: 'set', path: fact.canonical_path, value_json: valueJSON };
          } catch {
            nextErrors[descriptor.id] = 'Enter one valid JSON value.';
          }
        }
      } else {
        parsedValue = valueAtPointer(draft, fact.canonical_path);
        const valueJSON = stringifyLosslessJSON(parsedValue);
        if (valueJSON === undefined) {
          nextErrors[descriptor.id] = 'The edited value cannot be encoded as JSON.';
        } else {
          change = { op: 'set', path: fact.canonical_path, value_json: valueJSON };
        }
      }

      if (change !== undefined && nextErrors[descriptor.id] === undefined) {
        try {
          validatedDocument = change.op === 'unset'
            ? documentWithoutValue(validatedDocument, change.path)
            : documentWithValue(
                validatedDocument,
                change.path,
                parsedValue ?? parseLosslessJSON(change.value_json),
              );
          changes.push(change);
          changedPaths.add(change.path);
        } catch (error) {
          nextErrors[descriptor.id] = describeRequestError(error);
        }
      }
    }
    if (Object.keys(nextErrors).length > 0) {
      setErrors(nextErrors);
      return;
    }
    if (changes.length === 0) {
      setErrors({});
      setDirtyFields({});
      setMessage('No canonical field changes to save.');
      setConflict(false);
      return;
    }

    setErrors({});
    setIsSaving(true);
    setMessage('');
    setConflict(false);
    try {
      const result = await client.patchCanonical(changes, load.snapshot.id);
      const savedDocument = parseCanonicalDocument(result.revision);
      setDraft(savedDocument);
      setLoad({ status: 'ready', snapshot: result.revision, error: null });
      setJSONDrafts(initialJSONDrafts(descriptors, facts, savedDocument));
      setNumberDrafts(initialNumberDrafts(descriptors, facts, savedDocument));
      setDirtyFields({});
      setMessage(result.no_change ? `Revision #${result.revision.sequence} is unchanged.` : `Saved revision #${result.revision.sequence}.`);
      await onSaved();
    } catch (error) {
      if (error instanceof ApiRequestError && error.status === 412) {
        setConflict(true);
        setErrors({
          _form: 'The canonical revision changed on the server. Your draft is preserved; reload and review the current revision before saving again.',
        });
      } else {
        setErrors({ _form: describeRequestError(error) });
      }
    } finally {
      setIsSaving(false);
    }
  }

  return (
    <section className='capability-form' aria-labelledby='capability-form-title'>
      <div className='section-heading'>
        <div>
          <p className='eyebrow'>Pinned manifest presentation</p>
          <h2 id='capability-form-title'>Versioned canonical controls</h2>
        </div>
        <span>
          sing-box
          {' '}
          {exactVersion}
        </span>
      </div>
      <p className='capability-form__intro'>These built-in controls come only from the validated, exact pinned manifest. Saving applies lossless pointer changes with compare-and-swap; it does not render or apply.</p>

      {compatible
        ? (
            <label className='check-field capability-acceptance'>
              <input checked={acceptedCompatible} onChange={(event) => setAcceptedCompatible(event.target.checked)} type='checkbox' />
              <span>
                <strong>
                  Accept compatible controls for sing-box
                  {' '}
                  {exactVersion}
                </strong>
                <small>
                  This manifest declares compatible rather than native structured support.
                  Acceptance is required before any field can be edited or saved.
                </small>
              </span>
            </label>
          )
        : null}
      {load.status === 'loading' ? <div className='inline-loading' aria-busy='true'>Loading canonical snapshot…</div> : null}
      {load.status === 'error' ? <ErrorNotice error={load.error} title='Could not load canonical controls' /> : null}
      {load.status === 'ready' && draft !== null
        ? (
            <form onSubmit={(event) => void save(event)}>
              {Object.keys(errors).length > 0
                ? (
                    <div className='form-error capability-error-summary' ref={errorSummaryRef} role='alert' tabIndex={-1}>
                      <strong>{conflict ? 'Revision conflict' : 'Review the highlighted fields'}</strong>
                      <span>{errors._form ?? 'Correct the following fields before saving.'}</span>
                      {Object.entries(errors).some(([id]) => id !== '_form')
                        ? (
                            <ul>
                              {Object.entries(errors).filter(([id]) => id !== '_form').map(([id, message]) => {
                                const descriptor = descriptors.find((candidate) => candidate.id === id);
                                return (
                                  <li key={id}>
                                    <a href={`#${descriptor ? descriptorInputID(descriptor) : ''}`}>
                                      {descriptor?.label ?? id}
                                      :
                                      {' '}
                                      {message}
                                    </a>
                                  </li>
                                );
                              })}
                            </ul>
                          )
                        : null}
                    </div>
                  )
                : null}
              <div className='capability-fields'>
                {descriptors.map((descriptor) => {
                  if (!isVisible(descriptor, draft)) return null;
                  if (descriptor.kind === 'group') {
                    return (
                      <div className='capability-group' key={descriptor.id}>
                        <h3>{descriptor.label}</h3>
                        {descriptor.help ? <p>{descriptor.help}</p> : null}
                      </div>
                    );
                  }
                  const fact = facts.get(descriptor.fact_id);
                  return fact === undefined ? null : controlFor(descriptor, fact);
                })}
              </div>
              <div className='inline-actions capability-form__actions'>
                <button className='button button--primary' disabled={!editable || isSaving} type='submit'>{isSaving ? 'Saving…' : 'Save canonical revision'}</button>
                {conflict ? <button className='button button--warning' onClick={() => void loadCanonical()} type='button'>Reload current revision</button> : null}
                <small>
                  Base
                  {' '}
                  {load.snapshot.id.slice(0, 14)}
                  …
                </small>
              </div>
              {message
                ? (
                    <div className='notice notice--success' role='status'>
                      <strong>Canonical revision saved</strong>
                      <p>{message}</p>
                    </div>
                  )
                : null}
            </form>
          )
        : null}
    </section>
  );
}

function Classification({ fact }: { fact: CapabilitySemanticFact }) {
  return <span className={`capability-classification capability-classification--${fact.classification}`}>{classificationLabels[fact.classification]}</span>;
}
