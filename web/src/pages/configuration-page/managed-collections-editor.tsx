import type { RJSFSchema } from '@rjsf/utils';

import { CSS } from '@dnd-kit/utilities';
import { useTranslation } from 'react-i18next';
import { isLosslessNumber } from 'lossless-json';
import { useEffect, useMemo, useRef, useState } from 'react';
import {
  Braces,
  CircleAlert,
  GripVertical,
  MoreHorizontal,
  Pencil,
  Plus,
  Trash2,
  Wrench,
} from 'lucide-react';
import {
  closestCenter,
  DndContext,
  KeyboardSensor,
  PointerSensor,
  TouchSensor,
  useSensor,
  useSensors,
} from '@dnd-kit/core';
import {
  arrayMove,
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
} from '@dnd-kit/sortable';

import type { JsonObject } from '@/api/api-client';
import type { ReviewedSchemaResolution } from '@/schemas/resolve-reviewed-schema';

import { Badge } from '@/components/ui/badge';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { toast } from '@/components/ui/toast-manager';
import { useApiClient } from '@/api/api-client-context';
import { describeRequestError } from '@/components/error-notice';
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
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

interface SortableEntryCardProps {
  index: number;
  entry: EntryView;
  disabled: boolean;
  onEdit: () => void;
  onDelete: () => void;
  onRepair: () => void;
}

function SortableEntryCard({
  disabled,
  entry,
  index,
  onDelete,
  onEdit,
  onRepair,
}: SortableEntryCardProps) {
  const { t } = useTranslation();
  const sortable = useSortable({ disabled, id: `${entry.id}:${index}` });
  return (
    <article
      className='managed-node-card'
      ref={sortable.setNodeRef}
      style={{
        transform: CSS.Transform.toString(sortable.transform),
        transition: sortable.transition,
      }}
    >
      <Button
        {...sortable.attributes}
        {...sortable.listeners}
        aria-label={t('configuration.managed.reorder', { name: entry.tag })}
        className='managed-node-card__handle'
        disabled={disabled}
        size='icon-sm'
        type='button'
        variant='ghost'
      >
        <GripVertical aria-hidden='true' />
      </Button>
      <button
        className='managed-node-card__identity'
        disabled={disabled}
        onClick={onEdit}
        type='button'
      >
        <strong>{entry.tag}</strong>
        <span>{entry.type === 'untyped' ? t('configuration.managed.untyped') : entry.type}</span>
      </button>
      <div className='managed-node-card__badges'>
        {!entry.valid
          ? (
              <Badge variant='destructive'>{t('configuration.managed.needsRepair')}</Badge>
            )
          : null}
        {summaryBadges(entry.value).map((badge) => (
          <Badge key={badge} variant='outline'>
            {badge}
          </Badge>
        ))}
      </div>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={(
            <Button
              aria-label={t('configuration.managed.actions', { name: entry.tag })}
              disabled={disabled}
              size='icon-sm'
              variant='ghost'
            />
          )}
        >
          <MoreHorizontal aria-hidden='true' />
        </DropdownMenuTrigger>
        <DropdownMenuContent align='end'>
          <DropdownMenuGroup>
            <DropdownMenuItem onClick={onEdit}>
              <Pencil aria-hidden='true' />
              {t('common.edit')}
            </DropdownMenuItem>
            {!entry.valid
              ? (
                  <DropdownMenuItem onClick={onRepair}>
                    <Wrench aria-hidden='true' />
                    {t('configuration.managed.repair')}
                  </DropdownMenuItem>
                )
              : null}
            <DropdownMenuItem onClick={onDelete} variant='destructive'>
              <Trash2 aria-hidden='true' />
              {t('common.delete')}
            </DropdownMenuItem>
          </DropdownMenuGroup>
        </DropdownMenuContent>
      </DropdownMenu>
    </article>
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
  const [editingIndex, setEditingIndex] = useState<number | null>(() => {
    const index = linkedTag === undefined ? -1 : items.findIndex((item) => item.tag === linkedTag);
    return index < 0 ? null : index;
  });
  const [deletingIndex, setDeletingIndex] = useState<number | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const types = activeSchema === undefined ? [] : protocolTypes(activeSchema, resolution.schema);
  const [newID, setNewID] = useState('');
  const [newType, setNewType] = useState(() => defaultProtocolType(types));
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 6 } }),
    useSensor(TouchSensor, { activationConstraint: { delay: 180, tolerance: 5 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );
  const usedIDs = useMemo(() => new Set(views.map((entry) => entry.id)), [views]);
  const newIDValid = newID !== '' && newID.trim() === newID && !usedIDs.has(newID);

  function replace(next: JsonObject[]) {
    if (disabled) return;
    onChange((current) => ({
      ...current,
      [activeCollection]: next,
    }));
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
    setCreateOpen(true);
  }

  async function create() {
    if (!newIDValid || newType === '' || creating || disabled) return;
    const controller = new AbortController();
    createRequestRef.current = controller;
    setCreating(true);
    try {
      const initial
        = activeCollection === 'inbounds'
          ? await client.newInboundDefaults(newType, controller.signal)
          : { type: newType };
      if (controller.signal.aborted) return;
      replace([...items, { ...initial, tag: newID }]);
      setCreateOpen(false);
      setEditingIndex(items.length);
    } catch (error) {
      if (!controller.signal.aborted) toast.add({ title: describeRequestError(error), type: 'error' });
    } finally {
      if (!controller.signal.aborted) setCreating(false);
    }
  }

  return (
    <div className='managed-editor'>
      <div className='managed-editor__toolbar'>
        {selectedCollection === undefined
          ? (
              <div
                aria-label={t('configuration.managed.collections')}
                className='managed-editor__collections'
                role='tablist'
              >
                {collectionSchemas.map(([name, schema]) => (
                  <Button
                    aria-selected={name === activeCollection}
                    key={name}
                    onClick={() => setChosenCollection(name)}
                    role='tab'
                    size='sm'
                    type='button'
                    variant={name === activeCollection ? 'secondary' : 'ghost'}
                  >
                    {schemaLabel(schema, i18n.language, t(`configuration.managed.collection.${name}`))}
                    <Badge variant='outline'>{entries(draft[name]).length}</Badge>
                  </Button>
                ))}
              </div>
            )
          : (
              <span />
            )}
        <Button
          aria-label={t('configuration.managed.add')}
          disabled={disabled || types.length === 0}
          onClick={openCreate}
          size='icon'
          type='button'
          variant='ghost'
        >
          <Plus aria-hidden='true' data-icon='inline-start' />
        </Button>
      </div>

      {activeSchema === undefined || itemSchema === null
        ? (
            <div className='configuration-empty-copy' role='status'>
              <CircleAlert aria-hidden='true' />
              {t('configuration.schema.noManagedFields')}
            </div>
          )
        : items.length === 0
          ? (
              <div className='configuration-empty-copy'>
                <strong>{t('configuration.managed.emptyTitle')}</strong>
              </div>
            )
          : (
              <DndContext
                collisionDetection={closestCenter}
                onDragEnd={({ active, over }) => {
                  if (disabled || over === null || active.id === over.id) return;
                  const from = views.findIndex((entry, index) => `${entry.id}:${index}` === active.id);
                  const to = views.findIndex((entry, index) => `${entry.id}:${index}` === over.id);
                  if (from >= 0 && to >= 0) replace(arrayMove(items, from, to));
                }}
                sensors={sensors}
              >
                <SortableContext
                  items={views.map((entry, index) => `${entry.id}:${index}`)}
                  strategy={verticalListSortingStrategy}
                >
                  <div className='managed-node-list'>
                    {views.map((entry, index) => (
                      <SortableEntryCard
                        disabled={disabled}
                        entry={entry}
                        index={index}
                        key={`${entry.id}:${encodeCanonicalValue(entry.value)}`}
                        onDelete={() => setDeletingIndex(index)}
                        onEdit={() => setEditingIndex(index)}
                        onRepair={() => repair(index)}
                      />
                    ))}
                  </div>
                </SortableContext>
              </DndContext>
            )}

      <Dialog onOpenChange={(open) => !open && setEditingIndex(null)} open={editingIndex !== null}>
        <DialogContent className='configuration-entity-dialog'>
          <DialogHeader>
            <DialogTitle>{editingIndex === null ? '' : views[editingIndex]?.tag}</DialogTitle>
            <DialogDescription>{t('configuration.managed.dialogDescription')}</DialogDescription>
          </DialogHeader>
          {editingIndex !== null && itemSchema !== null && views[editingIndex] !== undefined
            ? (
                <Tabs defaultValue='form'>
                  <TabsList>
                    <TabsTrigger value='form'>
                      <Pencil aria-hidden='true' />
                      {t('configuration.managed.form')}
                    </TabsTrigger>
                    <TabsTrigger value='json'>
                      <Braces aria-hidden='true' />
                      {t('configuration.managed.json')}
                    </TabsTrigger>
                  </TabsList>
                  <TabsContent value='form'>
                    <div className='configuration-entity-form-scroll'>
                      <SchemaSectionForm
                        basePointer={`/${activeCollection}/${editingIndex}`}
                        data={items[editingIndex]}
                        disabled={disabled || !views[editingIndex].valid}
                        onChange={onChange}
                        protectedPaths={[`/${activeCollection}/${editingIndex}/type`]}
                        resolution={resolution}
                        schema={itemSchema}
                        uiSchema={{
                          ...uiSchemaFromPanel(
                            itemSchema,
                            ['type'],
                            resolution.schema,
                            items[editingIndex],
                          ),
                          type: { 'ui:readonly': true },
                        }}
                      />
                    </div>
                  </TabsContent>
                  <TabsContent value='json'>
                    <pre className='configuration-entity-json'>
                      {encodeCanonicalValue(items[editingIndex], 2)}
                    </pre>
                  </TabsContent>
                </Tabs>
              )
            : null}
        </DialogContent>
      </Dialog>

      <Dialog
        onOpenChange={(open) => {
          if (!open) {
            createRequestRef.current?.abort();
            setCreating(false);
          }
          setCreateOpen(open);
        }}
        open={createOpen}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('configuration.managed.createTitle')}</DialogTitle>
            <DialogDescription>{t('configuration.managed.createDescription')}</DialogDescription>
          </DialogHeader>
          <Field data-invalid={newID !== '' && !newIDValid ? true : undefined}>
            <FieldLabel htmlFor='managed-new-id'>{t('configuration.managed.panelID')}</FieldLabel>
            <Input
              disabled={disabled || creating}
              id='managed-new-id'
              onChange={(event) => setNewID(event.currentTarget.value)}
              value={newID}
            />
            <FieldDescription>{t('configuration.managed.panelIDHelp')}</FieldDescription>
          </Field>
          <Field>
            <FieldLabel htmlFor='managed-new-type'>
              {t('configuration.managed.protocol')}
            </FieldLabel>
            <Select
              items={types.map((type) => ({ label: type, value: type }))}
              disabled={disabled || creating}
              onValueChange={(value) => setNewType(value ?? '')}
              value={newType}
            >
              <SelectTrigger className='w-full' id='managed-new-type'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  {types.map((type) => (
                    <SelectItem key={type} value={type}>
                      {type}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          </Field>
          <Button
            disabled={disabled || creating || !newIDValid || newType === ''}
            onClick={create}
            type='button'
          >
            <Plus aria-hidden='true' data-icon='inline-start' />
            {t('common.create')}
          </Button>
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
