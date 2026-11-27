import type { ReactNode } from 'react';
import type { TFunction } from 'i18next';
import type { ArrayFieldItemButtonsTemplateProps, ArrayFieldItemTemplateProps, ArrayFieldTemplateProps, FieldProps, RJSFSchema } from '@rjsf/utils';

import { cn } from '@shared/lib/utils';
import { CSS } from '@dnd-kit/utilities';
import { createPortal } from 'react-dom';
import { getUiOptions } from '@rjsf/utils';
import { useTranslation } from 'react-i18next';
// @rjsf/core's entry also loads its test registry, which imports the Ajv validator.
import Form from '@rjsf/core/lib/components/Form.js';
import { createContext, use, useState } from 'react';
import { Button } from '@shared/components/ui/button';
import { Copy, MoreHorizontal, Plus, Trash2 } from 'lucide-react';
import getDefaultRegistry from '@rjsf/core/lib/getDefaultRegistry.js';
import { arrayMove, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable';
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@shared/components/ui/dialog';
import { DropdownMenu, DropdownMenuContent, DropdownMenuGroup, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@shared/components/ui/dropdown-menu';

import { usePrefersReducedMotion } from '@/hooks/use-prefers-reduced-motion';
import { dropSlot, SortContext, SortGrip, SortOverlay } from '@/components/sortable';

import { SchemaDialogLayout } from './schema-dialog-layout';
import { resolvedSchema, schemaDiscriminatorValues, selfContainedSchema, uiSchemaFromPanel } from '../schema-ui';
import { ConfigurationList, ConfigurationListActions, ConfigurationListAddButton, ConfigurationListAddRow, ConfigurationListCell, ConfigurationListRow, ConfigurationListText, SortableConfigurationListRow } from '../configuration-list';

const DefaultArrayField = getDefaultRegistry().fields.ArrayField;
const ArrayFieldActionsContext = createContext<{
  create: () => void;
  edit: (index: number) => void;
  /** Moves an entry to another's place, as a drag drops it. */
  reorder: (from: number, to: number) => void;
} | null>(null);

/** Stage additions and edits locally, including entries in nested collections. */
export function PanelArrayField(props: FieldProps) {
  const { t } = useTranslation();
  const { disabled, readonly, schema, registry, formData, fieldPathId, onChange } = props;
  const [pending, setPending] = useState<{ value: unknown; index: number | null } | null>(null);
  const [open, setOpen] = useState(false);
  const itemSchema = schema.items as RJSFSchema;
  function openCreate() {
    const value = registry.schemaUtils.getDefaultFormState(itemSchema) ?? {};
    const discriminator = resolvedSchema(itemSchema, registry.rootSchema).discriminator?.propertyName;
    // Native single-value enums identify the selected branch but are not RJSF defaults.
    if (typeof discriminator === 'string' && value[discriminator] === undefined) {
      const [first] = schemaDiscriminatorValues(itemSchema, registry.rootSchema, discriminator);
      if (first !== undefined) value[discriminator] = first;
    }
    setPending({ value, index: null });
    setOpen(true);
  }
  return (
    <ArrayFieldActionsContext value={{
      create: openCreate,
      edit: (index) => {
        setPending({ value: formData[index], index });
        setOpen(true);
      },
      reorder: (from, to) => {
        const list: unknown[] = Array.isArray(formData) ? formData : [];
        const outside = (index: number) => index < 0 || index >= list.length;
        if (disabled || readonly || from === to || outside(from) || outside(to)) return;
        onChange(arrayMove(list, from, to), fieldPathId.path);
      },
    }}>
      <DefaultArrayField {...props} />
      <Dialog open={open} onOpenChange={setOpen} onOpenChangeComplete={(open) => {
        // Retain the form and its size throughout the closing animation.
        if (!open) setPending(null);
      }}>
        <DialogContent className='configuration-entry-dialog'>
          <DialogHeader>
            <DialogTitle>{t(pending?.index == null ? 'configuration.general.addEntry' : 'configuration.general.editEntry')}</DialogTitle>
            <DialogDescription className='sr-only'>{t(pending?.index == null ? 'configuration.general.addDescription' : 'configuration.general.editDescription')}</DialogDescription>
          </DialogHeader>
          {pending !== null && (
            <div className='schema-form configuration-entry-dialog__body'>
              <SchemaDialogLayout schema={itemSchema} root={registry.rootSchema} data={pending.value}>
                <Form
                  tagName='div'
                  disabled={disabled || readonly}
                  schema={selfContainedSchema(itemSchema, registry.rootSchema)}
                  formData={pending.value}
                  idPrefix={`${fieldPathId.$id}-dialog`}
                  fields={registry.fields}
                  templates={registry.templates}
                  widgets={registry.widgets}
                  validator={registry.schemaUtils.getValidator()}
                  experimental_defaultFormStateBehavior={{ emptyObjectFields: 'populateRequiredDefaults' }}
                  noValidate
                  noHtml5Validate
                  uiSchema={{
                    ...uiSchemaFromPanel(itemSchema, [], registry.rootSchema, pending.value),
                    'ui:title': '', 'ui:description': '', 'ui:submitButtonOptions': { norender: true },
                  }}
                  onChange={({ formData: value }) => setPending((current) =>
                    current === null ? null : { ...current, value })}
                />
              </SchemaDialogLayout>
            </div>
          )}
          <DialogFooter>
            <DialogClose render={<Button type='button' variant='outline' />}>{t('common.cancel')}</DialogClose>
            <Button disabled={disabled || readonly} type='button' onClick={() => {
              if (pending === null || disabled || readonly) return;
              const next = [...(Array.isArray(formData) ? formData : [])];
              if (pending.index === null) next.push(pending.value);
              else next[pending.index] = pending.value;
              onChange(next, fieldPathId.path);
              setOpen(false);
            }}>
              {t(pending?.index == null ? 'common.add' : 'configuration.general.saveChanges')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </ArrayFieldActionsContext>
  );
}

interface ArrayEditorState {
  locked: boolean;
  /** Whether entries drag into their order by a grip, and whether that grip is off. */
  sortable: boolean;
  standalone?: boolean;
  edit: (index: number) => void;
  userCredentials?: typeof userCredentials;
  records: Record<string, unknown>[] | null;
}

const ArrayEditorContext = createContext<ArrayEditorState | null>(null);

const userCredentials = [
  { keys: ['password', 'Password'], label: 'configuration.fields.password' },
  { keys: ['uuid'], label: 'configuration.userList.uuid' },
  { keys: ['auth', 'auth_str'], label: 'configuration.userList.auth' },
  { keys: ['userkey'], label: 'configuration.userList.userKey' },
  { keys: ['token'], label: 'configuration.userList.token' },
];

function itemLabel(item: Record<string, unknown>, index: number): string {
  const value = [item.tag, item.name, item.id].find((value) => typeof value === 'string' && value !== '');
  return typeof value === 'string' ? value : String(index + 1).padStart(2, '0');
}

function itemSummary(item: Record<string, unknown>): string {
  return [item.server, item.outbound, item.path, item.url, item.domain, item.rule_set]
    .flatMap((value) => typeof value === 'string' ? [value] : Array.isArray(value) ? value.filter((part) => typeof part === 'string') : [])
    .slice(0, 3)
    .join(' · ');
}

function hasObjectItems(schema: RJSFSchema, root: RJSFSchema): boolean {
  const resolved = resolvedSchema(schema, root);
  if (resolved.type === 'object') return true;
  const branches = resolved.oneOf ?? resolved.anyOf;
  return branches !== undefined && branches.length > 0
    && branches.every((branch) => typeof branch === 'object' && hasObjectItems(branch, root));
}

export function PanelArrayFieldTemplate(props: ArrayFieldTemplateProps) {
  const { t } = useTranslation();
  const {
    canAdd, disabled, fieldPathId, formData, items, onAddClick, readonly, registry, schema, title, uiSchema,
  } = props;
  const actions = use(ArrayFieldActionsContext);
  const data: unknown[] = Array.isArray(formData) ? formData : [];
  const objectItems = schema.items !== undefined && typeof schema.items === 'object' && !Array.isArray(schema.items)
    && hasObjectItems(schema.items, registry.rootSchema);
  const records = (data.length > 0 ? data.every((item) => item !== null && typeof item === 'object' && !Array.isArray(item)) : objectItems)
    ? data as Record<string, unknown>[]
    : null;
  const { ArrayFieldTitleTemplate } = registry.templates;
  const actionContainer = fieldPathId.path.length === 0
    ? registry.formContext?.arrayActionContainer as HTMLElement | null | undefined
    : null;
  const standalone = records !== null && fieldPathId.path.length === 0 && registry.formContext?.arrayLayout === 'standalone';
  const userList = records !== null && fieldPathId.path.at(-1) === 'users';
  const userFields = userList ? resolvedSchema(schema.items as RJSFSchema, registry.rootSchema).properties : undefined;
  const credentials = userList
    ? userCredentials.filter(({ keys }) => keys.some((key) => userFields?.[key] !== undefined))
    : undefined;
  // An orderable array's entries drag into their order by a grip, which a locked form turns off.
  const sortable = actions !== null && getUiOptions(uiSchema).orderable !== false;
  const locked = Boolean(disabled || readonly);
  const entryName = (index: number) => {
    const record = records?.[index];
    if (record === undefined) return String(data[index] ?? index + 1);
    return userList ? userName(record, index, t) : itemLabel(record, index);
  };
  const sortableItems = (content: ReactNode) => sortable
    ? (
        <SortContext
          ended={t('common.orderUpdated')}
          instructions={t('common.reorderInstructions')}
          items={items.map((_, index) => ({ id: String(index), label: entryName(index) }))}
          onMove={(active, over) => actions?.reorder(Number(active), Number(over))}
          renderOverlay={(id) => {
            const index = Number(id);
            const record = records?.[index];
            const type = record?.type ?? record?.action;
            return <SortOverlay detail={typeof type === 'string' ? type : undefined} name={entryName(index)} />;
          }}
          strategy={verticalListSortingStrategy}
        >
          {content}
        </SortContext>
      )
    : content;
  const addButton = canAdd
    ? (
        standalone
          ? (
              <ConfigurationListAddButton
                disabled={disabled || readonly}
                onClick={actions !== null ? actions.create : onAddClick}
              >
                <Plus aria-hidden data-icon='inline-start' />
                {t('common.add')}
              </ConfigurationListAddButton>
            )
          : (
              <Button disabled={disabled || readonly} onClick={records !== null && actions !== null ? actions.create : onAddClick} size='sm' type='button' variant={records === null ? 'ghost' : 'outline'}>
                <Plus aria-hidden data-icon='inline-start' />
                {t('common.add')}
              </Button>
            )
      )
    : null;
  if (standalone) {
    return (
      <fieldset className='min-w-0 p-0' id={`${fieldPathId.$id}-group`}>
        <ArrayEditorContext value={{ records, standalone, sortable, locked, edit: (index) => actions?.edit(index) }}>
          <ConfigurationList labels={[t('configuration.fields.tag'), t('configuration.fields.type'), t('configuration.general.summary'), t('common.actions')]} sortable={sortable}>
            {sortableItems(items)}
            {addButton && <ConfigurationListAddRow>{addButton}</ConfigurationListAddRow>}
          </ConfigurationList>
        </ArrayEditorContext>
      </fieldset>
    );
  }
  return (
    <fieldset className={`schema-form__array${records === null ? ' schema-form__array--values' : ''}${userList ? ' schema-form__array--users' : ''}${sortable ? ' schema-form__array--sortable' : ''}`} data-credentials={credentials !== undefined && credentials.length > 0} id={`${fieldPathId.$id}-group`}>
      <div className='schema-form__array-toolbar' hidden={!!actionContainer}>
        <div className='schema-form__array-heading'>
          <ArrayFieldTitleTemplate {...props} title={uiSchema?.['ui:title'] === '' ? '' : title} />
        </div>
        {actionContainer ? createPortal(addButton, actionContainer) : addButton}
      </div>
      <ArrayEditorContext
        value={{ records, sortable, locked, userCredentials: credentials, edit: (index) => actions?.edit(index) }}
      >
        {records !== null && items.length > 0
          ? (
              <div aria-hidden className='schema-form__list-header'>
                {credentials !== undefined
                  ? (
                      <>
                        <span>{t('configuration.fields.username')}</span>
                        {credentials.length > 0 && <span>{t(credentials.length === 1 ? credentials[0].label : 'configuration.userList.credentials')}</span>}
                      </>
                    )
                  : (
                      <>
                        <span>{t('configuration.fields.tag')}</span>
                        <span>{t('configuration.fields.type')}</span>
                        <span>{t('configuration.general.summary')}</span>
                      </>
                    )}
                <span>{t('common.actions')}</span>
              </div>
            )
          : null}
        <div className='schema-form__array-items'>{sortableItems(items)}</div>
        {items.length === 0 && records !== null ? <p className='schema-form__empty'>{t('configuration.general.empty')}</p> : null}
      </ArrayEditorContext>
    </fieldset>
  );
}

/** A user's name in a user list, or a numbered placeholder for an unnamed one. */
function userName(record: Record<string, unknown>, index: number, t: TFunction): string {
  const username = [record.username, record.Username, record.name].find((value) => typeof value === 'string' && value.trim() !== '');
  return typeof username === 'string' ? username : t('configuration.userList.unnamed', { number: index + 1 });
}

/** A sortable entry's place and style while it drags, and the grip that drags it. */
function useSortableEntry(index: number, name: string, state: ArrayEditorState | null) {
  const { t } = useTranslation();
  const reducedMotion = usePrefersReducedMotion();
  const sortable = state?.sortable === true;
  const { attributes, isDragging, listeners, setActivatorNodeRef, setNodeRef, transform, transition }
    = useSortable({ id: String(index), disabled: !sortable || state.locked });
  return {
    className: isDragging ? dropSlot : undefined,
    grip: (className: string) => sortable
      ? <SortGrip activatorRef={setActivatorNodeRef} attributes={attributes} className={className} disabled={state.locked} label={t('common.reorderItem', { name })} listeners={listeners} />
      : null,
    ref: sortable ? setNodeRef : undefined,
    style: sortable
      ? { transform: CSS.Translate.toString(transform), transition: reducedMotion ? undefined : transition }
      : undefined,
  };
}

/** An object entry's line: its name, its type and summary or its credentials, then edit and its buttons. */
function RecordRow({ buttons, credentials, index, name, record, state }: {
  index: number;
  name: string;
  buttons: ReactNode;
  credentials?: string;
  record: Record<string, unknown>;
  state: ArrayEditorState | null;
}) {
  const { t } = useTranslation();
  const entry = useSortableEntry(index, name, state);
  return (
    <div className={cn('schema-form__list-row', entry.className)} ref={entry.ref} style={entry.style}>
      {entry.grip('schema-form__item-grip')}
      <span className='schema-form__item-name' title={name}>{name}</span>
      {credentials !== undefined
        ? (
            credentials !== '' && <span className='schema-form__item-summary schema-form__item-credentials'>{credentials}</span>
          )
        : (
            <>
              <span className='schema-form__item-type'>{String(record.type ?? record.action ?? '—')}</span>
              <span className='schema-form__item-summary'>{itemSummary(record) || '—'}</span>
            </>
          )}
      <div className='schema-form__item-actions'>
        <Button disabled={state?.locked} onClick={() => state?.edit(index)} size='sm' type='button' variant='ghost'>{t('common.edit')}</Button>
        {buttons}
      </div>
    </div>
  );
}

/** A plain value's line: its field, then a menu to copy or remove it. */
function ValueRow({ buttonsProps, children, index, name, state }: {
  index: number;
  name: string;
  children: ReactNode;
  state: ArrayEditorState | null;
  buttonsProps: ArrayFieldItemButtonsTemplateProps;
}) {
  const { t } = useTranslation();
  const entry = useSortableEntry(index, name, state);
  return (
    <div className={cn('schema-form__value-row', entry.className)} ref={entry.ref} style={entry.style}>
      {entry.grip('')}
      <div className='schema-form__value-field'>{children}</div>
      {buttonsProps.hasCopy || buttonsProps.hasRemove
        ? (
            <DropdownMenu>
              <DropdownMenuTrigger render={<Button aria-label={t('common.actions')} disabled={buttonsProps.disabled || buttonsProps.readonly} size='icon-sm' type='button' variant='ghost' />}>
                <MoreHorizontal aria-hidden />
              </DropdownMenuTrigger>
              <DropdownMenuContent align='end' className='w-auto min-w-40'>
                {buttonsProps.hasCopy
                  ? (
                      <DropdownMenuGroup>
                        <DropdownMenuItem onClick={buttonsProps.onCopyItem}>
                          <Copy aria-hidden='true' />
                          {t('common.copy')}
                        </DropdownMenuItem>
                      </DropdownMenuGroup>
                    )
                  : null}
                {buttonsProps.hasCopy && buttonsProps.hasRemove ? <DropdownMenuSeparator /> : null}
                {buttonsProps.hasRemove
                  ? (
                      <DropdownMenuGroup>
                        <DropdownMenuItem onClick={buttonsProps.onRemoveItem} variant='destructive'>
                          <Trash2 aria-hidden='true' />
                          {t('common.remove')}
                        </DropdownMenuItem>
                      </DropdownMenuGroup>
                    )
                  : null}
              </DropdownMenuContent>
            </DropdownMenu>
          )
        : null}
    </div>
  );
}

/**
 * An array entry: a row of a standalone collection, an object's line or a
 * plain value's line. In an orderable array each starts with the grip that
 * drags it into its order, so none carries move up or down.
 */
export function PanelArrayFieldItemTemplate({ buttonsProps, children, index, registry }: ArrayFieldItemTemplateProps) {
  const { t } = useTranslation();
  const state = use(ArrayEditorContext);
  const { ArrayFieldItemButtonsTemplate } = registry.templates;
  const buttons = <ArrayFieldItemButtonsTemplate {...buttonsProps} hasMoveDown={false} hasMoveUp={false} />;
  const record = state?.records?.[index];
  if (record !== undefined && state?.standalone) {
    const name = itemLabel(record, index);
    const cells = (
      <>
        <ConfigurationListCell>
          <ConfigurationListText title={name}>{name}</ConfigurationListText>
        </ConfigurationListCell>
        <ConfigurationListCell secondary><ConfigurationListText>{String(record.type ?? record.action ?? '—')}</ConfigurationListText></ConfigurationListCell>
        <ConfigurationListCell secondary><ConfigurationListText title={itemSummary(record)}>{itemSummary(record) || '—'}</ConfigurationListText></ConfigurationListCell>
        <ConfigurationListCell>
          <ConfigurationListActions>
            <Button disabled={buttonsProps.disabled || buttonsProps.readonly} onClick={() => state.edit(index)} size='sm' type='button' variant='ghost'>{t('common.edit')}</Button>
            {buttons}
          </ConfigurationListActions>
        </ConfigurationListCell>
      </>
    );
    return state.sortable
      ? (
          <SortableConfigurationListRow disabled={state.locked} id={String(index)} name={name}>
            {cells}
          </SortableConfigurationListRow>
        )
      : <ConfigurationListRow>{cells}</ConfigurationListRow>;
  }
  if (record !== undefined) {
    const name = state?.userCredentials !== undefined ? userName(record, index, t) : itemLabel(record, index);
    const credentials = state?.userCredentials?.map(({ keys, label }) => {
      const values = keys.flatMap((key) => {
        const value = record[key];
        if (typeof value === 'string' && value.length > 0) return [value];
        if (Array.isArray(value) && value.length > 0) return [JSON.stringify(value)];
        return [];
      });
      if (state.userCredentials?.length === 1) return values.join(' · ') || '—';
      return values.length > 0
        ? t('configuration.userList.credentialValue', { credential: t(label), value: values.join(' · ') })
        : t('configuration.userList.credentialMissing', { credential: t(label) });
    }).join('\n');
    return (
      <RecordRow buttons={buttons} credentials={credentials} index={index} name={name} record={record} state={state} />
    );
  }
  return (
    <ValueRow buttonsProps={buttonsProps} index={index} name={t('configuration.general.entry', { number: index + 1 })} state={state}>
      {children}
    </ValueRow>
  );
}
