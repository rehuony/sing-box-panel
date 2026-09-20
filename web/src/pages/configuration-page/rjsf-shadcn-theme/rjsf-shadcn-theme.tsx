/* eslint-disable react-refresh/only-export-components */
// RJSF requires one registry module that exports component maps.
import type { TFunction } from 'i18next';
import type { ComponentType } from 'react';
import type {
  ArrayFieldTitleProps,
  BaseInputTemplateProps,
  FieldProps,
  FieldTemplateProps,
  IconButtonProps,
  MultiSchemaFieldTemplateProps,
  ObjectFieldTemplateProps,
  RJSFSchema,
  TemplatesType,
  WidgetProps,
} from '@rjsf/utils';

import { useTranslation } from 'react-i18next';
import { getDefaultRegistry } from '@rjsf/core';
import { createContext, isValidElement, use, useId } from 'react';
import {
  ArrowDown,
  ArrowUp,
  Copy,
  Plus,
  Trash2,
  X,
} from 'lucide-react';
import {
  ADDITIONAL_PROPERTY_FLAG,
  canExpand,
  enumOptionSelectedValue,
  enumOptionValueDecoder,
  enumOptionValueEncoder,
  getInputProps,
  getOptionValueFormat,
} from '@rjsf/utils';

import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { Switch } from '@/components/ui/switch';
import { Textarea } from '@/components/ui/textarea';
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
  FieldLegend,
} from '@/components/ui/field';
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';

import { PanelArrayFieldItemTemplate, PanelArrayFieldTemplate } from './array-field-templates';
import './rjsf-shadcn-theme.css';

const SchemaChoiceContext = createContext<{ id: string; label?: string } | null>(null);

interface PanelMetadata {
  order?: number;
  widget?: string;
  sensitive?: boolean;
  label?: Record<string, string>;
}

function metadata(schema: RJSFSchema): PanelMetadata {
  const value = schema['x-panel'];
  return value !== null && typeof value === 'object' && !Array.isArray(value)
    ? value as PanelMetadata
    : {};
}

function localizedLabel(schema: RJSFSchema, fallback: string, language: string, t: TFunction): string {
  if (fallback.startsWith('configuration.valueTypes.')) return t(fallback);
  const labels = metadata(schema).label;
  return labels?.[language] ?? labels?.en ?? t(`configuration.fields.${fallback}`, { defaultValue: schema.title ?? fallback });
}

function PanelFieldTemplate(props: FieldTemplateProps) {
  const { i18n, t } = useTranslation();
  const {
    children, description, disabled, displayLabel, errors, fieldPathId, hidden, id,
    label, rawDescription, rawErrors, required, schema,
  } = props;
  if (hidden) return children;
  if (ADDITIONAL_PROPERTY_FLAG in schema) {
    return (
      <div className='schema-form__map-entry'>
        <Input aria-label={t('configuration.general.propertyName')} defaultValue={label} disabled={disabled || props.readonly}
          key={label} onBlur={props.onKeyRenameBlur} />
        <div className='schema-form__map-value'>{children}</div>
        <Button aria-label={t('common.remove')} disabled={disabled || props.readonly} onClick={props.onRemoveProperty}
          size='icon-sm' type='button' variant='ghost'>
          <Trash2 aria-hidden />
        </Button>
      </div>
    );
  }
  if (fieldPathId.path.length === 0 || schema.type === 'object' || schema.type === 'array') {
    return children;
  }
  const discriminator = (schema.discriminator as { propertyName?: string } | undefined)?.propertyName;
  if (discriminator && (schema.oneOf || schema.anyOf)) return children;
  const resolvedLabel = localizedLabel(schema, label, i18n.language, t);
  const booleanField = schema.type === 'boolean';
  const labelTarget = id;
  return (
    <Field
      className={`schema-form__field${schema.oneOf || schema.anyOf ? ' schema-form__field--union' : ''}`}
      data-unlabelled={(displayLabel === false && !booleanField) || undefined}
      orientation={booleanField ? 'horizontal' : 'vertical'}
      data-disabled={disabled || undefined}
      data-invalid={rawErrors !== undefined && rawErrors.length > 0 ? true : undefined}
    >
      {displayLabel === false && !booleanField && !discriminator
        ? null
        : (
            <FieldLabel htmlFor={labelTarget}>
              {resolvedLabel}
              {required ? <span aria-hidden='true'>*</span> : null}
            </FieldLabel>
          )}
      <div className='schema-form__control'>{children}</div>
      {rawDescription ? <FieldDescription>{description}</FieldDescription> : null}
      {rawErrors !== undefined && rawErrors.length > 0 ? <FieldError>{errors}</FieldError> : null}
    </Field>
  );
}

function PanelObjectFieldTemplate(props: ObjectFieldTemplateProps) {
  const { i18n, t } = useTranslation();
  const groupId = useId();
  const {
    description, fieldPathId, optionalDataControl, properties, schema, title,
  } = props;
  const visible = properties.filter((property) => !property.hidden);
  const resolvedTitle = localizedLabel(schema, title, i18n.language, t);
  const connectionFields = new Set([
    'bind_address_no_port', 'bind_interface', 'connect_timeout', 'disable_tcp_keep_alive',
    'fallback_delay', 'fallback_network_type', 'inet4_bind_address', 'inet6_bind_address',
    'netns', 'network_strategy', 'network_type', 'protect_path', 'reuse_addr', 'routing_mark',
    'tcp_fast_open', 'tcp_keep_alive', 'tcp_keep_alive_interval', 'tcp_multi_path', 'udp_fragment',
  ]);
  const connection = visible.filter((property) => connectionFields.has(property.name));
  const commonRuleFields = new Set(['type', 'inbound', 'domain', 'domain_suffix', 'ip_cidr', 'ip_is_private', 'network', 'protocol', 'port', 'rule_set', 'invert']);
  const rule = visible.some((property) => property.name === 'domain') && visible.some((property) => property.name === 'ip_cidr');
  const matching = rule
    ? visible.filter((property) => !commonRuleFields.has(property.name) && !connectionFields.has(property.name))
    : [];
  const main = visible.filter((property) => !connectionFields.has(property.name) && !matching.includes(property));
  const root = fieldPathId.path.length === 0 || typeof fieldPathId.path.at(-1) === 'number';
  return (
    <fieldset className={root ? 'schema-form__root' : 'schema-form__object'} id={`${fieldPathId.$id}-group-${groupId}`}>
      {!root && resolvedTitle !== '' ? <FieldLegend>{resolvedTitle}</FieldLegend> : null}
      {optionalDataControl}
      {description ? <FieldDescription>{description}</FieldDescription> : null}
      <FieldGroup className='schema-form__grid'>
        {main.map((property) => <div key={property.name}>{property.content}</div>)}
      </FieldGroup>
      {canExpand(schema, props.uiSchema, props.formData)
        ? (
            <Button className='schema-form__add-property' disabled={props.disabled || props.readonly} onClick={props.onAddProperty}
              size='sm' type='button' variant='outline'>
              <Plus aria-hidden />
              {t('configuration.general.addProperty')}
            </Button>
          )
        : null}
      {matching.length > 0
        ? (
            <details className='schema-form__details'>
              <summary>{t('configuration.general.moreConditions')}</summary>
              <FieldGroup className='schema-form__grid'>
                {matching.map((property) => <div key={property.name}>{property.content}</div>)}
              </FieldGroup>
            </details>
          )
        : null}
      {connection.length > 0
        ? (
            <details className='schema-form__details'>
              <summary>{t('configuration.general.connection')}</summary>
              <FieldGroup className='schema-form__grid'>
                {connection.map((property) => <div key={property.name}>{property.content}</div>)}
              </FieldGroup>
            </details>
          )
        : null}
    </fieldset>
  );
}

function PanelArrayTitleTemplate(props: ArrayFieldTitleProps) {
  const { i18n, t } = useTranslation();
  const { title, schema, optionalDataControl } = props;
  return (
    <>
      {title && !title.startsWith('configuration.valueTypes.')
        ? <FieldLegend>{localizedLabel(schema, title, i18n.language, t)}</FieldLegend>
        : null}
      {optionalDataControl}
    </>
  );
}

const DefaultObjectField = getDefaultRegistry().fields.ObjectField;

/** Explicitly configured empty objects must remain open, even before their first field is filled. */
function PanelObjectField(props: FieldProps) {
  const { i18n, t } = useTranslation();
  const { disabled, fieldPathId, formData, name, onChange, readonly, required, schema } = props;
  if (fieldPathId.path.length === 0 || typeof fieldPathId.path.at(-1) === 'number' || required) return <DefaultObjectField {...props} />;
  const present = formData !== undefined && formData !== null;
  if (!present) {
    return (
      <fieldset className='schema-form__object schema-form__object--empty'>
        <FieldLegend>{localizedLabel(schema, name, i18n.language, t)}</FieldLegend>
        <Button disabled={disabled || readonly} onClick={() => onChange({}, fieldPathId.path)} size='sm' type='button' variant='outline'>
          <Plus aria-hidden data-icon='inline-start' />
          {t('configuration.general.configure')}
        </Button>
      </fieldset>
    );
  }
  return (
    <div className='schema-form__optional'>
      <DefaultObjectField {...props} />
      <Button disabled={disabled || readonly} onClick={() => onChange(undefined, fieldPathId.path)} size='sm' type='button' variant='ghost'>
        <X aria-hidden data-icon='inline-start' />
        {t('configuration.general.remove')}
      </Button>
    </div>
  );
}

export const panelRJSFFields = { ObjectField: PanelObjectField };

function PanelBaseInputTemplate(props: BaseInputTemplateProps) {
  const { i18n, t } = useTranslation();
  const {
    autofocus, disabled, htmlName, id, onBlur, onChange, onChangeOverride, onFocus,
    options, readonly, schema, type, value,
  } = props;
  const inputProps = getInputProps(schema, type, options);
  const inputValue = value === undefined || value === null ? '' : String(value);
  return (
    <Input
      {...inputProps}
      aria-label={localizedLabel(schema, props.label, i18n.language, t)}
      aria-invalid={props.rawErrors !== undefined && props.rawErrors.length > 0 ? true : undefined}
      autoFocus={autofocus}
      disabled={disabled}
      id={id}
      name={htmlName ?? id}
      onBlur={(event) => onBlur(id, event.currentTarget.value)}
      onChange={onChangeOverride ?? ((event) => onChange(event.currentTarget.value === '' ? options.emptyValue : event.currentTarget.value))}
      onFocus={(event) => onFocus(id, event.currentTarget.value)}
      readOnly={readonly}
      type={inputProps.type}
      value={inputValue}
    />
  );
}

function PanelCheckboxWidget(props: WidgetProps) {
  const { i18n, t } = useTranslation();
  const checked = props.value === true;
  return (
    <Switch
      aria-label={localizedLabel(props.schema, props.label, i18n.language, t)}
      checked={checked}
      disabled={props.disabled || props.readonly}
      id={props.id}
      onCheckedChange={(next) => props.onChange(next)}
    />
  );
}

function PanelSelectWidget(props: WidgetProps) {
  const { i18n, t } = useTranslation();
  const choice = use(SchemaChoiceContext);
  const enumOptions = props.options.enumOptions ?? [];
  const variantSelector = props.name?.endsWith('__oneof_select') || props.name?.endsWith('__anyof_select');
  const optionValueFormat = getOptionValueFormat(props.options);
  const encodedItems = enumOptions.map((option, index) => ({
    label: option.label.startsWith('configuration.valueTypes.') ? t(option.label) : option.label,
    value: String(enumOptionValueEncoder(option.value, index, optionValueFormat)),
  }));
  const selected = enumOptionSelectedValue(
    props.value,
    enumOptions,
    false,
    optionValueFormat,
    '',
  );
  return (
    <Select
      disabled={props.disabled || props.readonly}
      items={encodedItems}
      onValueChange={(next) => props.onChange(enumOptionValueDecoder(
        next ?? '', enumOptions, optionValueFormat, props.options.emptyValue,
      ))}
      value={String(selected ?? '')}
    >
      <SelectTrigger
        aria-label={localizedLabel(props.schema, choice?.label ?? props.label, i18n.language, t)}
        className={variantSelector ? 'schema-form__variant-select w-full' : 'w-full'}
        id={variantSelector ? `${choice?.id ?? props.id}-variant` : props.id}
      >
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectGroup>
          {encodedItems.map((item) => (
            <SelectItem key={item.value} value={item.value}>{item.label}</SelectItem>
          ))}
        </SelectGroup>
      </SelectContent>
    </Select>
  );
}

function PanelTextareaWidget(props: WidgetProps) {
  const { i18n, t } = useTranslation();
  return (
    <Textarea
      aria-label={localizedLabel(props.schema, props.label, i18n.language, t)}
      disabled={props.disabled}
      id={props.id}
      onBlur={(event) => props.onBlur(props.id, event.currentTarget.value)}
      onChange={(event) => props.onChange(event.currentTarget.value)}
      onFocus={(event) => props.onFocus(props.id, event.currentTarget.value)}
      readOnly={props.readonly}
      value={props.value === undefined ? '' : String(props.value)}
    />
  );
}

function HiddenWidget() {
  return null;
}

function iconButton(
  labelKey: string,
  Icon: ComponentType<{ 'aria-hidden'?: boolean; 'data-icon'?: string }>,
  destructive = false,
) {
  return function PanelIconButton({
    icon: _icon, registry: _registry, uiSchema: _uiSchema, ...props
  }: IconButtonProps) {
    const { t } = useTranslation();
    return (
      <Button
        {...props}
        aria-label={props['aria-label'] ?? t(labelKey)}
        size='icon-sm'
        type='button'
        className={destructive ? 'schema-form__remove' : undefined}
        variant='ghost'
      >
        <Icon aria-hidden data-icon='inline-start' />
      </Button>
    );
  };
}

function PanelMultiSchemaFieldTemplate({ selector, optionSchemaField, schema }: MultiSchemaFieldTemplateProps) {
  const { t } = useTranslation();
  const choiceId = `schema-choice-${useId()}`;
  const discriminator = (schema.discriminator as { propertyName?: string } | undefined)?.propertyName;
  const choice = <SchemaChoiceContext value={{ id: choiceId, label: discriminator }}>{selector}</SchemaChoiceContext>;
  if (discriminator) {
    return (
      <div className='schema-form__discriminated'>
        <Field className='schema-form__field'>
          <FieldLabel htmlFor={`${choiceId}-variant`}>{t(`configuration.fields.${discriminator}`, { defaultValue: discriminator })}</FieldLabel>
          <div className='schema-form__control'>{choice}</div>
        </Field>
        {optionSchemaField}
      </div>
    );
  }
  const option = isValidElement<{ schema: RJSFSchema }>(optionSchemaField) ? optionSchemaField.props.schema : undefined;
  const compact = typeof option?.type === 'string' && ['string', 'number', 'integer', 'boolean', 'null', 'array'].includes(option.type);
  return (
    <div className={`schema-form__union${compact ? ' schema-form__union--compact' : ''}`}>
      <div className='schema-form__union-selector'>
        {choice}
      </div>
      <div className='schema-form__union-value'>{optionSchemaField}</div>
    </div>
  );
}

export const panelRJSFTemplates: Partial<TemplatesType> = {
  ArrayFieldTemplate: PanelArrayFieldTemplate,
  ArrayFieldItemTemplate: PanelArrayFieldItemTemplate,
  MultiSchemaFieldTemplate: PanelMultiSchemaFieldTemplate,
  ArrayFieldTitleTemplate: PanelArrayTitleTemplate,
  BaseInputTemplate: PanelBaseInputTemplate,
  FieldTemplate: PanelFieldTemplate,
  ObjectFieldTemplate: PanelObjectFieldTemplate,
  ButtonTemplates: {
    SubmitButton: () => null,
    AddButton: iconButton('common.add', Plus),
    CopyButton: iconButton('common.copy', Copy),
    MoveDownButton: iconButton('common.moveDown', ArrowDown),
    MoveUpButton: iconButton('common.moveUp', ArrowUp),
    RemoveButton: iconButton('common.remove', Trash2, true),
    ClearButton: iconButton('common.clear', X),
  },
};

export const panelRJSFWidgets = {
  CheckboxWidget: PanelCheckboxWidget,
  SelectWidget: PanelSelectWidget,
  TextareaWidget: PanelTextareaWidget,
  hidden: HiddenWidget,
  password: PanelBaseInputTemplate,
};
