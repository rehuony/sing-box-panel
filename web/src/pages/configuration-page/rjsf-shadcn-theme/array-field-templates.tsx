import type { ArrayFieldItemTemplateProps, ArrayFieldTemplateProps, FieldProps, RJSFSchema } from '@rjsf/utils';

import { createPortal } from 'react-dom';
import { useTranslation } from 'react-i18next';
import { createContext, use, useState } from 'react';
import Form, { getDefaultRegistry } from '@rjsf/core';
import { ArrowDown, ArrowUp, MoreHorizontal, Plus } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { DropdownMenu, DropdownMenuContent, DropdownMenuGroup, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu';

import { resolvedSchema, schemaDiscriminatorValues, selfContainedSchema, uiSchemaFromPanel } from '../schema-ui';

const DefaultArrayField = getDefaultRegistry().fields.ArrayField;
const ArrayFieldActionsContext = createContext<{ create: () => void; edit: (index: number) => void } | null>(null);

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
    }}>
      <DefaultArrayField {...props} />
      <Dialog open={open} onOpenChange={setOpen} onOpenChangeComplete={(open) => {
        // Retain the form and its size throughout the closing animation.
        if (!open) setPending(null);
      }}>
        <DialogContent className='configuration-entry-dialog'>
          <DialogHeader>
            <DialogTitle>{t(pending?.index == null ? 'configuration.general.addEntry' : 'configuration.general.editEntry')}</DialogTitle>
            <DialogDescription>{t(pending?.index == null ? 'configuration.general.addDescription' : 'configuration.general.editDescription')}</DialogDescription>
          </DialogHeader>
          {pending !== null && (
            <div className='schema-form configuration-entry-dialog__body'>
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
            </div>
          )}
          <DialogFooter>
            <DialogClose render={<Button type='button' />}>{t('common.cancel')}</DialogClose>
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
  standalone?: boolean;
  edit: (index: number) => void;
  records: Record<string, unknown>[] | null;
}

const ArrayEditorContext = createContext<ArrayEditorState | null>(null);

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
  const addButton = canAdd
    ? (
        <Button className={standalone ? 'configuration-list__add' : undefined} disabled={disabled || readonly} onClick={records !== null && actions !== null ? actions.create : onAddClick} size={standalone ? 'content' : 'sm'} type='button' variant={records === null ? 'ghost' : 'outline'}>
          <Plus aria-hidden data-icon='inline-start' />
          {t('common.add')}
        </Button>
      )
    : null;
  if (standalone) {
    return (
      <fieldset className='schema-form__array schema-form__array--standalone' id={`${fieldPathId.$id}-group`}>
        <ArrayEditorContext value={{ records, standalone, edit: (index) => actions?.edit(index) }}>
          <Table className='configuration-list'>
            <TableHeader>
              <TableRow>
                <TableHead scope='col'>{t('configuration.fields.tag')}</TableHead>
                <TableHead scope='col'>{t('configuration.fields.type')}</TableHead>
                <TableHead scope='col'>{t('configuration.general.summary')}</TableHead>
                <TableHead scope='col'>{t('common.actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {items}
              {addButton && (
                <TableRow className='configuration-list__add-row'>
                  <TableCell colSpan={4}>{addButton}</TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </ArrayEditorContext>
      </fieldset>
    );
  }
  return (
    <fieldset className={`schema-form__array${records === null ? ' schema-form__array--values' : ''}`} id={`${fieldPathId.$id}-group`}>
      <div className='schema-form__array-toolbar' hidden={!!actionContainer}>
        <div className='schema-form__array-heading'>
          <ArrayFieldTitleTemplate {...props} title={uiSchema?.['ui:title'] === '' ? '' : title} />
        </div>
        {actionContainer ? createPortal(addButton, actionContainer) : addButton}
      </div>
      <ArrayEditorContext value={{ records, edit: (index) => actions?.edit(index) }}>
        {records !== null && items.length > 0
          ? (
              <div aria-hidden className='schema-form__list-header'>
                <span>{t('configuration.fields.tag')}</span>
                <span>{t('configuration.fields.type')}</span>
                <span>{t('configuration.general.summary')}</span>
                <span>{t('common.actions')}</span>
              </div>
            )
          : null}
        <div className='schema-form__array-items'>{items}</div>
        {items.length === 0 && records !== null ? <p className='schema-form__empty'>{t('configuration.general.empty')}</p> : null}
      </ArrayEditorContext>
    </fieldset>
  );
}

export function PanelArrayFieldItemTemplate({ buttonsProps, children, index, registry }: ArrayFieldItemTemplateProps) {
  const { t } = useTranslation();
  const state = use(ArrayEditorContext);
  const { ArrayFieldItemButtonsTemplate } = registry.templates;
  const buttons = <div className='schema-form__item-actions'><ArrayFieldItemButtonsTemplate {...buttonsProps} /></div>;
  const record = state?.records?.[index];
  if (record !== undefined && state?.standalone) {
    const name = itemLabel(record, index);
    return (
      <TableRow>
        <TableCell><span className='configuration-list__text' title={name}>{name}</span></TableCell>
        <TableCell className='configuration-list__secondary'><span className='configuration-list__text'>{String(record.type ?? record.action ?? '—')}</span></TableCell>
        <TableCell className='configuration-list__secondary'><span className='configuration-list__text' title={itemSummary(record)}>{itemSummary(record) || '—'}</span></TableCell>
        <TableCell>
          <div className='configuration-list__actions'>
            <Button disabled={buttonsProps.disabled || buttonsProps.readonly} onClick={() => state.edit(index)} size='sm' type='button' variant='ghost'>{t('common.edit')}</Button>
            <Button aria-label={t('configuration.general.moveUp', { name })} title={t('common.moveUp')} disabled={buttonsProps.disabled || buttonsProps.readonly || !buttonsProps.hasMoveUp} onClick={buttonsProps.onMoveUpItem} size='icon-sm' type='button' variant='ghost'><ArrowUp aria-hidden='true' /></Button>
            <Button aria-label={t('configuration.general.moveDown', { name })} title={t('common.moveDown')} disabled={buttonsProps.disabled || buttonsProps.readonly || !buttonsProps.hasMoveDown} onClick={buttonsProps.onMoveDownItem} size='icon-sm' type='button' variant='ghost'><ArrowDown aria-hidden='true' /></Button>
            <ArrayFieldItemButtonsTemplate {...buttonsProps} hasMoveUp={false} hasMoveDown={false} />
          </div>
        </TableCell>
      </TableRow>
    );
  }
  if (record !== undefined) {
    return (
      <div className='schema-form__list-row'>
        <span className='schema-form__item-name'>{itemLabel(record, index)}</span>
        <span className='schema-form__item-type'>{String(record.type ?? record.action ?? '—')}</span>
        <span className='schema-form__item-summary'>{itemSummary(record) || '—'}</span>
        <div className='schema-form__item-actions'>
          <Button disabled={buttonsProps.disabled || buttonsProps.readonly} onClick={() => state?.edit(index)} size='sm' type='button' variant='ghost'>{t('common.edit')}</Button>
          {buttons}
        </div>
      </div>
    );
  }
  return (
    <div className='schema-form__value-row'>
      <div>{children}</div>
      <DropdownMenu>
        <DropdownMenuTrigger render={<Button aria-label={t('common.actions')} disabled={buttonsProps.disabled || buttonsProps.readonly} size='icon-sm' type='button' variant='ghost' />}>
          <MoreHorizontal aria-hidden />
        </DropdownMenuTrigger>
        <DropdownMenuContent align='end'>
          <DropdownMenuGroup>
            {(buttonsProps.hasMoveUp || buttonsProps.hasMoveDown)
              ? (
                  <>
                    <DropdownMenuItem disabled={!buttonsProps.hasMoveUp} onClick={buttonsProps.onMoveUpItem}>{t('common.moveUp')}</DropdownMenuItem>
                    <DropdownMenuItem disabled={!buttonsProps.hasMoveDown} onClick={buttonsProps.onMoveDownItem}>{t('common.moveDown')}</DropdownMenuItem>
                  </>
                )
              : null}
            {buttonsProps.hasCopy ? <DropdownMenuItem onClick={buttonsProps.onCopyItem}>{t('common.copy')}</DropdownMenuItem> : null}
            {buttonsProps.hasRemove ? <DropdownMenuItem onClick={buttonsProps.onRemoveItem} variant='destructive'>{t('common.remove')}</DropdownMenuItem> : null}
          </DropdownMenuGroup>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}
