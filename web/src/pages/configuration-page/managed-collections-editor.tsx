import type { RJSFSchema } from '@rjsf/utils';

import { useTranslation } from 'react-i18next';
import { isLosslessNumber } from 'lossless-json';
import { useEffect, useMemo, useRef, useState } from 'react';
import {
  ArrowDown,
  ArrowUp,
  CircleAlert,
  Plus,
  Trash2,
  Wrench,
} from 'lucide-react';

import type { JsonObject } from '@/api/api-client';
import type { ReviewedSchemaResolution } from '@/schemas/resolve-reviewed-schema';

import { Badge } from '@/components/ui/badge';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { toast } from '@/components/ui/toast-manager';
import { useApiClient } from '@/api/api-client-context';
import { describeRequestError } from '@/components/error-notice';
import { Field, FieldError, FieldGroup, FieldLabel } from '@/components/ui/field';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  Combobox,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxGroup,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
  ComboboxTrigger,
} from '@/components/ui/combobox';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';

import type { CanonicalDraft } from './use-canonical-configuration';

import { SchemaDialogLayout } from './rjsf-shadcn-theme';
import { SchemaSectionForm } from './schema-section-form';
import { encodeCanonicalValue } from './use-canonical-configuration';
import {
  collectionItemSchema,
  panelMetadata,
  schemaDiscriminatorValues,
  schemaProperties,
  uiSchemaFromPanel,
} from './schema-ui';

type ManagedCollection = 'endpoints' | 'inbounds' | 'outbounds' | 'services';

interface ManagedCollectionsEditorProps {
  disabled?: boolean;
  linkedTag?: string;
  draft: CanonicalDraft;
  resolution: ReviewedSchemaResolution;
  selectedCollection?: ManagedCollection;
  onChange: (change: (draft: CanonicalDraft) => CanonicalDraft) => void;
}

interface EntryView {
  id: string;
  tag: string;
  type: string;
  valid: boolean;
  value: JsonObject;
}

function object(value: unknown): JsonObject | null {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
    ? (value as JsonObject)
    : null;
}

function entries(value: unknown): JsonObject[] {
  return Array.isArray(value) ? value.map((item) => object(item) ?? {}) : [];
}

function entryView(value: JsonObject, index: number): EntryView {
  const identity = [value.tag, value.id, value.name].find(
    (candidate) => typeof candidate === 'string' && candidate !== '',
  );
  const id = typeof identity === 'string' ? identity : `entry-${index + 1}`;
  const type = typeof value.type === 'string' && value.type !== '' ? value.type : 'untyped';
  const tag = typeof value.tag === 'string' && value.tag !== '' ? value.tag : id;
  return {
    id,
    type,
    tag,
    valid: type !== 'untyped',
    value,
  };
}

function nestedString(value: JsonObject, path: string[]): string {
  let current: unknown = value;
  for (const key of path) {
    current = object(current)?.[key];
  }
  return typeof current === 'string' ? current : '';
}

function summaryBadges(value: JsonObject): string[] {
  const host = [value.server, value.listen].find((candidate) => typeof candidate === 'string');
  const port = [value.server_port, value.listen_port].find(
    (candidate) =>
      typeof candidate === 'number' || typeof candidate === 'string' || isLosslessNumber(candidate),
  );
  const result = [
    typeof host === 'string' && host !== ''
      ? `${host}${port === undefined ? '' : `:${String(port)}`}`
      : '',
    nestedString(value, ['tls', 'server_name']),
    nestedString(value, ['transport', 'type']),
    nestedString(value, ['obfs', 'type']),
  ].filter((item) => item !== '');
  return result.slice(0, 3);
}

function schemaLabel(schema: RJSFSchema, language: string, fallback: string): string {
  const value = schema['x-panel'];
  if (value !== null && typeof value === 'object' && !Array.isArray(value)) {
    const labels = (value as { label?: Record<string, string> }).label;
    return labels?.[language] ?? labels?.en ?? fallback;
  }
  return fallback;
}

function protocolTypes(schema: RJSFSchema, root: RJSFSchema): string[] {
  const item = collectionItemSchema(schema, root);
  return item === null ? [] : schemaDiscriminatorValues(item, root, 'type');
}

function defaultProtocolType(types: string[]): string {
  return types.includes('mixed') ? 'mixed' : (types[0] ?? '');
}

function nextIdentifier(
  items: JsonObject[],
  collection: ManagedCollection,
  unavailableMessage: string,
): string {
  const used = new Set(items.map((item, index) => entryView(item, index).id));
  const prefix = collection === 'services' ? 'service' : collection.slice(0, -1);
  for (let suffix = 1; suffix <= 10_000; suffix += 1) {
    const candidate = `${prefix}-${suffix}`;
    if (!used.has(candidate)) return candidate;
  }
  throw new Error(unavailableMessage);
}

interface EntryRowProps {
  entry: EntryView;
  disabled: boolean;
  canMoveUp: boolean;
  onEdit: () => void;
  canMoveDown: boolean;
  onDelete: () => void;
  onMoveUp: () => void;
  onRepair: () => void;
  onMoveDown: () => void;
}

function EntryRow({
  disabled,
  entry,
  canMoveUp,
  canMoveDown,
  onMoveUp,
  onMoveDown,
  onDelete,
  onEdit,
  onRepair,
}: EntryRowProps) {
  const { t } = useTranslation();
  return (
    <TableRow>
      <TableCell><span className='configuration-list__text' title={entry.tag}>{entry.tag}</span></TableCell>
      <TableCell className='configuration-list__secondary'><span className='configuration-list__text'>{entry.type === 'untyped' ? t('configuration.managed.untyped') : entry.type}</span></TableCell>
      <TableCell className='configuration-list__secondary'>
        <div className='configuration-list__summary'>
          {!entry.valid
            ? (
                <Badge variant='destructive'>{t('configuration.managed.needsRepair')}</Badge>
              )
            : null}
          {summaryBadges(entry.value).map((badge) => (
            <span className='configuration-list__text' key={badge} title={badge}>{badge}</span>
          ))}
          {entry.valid && summaryBadges(entry.value).length === 0 ? '—' : null}
        </div>
      </TableCell>
      <TableCell>
        <div className='configuration-list__actions'>
          <Button disabled={disabled} onClick={onEdit} size='sm' type='button' variant='ghost'>{t('common.edit')}</Button>
          <Button aria-label={t('configuration.general.moveUp', { name: entry.tag })} title={t('common.moveUp')} disabled={disabled || !canMoveUp} onClick={onMoveUp} size='icon-sm' type='button' variant='ghost'><ArrowUp aria-hidden='true' /></Button>
          <Button aria-label={t('configuration.general.moveDown', { name: entry.tag })} title={t('common.moveDown')} disabled={disabled || !canMoveDown} onClick={onMoveDown} size='icon-sm' type='button' variant='ghost'><ArrowDown aria-hidden='true' /></Button>
          {!entry.valid && (
            <Button aria-label={t('configuration.managed.repair')} title={t('configuration.managed.repair')} disabled={disabled} onClick={onRepair} size='icon-sm' type='button' variant='ghost'><Wrench aria-hidden='true' /></Button>
          )}
          <Button aria-label={t('common.delete')} title={t('common.delete')} className='configuration-list__remove' disabled={disabled} onClick={onDelete} size='icon-sm' type='button' variant='ghost'><Trash2 aria-hidden='true' /></Button>
        </div>
      </TableCell>
    </TableRow>
  );
}

export function ManagedCollectionsEditor({
  disabled = false,
  draft,
  linkedTag,
  onChange,
  resolution,
  selectedCollection,
}: ManagedCollectionsEditorProps) {
  const { i18n, t } = useTranslation();
  const client = useApiClient();
  const createRequestRef = useRef<AbortController | null>(null);
  const [creating, setCreating] = useState(false);
  useEffect(() => () => createRequestRef.current?.abort(), []);
  const collectionSchemas = Object.entries(schemaProperties(resolution.schema, resolution.schema))
    .filter(
      (entry): entry is [ManagedCollection, RJSFSchema] =>
        panelMetadata(entry[1]).section === 'managed'
        && ['endpoints', 'inbounds', 'outbounds', 'services'].includes(entry[0]),
    )
    .sort(
      ([, left], [, right]) => (panelMetadata(left).order ?? 0) - (panelMetadata(right).order ?? 0),
    );
  const [chosenCollection, setChosenCollection] = useState<ManagedCollection>(
    collectionSchemas[0]?.[0] ?? 'inbounds',
  );
  const collection = selectedCollection ?? chosenCollection;
  const activeSchema
    = collectionSchemas.find(([name]) => name === collection)?.[1] ?? collectionSchemas[0]?.[1];
  const activeCollection
    = activeSchema === undefined
      ? collection
      : (collectionSchemas.find(([, schema]) => schema === activeSchema)?.[0] ?? collection);
  const itemSchema
    = activeSchema === undefined ? null : collectionItemSchema(activeSchema, resolution.schema);
  const items = entries(draft[activeCollection]);
  const views = items.map(entryView);
  const [editing, setEditing] = useState<{ index: number; draft: CanonicalDraft } | null>(() => {
    const index = linkedTag === undefined ? -1 : items.findIndex((item) => item.tag === linkedTag);
    return index < 0 ? null : { index, draft: { [activeCollection]: [items[index]] } };
  });
  const [editOpen, setEditOpen] = useState(editing !== null);
  const [deletingIndex, setDeletingIndex] = useState<number | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [pendingEntry, setPendingEntry] = useState<CanonicalDraft | null>(null);
  const types = activeSchema === undefined ? [] : protocolTypes(activeSchema, resolution.schema);
  const [newID, setNewID] = useState('');
  const [newType, setNewType] = useState(() => defaultProtocolType(types));
  const protocolAnchorRef = useRef<HTMLDivElement>(null);
  const newTypeValid = types.includes(newType);
  const usedIDs = useMemo(() => new Set(views.map((entry) => entry.id)), [views]);
  const newIDValid = newID !== '' && newID.trim() === newID && !usedIDs.has(newID);

  function replace(next: JsonObject[]) {
    if (disabled) return;
    onChange((current) => ({
      ...current,
      [activeCollection]: next,
    }));
  }

  function move(index: number, delta: -1 | 1) {
    const target = index + delta;
    if (disabled || index < 0 || index >= items.length || target < 0 || target >= items.length) return;
    const next = [...items];
    [next[index], next[target]] = [next[target], next[index]];
    replace(next);
  }

  function repair(index: number) {
    const next = [...items];
    const fallbackID = nextIdentifier(
      items.filter((_, itemIndex) => itemIndex !== index),
      activeCollection,
      t('configuration.managed.noAvailableIdentifier'),
    );
    const current = next[index];
    if ((typeof current.type !== 'string' || current.type === '') && types.length === 0) return;
    const currentID
      = typeof current.tag === 'string'
        && current.tag !== ''
        && !views.some((entry, itemIndex) => itemIndex !== index && entry.id === current.tag)
        ? current.tag
        : fallbackID;
    next[index] = {
      ...current,
      type:
        typeof current.type === 'string' && current.type !== ''
          ? current.type
          : defaultProtocolType(types),
      tag: typeof current.tag === 'string' && current.tag !== '' ? current.tag : currentID,
    };
    replace(next);
  }

  function openCreate() {
    setNewID(
      nextIdentifier(items, activeCollection, t('configuration.managed.noAvailableIdentifier')),
    );
    setNewType(defaultProtocolType(types));
    setCreating(false);
    setPendingEntry(null);
    setCreateOpen(true);
  }

  async function create() {
    if (!newIDValid || !newTypeValid || creating || disabled) return;
    const controller = new AbortController();
    createRequestRef.current = controller;
    setCreating(true);
    try {
      const initial
        = activeCollection === 'inbounds'
          ? await client.newInboundDefaults(newType, controller.signal)
          : { type: newType };
      if (controller.signal.aborted) return;
      setPendingEntry({ [activeCollection]: [{ ...initial, tag: newID }] });
    } catch (error) {
      if (!controller.signal.aborted) toast.add({ title: describeRequestError(error), type: 'error' });
    } finally {
      if (!controller.signal.aborted) setCreating(false);
    }
  }

  const identityField = (
    <Field className={pendingEntry === null ? undefined : 'schema-form__field'} data-invalid={createOpen && !newIDValid ? true : undefined}>
      <FieldLabel htmlFor='managed-new-id'>{t('configuration.managed.panelID')}</FieldLabel>
      <div className='schema-form__control'>
        <Input
          aria-invalid={createOpen && !newIDValid ? true : undefined}
          aria-describedby={createOpen && !newIDValid ? 'managed-new-id-error' : undefined}
          disabled={disabled || creating}
          id='managed-new-id'
          onChange={(event) => setNewID(event.currentTarget.value)}
          value={newID}
        />
      </div>
      {createOpen && !newIDValid ? <FieldError id='managed-new-id-error'>{t('configuration.managed.panelIDHelp')}</FieldError> : null}
    </Field>
  );

  return (
    <div className='managed-editor'>
      {selectedCollection === undefined && (
        <div className='managed-editor__collections' role='tablist' aria-label={t('configuration.managed.collections')}>
          {collectionSchemas.map(([name, schema]) => (
            <Button
              aria-selected={name === activeCollection} key={name}
              onClick={() => setChosenCollection(name)} role='tab' size='sm' type='button'
              variant={name === activeCollection ? 'secondary' : 'ghost'}
            >
              {schemaLabel(schema, i18n.language, t(`configuration.managed.collection.${name}`))}
            </Button>
          ))}
        </div>
      )}
      {activeSchema === undefined || itemSchema === null
        ? (
            <div className='configuration-empty-copy' role='status'>
              <CircleAlert aria-hidden='true' />
              {t('configuration.schema.noManagedFields')}
            </div>
          )
        : (
            <Table className='configuration-list' aria-label={t(`configuration.managed.collection.${activeCollection}`)}>
              <TableHeader>
                <TableRow>
                  <TableHead scope='col'>{t('configuration.fields.tag')}</TableHead>
                  <TableHead scope='col'>{t('configuration.fields.type')}</TableHead>
                  <TableHead scope='col'>{t('configuration.general.summary')}</TableHead>
                  <TableHead scope='col'>{t('common.actions')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {views.map((entry, index) => (
                  <EntryRow
                    disabled={disabled} entry={entry}
                    canMoveUp={index > 0} canMoveDown={index < views.length - 1}
                    onMoveUp={() => move(index, -1)} onMoveDown={() => move(index, 1)}
                    key={`${entry.id}:${encodeCanonicalValue(entry.value)}`}
                    onDelete={() => setDeletingIndex(index)}
                    onEdit={() => {
                      setEditing({ index, draft: { [activeCollection]: [items[index]] } });
                      setEditOpen(true);
                    }}
                    onRepair={() => repair(index)}
                  />
                ))}
                <TableRow className='configuration-list__add-row'>
                  <TableCell colSpan={4}>
                    <Button
                      aria-label={t('configuration.managed.add')}
                      className='configuration-list__add'
                      disabled={disabled || types.length === 0} onClick={openCreate}
                      size='content' type='button' variant='outline'
                    >
                      <Plus aria-hidden='true' data-icon='inline-start' />
                      {t('common.add')}
                    </Button>
                  </TableCell>
                </TableRow>
              </TableBody>
            </Table>
          )}

      <Dialog open={editOpen} onOpenChange={setEditOpen} onOpenChangeComplete={(open) => {
        if (!open) setEditing(null);
      }}>
        <DialogContent className='configuration-entry-dialog'>
          <DialogHeader>
            <DialogTitle>{t('configuration.general.editEntry')}</DialogTitle>
            <DialogDescription className='sr-only'>{t('configuration.general.editDescription')}</DialogDescription>
          </DialogHeader>
          {editing !== null && itemSchema !== null && views[editing.index] !== undefined
            ? (
                <div className='configuration-entry-dialog__body'>
                  <SchemaSectionForm
                    dialogLayout
                    dialogJsonPreview={<pre className='configuration-entity-json'>{encodeCanonicalValue(entries(editing.draft[activeCollection])[0], 2)}</pre>}
                    basePointer={`/${activeCollection}/0`}
                    data={entries(editing.draft[activeCollection])[0]}
                    disabled={disabled || !views[editing.index].valid}
                    onChange={(change) => setEditing((current) => current === null
                      ? null
                      : { ...current, draft: change(current.draft) })}
                    protectedPaths={[`/${activeCollection}/0/type`]}
                    resolution={resolution}
                    schema={itemSchema}
                    uiSchema={{
                      ...uiSchemaFromPanel(itemSchema, ['type'], resolution.schema, entries(editing.draft[activeCollection])[0]),
                      type: { 'ui:readonly': true },
                    }}
                  />

                </div>
              )
            : null}
          <DialogFooter>
            <DialogClose render={<Button type='button' />}>{t('common.cancel')}</DialogClose>
            <Button disabled={disabled || editing === null || !views[editing.index]?.valid} type='button' onClick={() => {
              if (editing === null) return;
              const updated = entries(editing.draft[activeCollection])[0];
              replace(items.map((item, index) => index === editing.index ? updated : item));
              setEditOpen(false);
            }}>
              {t('configuration.general.saveChanges')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog
        onOpenChange={(open) => {
          if (!open) {
            createRequestRef.current?.abort();
          }
          setCreateOpen(open);
        }}
        onOpenChangeComplete={(open) => {
          if (!open) {
            setCreating(false);
            setPendingEntry(null);
          }
        }}
        open={createOpen}
      >
        <DialogContent className='configuration-entry-dialog'>
          <DialogHeader>
            <DialogTitle>{t('configuration.managed.createTitle')}</DialogTitle>
            <DialogDescription className='sr-only'>{t('configuration.managed.createDescription')}</DialogDescription>
          </DialogHeader>
          <div className='configuration-entry-dialog__body'>
            {pendingEntry === null
              ? (
                  <SchemaDialogLayout schema={{ type: 'object' }} data={{}}>
                    <FieldGroup className='configuration-entry-dialog__identity'>
                      {identityField}
                      <Field>
                        <FieldLabel htmlFor='managed-new-type'>
                          {t('configuration.managed.protocol')}
                        </FieldLabel>
                        <Combobox
                          items={types}
                          autoHighlight
                          openOnInputClick={false}
                          disabled={disabled || creating}
                          onValueChange={(value) => setNewType(value ?? '')}
                          onInputValueChange={setNewType}
                          inputValue={newType}
                          value={newTypeValid ? newType : null}
                        >
                          <div className='flex min-w-0 items-center gap-2' ref={protocolAnchorRef}>
                            <ComboboxInput className='min-w-0 flex-1' id='managed-new-type'
                              disabled={disabled || creating} showTrigger={false} />
                            <Button aria-label={t('configuration.managed.chooseProtocol')}
                              disabled={disabled || creating} size='icon' type='button'
                              variant='secondary' render={<ComboboxTrigger />} />
                          </div>
                          <ComboboxContent anchor={protocolAnchorRef} side='bottom' collisionPadding={12}
                            collisionAvoidance={{ side: 'none', align: 'shift', fallbackAxisSide: 'none' }}>
                            <ComboboxEmpty>{t('configuration.managed.noMatchingProtocol')}</ComboboxEmpty>
                            <ComboboxList>
                              {(type: string) => (
                                <ComboboxGroup key={type}>
                                  <ComboboxItem value={type}>{type}</ComboboxItem>
                                </ComboboxGroup>
                              )}
                            </ComboboxList>
                          </ComboboxContent>
                        </Combobox>
                      </Field>
                    </FieldGroup>
                  </SchemaDialogLayout>
                )
              : itemSchema !== null
                ? (
                    <SchemaSectionForm
                      dialogLayout
                      dialogPrimaryContent={(
                        <FieldGroup className='schema-form configuration-entry-dialog__identity'>
                          {identityField}
                        </FieldGroup>
                      )}
                      basePointer={`/${activeCollection}/0`}
                      data={entries(pendingEntry[activeCollection])[0]}
                      disabled={disabled}
                      onChange={(change) => setPendingEntry((current) => current === null ? null : change(current))}
                      protectedPaths={[`/${activeCollection}/0/type`, `/${activeCollection}/0/tag`]}
                      resolution={resolution}
                      schema={itemSchema}
                      uiSchema={{
                        ...uiSchemaFromPanel(
                          itemSchema, [], resolution.schema, entries(pendingEntry[activeCollection])[0],
                        ),
                        tag: { 'ui:widget': 'hidden' },
                        type: { 'ui:readonly': true },
                      }}
                    />
                  )
                : null}
          </div>
          <DialogFooter>
            <DialogClose render={<Button type='button' variant='secondary' />}>{t('common.cancel')}</DialogClose>
            <Button
              disabled={disabled || creating || (createOpen && !newIDValid) || !newTypeValid}
              onClick={() => {
                if (pendingEntry === null) {
                  void create();
                  return;
                }
                replace([...items, { ...entries(pendingEntry[activeCollection])[0], tag: newID }]);
                setCreateOpen(false);
              }}
              type='button'
            >
              {t(pendingEntry === null ? 'configuration.general.continue' : 'common.create')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <AlertDialog
        onOpenChange={(open) => !open && setDeletingIndex(null)}
        open={deletingIndex !== null}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('configuration.managed.deleteTitle')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t('configuration.managed.deleteDescription', {
                name: deletingIndex === null ? '' : views[deletingIndex]?.tag,
              })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction
              disabled={disabled}
              onClick={() => {
                if (deletingIndex !== null) replace(items.filter((_, index) => index !== deletingIndex));
                setDeletingIndex(null);
              }}
              variant='destructive'
            >
              {t('common.delete')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
