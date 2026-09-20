import type { ArrayFieldItemTemplateProps, ArrayFieldTemplateProps } from '@rjsf/utils';

import { useTranslation } from 'react-i18next';
import { createContext, use, useState } from 'react';
import { ArrowLeft, MoreHorizontal, Plus } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { DropdownMenu, DropdownMenuContent, DropdownMenuGroup, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu';

interface ArrayEditorState {
  editing: number | null;
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

export function PanelArrayFieldTemplate(props: ArrayFieldTemplateProps) {
  const { t } = useTranslation();
  const {
    canAdd, disabled, fieldPathId, formData, items, onAddClick, readonly, registry, schema, title, uiSchema,
  } = props;
  const [editing, setEditing] = useState<number | null>(null);
  const data: unknown[] = Array.isArray(formData) ? formData : [];
  const objectItems = schema.items !== undefined && typeof schema.items === 'object' && !Array.isArray(schema.items)
    && (schema.items.type === 'object' || schema.items.$ref !== undefined || schema.items.oneOf !== undefined || schema.items.anyOf !== undefined);
  const records = (data.length > 0 ? data.every((item) => item !== null && typeof item === 'object' && !Array.isArray(item)) : objectItems)
    ? data as Record<string, unknown>[]
    : null;
  const active = editing !== null && editing < items.length ? editing : null;
  const { ArrayFieldTitleTemplate } = registry.templates;
  return (
    <fieldset className={`schema-form__array${records === null ? ' schema-form__array--values' : ''}`} id={`${fieldPathId.$id}-group`}>
      <div className='schema-form__array-toolbar'>
        {active !== null && records !== null
          ? (
              <>
                <Button onClick={() => setEditing(null)} size='sm' type='button' variant='ghost'>
                  <ArrowLeft aria-hidden />
                  {t('common.back')}
                </Button>
                <span className='schema-form__editing-name'>{itemLabel(records[active], active)}</span>
                <Button onClick={() => setEditing(null)} size='sm' type='button' variant='secondary'>{t('configuration.general.done')}</Button>
              </>
            )
          : (
              <>
                <div className='schema-form__array-heading'>
                  <ArrayFieldTitleTemplate {...props} title={uiSchema?.['ui:title'] === '' ? '' : title} />
                  <span className='schema-form__count'>{t('configuration.general.items', { count: items.length })}</span>
                </div>
                {canAdd
                  ? (
                      <Button disabled={disabled || readonly} onClick={(event) => {
                        if (records !== null) setEditing(items.length);
                        onAddClick(event);
                      }} size='sm' type='button' variant={records === null ? 'ghost' : 'outline'}>
                        <Plus aria-hidden />
                        {t('common.add')}
                      </Button>
                    )
                  : null}
              </>
            )}
      </div>
      <ArrayEditorContext value={{ records, editing: active, edit: setEditing }}>
        {records !== null && active === null && items.length > 0
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
  if (record !== undefined) {
    if (state?.editing !== null) return state?.editing === index ? <div className='schema-form__item-editor'>{children}</div> : null;
    return (
      <div className='schema-form__list-row'>
        <button className='schema-form__item-name' onClick={() => state?.edit(index)} type='button'>{itemLabel(record, index)}</button>
        <span className='schema-form__item-type'>{String(record.type ?? record.action ?? '—')}</span>
        <span className='schema-form__item-summary'>{itemSummary(record) || '—'}</span>
        <div className='schema-form__item-actions'>
          <Button onClick={() => state?.edit(index)} size='sm' type='button' variant='ghost'>{t('common.edit')}</Button>
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
