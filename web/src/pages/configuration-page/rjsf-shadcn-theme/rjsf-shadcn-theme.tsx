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
  Registry,
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
  Dices,
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
  getSchemaType,
} from '@rjsf/utils';

import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { Switch } from '@/components/ui/switch';
import { Textarea } from '@/components/ui/textarea';
import { toast } from '@/components/ui/toast-manager';
import { ErrorNotice } from '@/components/error-notice';
import { ServerPathInput } from '@/components/server-path-input';
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupInput } from '@/components/ui/input-group';
import {
  Field,
  FieldDescription,
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

import { resolvedSchema } from '../schema-ui';
import { SchemaFieldHelp } from './schema-field-help';
import { SchemaDialogContext } from './schema-dialog-context';
import { readConfigurationPathMode } from '../configuration-path-fields';
import { credentialGenerator, generateCredential } from '../configuration-credentials';
import { readConfigurationFieldHelp, withConfigurationFieldHelp } from '../configuration-field-help';
import { PanelArrayField, PanelArrayFieldItemTemplate, PanelArrayFieldTemplate } from './array-field-templates';
import './rjsf-shadcn-theme.css';

const SchemaChoiceContext = createContext<{ id: string; label?: string } | null>(null);
const ShadowsocksMethodContext = createContext<string | undefined>(undefined);

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
  const help = readConfigurationFieldHelp(schema);
  if (language.startsWith('zh') && help) return help.label;
  const labels = metadata(schema).label;
  return labels?.[language] ?? t(`configuration.fields.${fallback}`, { defaultValue: labels?.en ?? schema.title ?? fallback });
}

function PanelFieldTemplate(props: FieldTemplateProps) {
  const { i18n, t } = useTranslation();
  const {
    children, disabled, displayLabel, fieldPathId, hidden, id,
    label, rawDescription, rawErrors, required, schema,
  } = props;
  if (hidden) return children;
  if (ADDITIONAL_PROPERTY_FLAG in schema) {
    return (
      <div className='schema-form__map-entry'>
        <Input aria-label={t('configuration.general.propertyName')} className='schema-form__map-key'
          defaultValue={label} disabled={disabled || props.readonly}
          key={label} onBlur={props.onKeyRenameBlur} />
        <div className='schema-form__map-value'>{children}</div>
        <Button aria-label={t('common.remove')} disabled={disabled || props.readonly} onClick={props.onRemoveProperty}
          size='icon-sm' type='button' variant='ghost'>
          <Trash2 aria-hidden />
        </Button>
        {rawErrors !== undefined && rawErrors.length > 0 ? <ErrorNotice error={rawErrors.join('; ')} title={label} /> : null}
      </div>
    );
  }
  const schemaType = getSchemaType(schema);
  if (fieldPathId.path.length === 0 || schemaType === 'object' || schemaType === 'array') {
    return children;
  }
  const discriminator = (schema.discriminator as { propertyName?: string } | undefined)?.propertyName;
  if (discriminator && (schema.oneOf || schema.anyOf)) return children;
  const resolvedLabel = localizedLabel(schema, label, i18n.language, t);
  const booleanField = schemaType === 'boolean';
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
            <div className='schema-form__label'>
              <FieldLabel htmlFor={labelTarget}>
                {resolvedLabel}
                {required ? <span aria-hidden='true'>*</span> : null}
              </FieldLabel>
              <SchemaFieldHelp schema={schema} label={resolvedLabel} description={rawDescription} />
            </div>
          )}
      <div className='schema-form__control'>{children}</div>
      {rawErrors !== undefined && rawErrors.length > 0 ? <ErrorNotice error={rawErrors.join('; ')} title={label} /> : null}
    </Field>
  );
}

function PanelObjectFieldTemplate(props: ObjectFieldTemplateProps) {
  const { i18n, t } = useTranslation();
  const dialogLayout = use(SchemaDialogContext);
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
  const map = visible.length > 0 && visible.every((property) => {
    const child = schema.properties?.[property.name];
    return typeof child === 'object' && ADDITIONAL_PROPERTY_FLAG in child;
  });
  const objectValues = typeof schema.additionalProperties === 'object'
    && resolvedSchema(schema.additionalProperties, props.registry.rootSchema).type === 'object';
  // RJSF appends a synthetic segment for the base object alongside a root union.
  if (dialogLayout && fieldPathId.path.every((part) => part === 'XxxOf')) {
    const activeFields = visible.filter((property) =>
      dialogLayout.groupForField(property.name) === dialogLayout.active);
    const singleField = activeFields.length === 1 ? activeFields[0].name : undefined;
    const singleSchema = singleField === undefined ? undefined : schema.properties?.[singleField];
    const singleValue = singleField === undefined ? undefined : props.formData?.[singleField];
    const emptyArray = typeof singleSchema === 'object'
      && resolvedSchema(singleSchema, props.registry.rootSchema).type === 'array'
      && (singleValue === undefined || (Array.isArray(singleValue) && singleValue.length === 0));
    return (
      <fieldset className='schema-form__root schema-form__dialog-root' data-empty-array={emptyArray} id={`${fieldPathId.$id}-group-${groupId}`}>
        {optionalDataControl}
        <FieldGroup className='schema-form__grid'>
          {visible.map((property) => (
            <div key={property.name} className='schema-form__dialog-property'
              hidden={dialogLayout.groupForField(property.name) !== dialogLayout.active}>
              {property.content}
            </div>
          ))}
        </FieldGroup>
        {canExpand(schema, props.uiSchema, props.formData) && dialogLayout.active === dialogLayout.groupForField('')
          ? (
              <Button className='schema-form__add-property' disabled={props.disabled || props.readonly}
                onClick={props.onAddProperty} size='sm' type='button' variant='outline'>
                <Plus aria-hidden />
                {t('configuration.general.addProperty')}
              </Button>
            )
          : null}
      </fieldset>
    );
  }
  return (
    <fieldset className={root ? 'schema-form__root' : 'schema-form__object'} id={`${fieldPathId.$id}-group-${groupId}`}>
      {!root && resolvedTitle !== '' && props.uiSchema?.['ui:options']?.label !== false
        ? (
            <FieldLegend>
              <span className='schema-form__label'>
                {resolvedTitle}
                <SchemaFieldHelp schema={schema} label={resolvedTitle} />
              </span>
            </FieldLegend>
          )
        : null}
      {optionalDataControl}
      {root && description ? <FieldDescription>{description}</FieldDescription> : null}
      {map && !objectValues
        ? (
            <div aria-hidden className='schema-form__map-header'>
              <span>{t('configuration.general.propertyName')}</span>
              <span>{t('configuration.general.propertyValue')}</span>
              <span className='sr-only'>{t('common.actions')}</span>
            </div>
          )
        : null}
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
        ? (
            <FieldLegend>
              <span className='schema-form__label'>
                {localizedLabel(schema, title, i18n.language, t)}
                <SchemaFieldHelp schema={schema} label={localizedLabel(schema, title, i18n.language, t)} />
              </span>
            </FieldLegend>
          )
        : null}
      {optionalDataControl}
    </>
  );
}

const DefaultObjectField = getDefaultRegistry().fields.ObjectField;

function newMapValue(schema: RJSFSchema, registry: Registry): RJSFSchema['default'] {
  const resolved = resolvedSchema(schema, registry.rootSchema);
  if (resolved.default !== undefined) return resolved.default;
  if (resolved.const !== undefined) return resolved.const;
  const first = (resolved.anyOf ?? resolved.oneOf)?.[0];
  if (resolved.type === undefined && first && typeof first === 'object') return newMapValue(first, registry);
  const generated = registry.schemaUtils.getDefaultFormState(resolved);
  if (generated !== undefined) return generated;
  switch (resolved.type) {
    case 'object': return {};
    case 'array': return [];
    case 'boolean': return false;
    case 'number':
    case 'integer': return 0;
    case 'null': return null;
    default: return '';
  }
}

function objectSchemaForForm(schema: RJSFSchema, registry: Registry): RJSFSchema {
  if (!schema.additionalProperties || typeof schema.additionalProperties !== 'object') return schema;
  const valueSchema = resolvedSchema(schema.additionalProperties, registry.rootSchema);
  return {
    ...schema,
    additionalProperties: { ...schema.additionalProperties, default: newMapValue(valueSchema, registry) },
    // RJSF infers `object` for untyped map unions. Restore the declared value schema.
    properties: Object.fromEntries(Object.entries(schema.properties ?? {}).map(([key, value]) => [
      key,
      typeof value === 'object' && ADDITIONAL_PROPERTY_FLAG in value
      && valueSchema.type === undefined && (valueSchema.anyOf || valueSchema.oneOf)
        ? { ...valueSchema, [ADDITIONAL_PROPERTY_FLAG]: true }
        : value,
    ])),
  };
}

function PanelObjectField(props: FieldProps) {
  const inheritedMethod = use(ShadowsocksMethodContext);
  // React context follows nested array dialogs, including their uncommitted edits.
  const method = props.formData?.type === 'shadowsocks' ? props.formData.method : inheritedMethod;
  return <ShadowsocksMethodContext value={method}><PanelObjectFieldContent {...props} /></ShadowsocksMethodContext>;
}

/** Explicitly configured empty objects must remain open, even before their first field is filled. */
function PanelObjectFieldContent(props: FieldProps) {
  const { i18n, t } = useTranslation();
  const labelId = useId();
  const { disabled, fieldPathId, formData, name, onChange, readonly, required, schema } = props;
  const handleChange: FieldProps['onChange'] = (value, path, errors, id) => {
    // RJSF displays an optional union's first branch without materializing its
    // constants. Commit the selected branch's required identity on an edit,
    // before the leaf change; merely opening the form must not enable options.
    if (value !== undefined) {
      for (const key of schema.required ?? []) {
        const property = schema.properties?.[key];
        if (!property || typeof property !== 'object' || formData?.[key] !== undefined) continue;
        const constant = property.const ?? (property.enum?.length === 1 ? property.enum[0] : undefined);
        if (constant !== undefined) onChange(constant, [...fieldPathId.path, key]);
      }
    }
    onChange(value, path, errors, id);
  };
  const content = (
    <DefaultObjectField {...props} onChange={handleChange} schema={objectSchemaForForm(schema, props.registry)} />
  );
  if (fieldPathId.path.length === 0 || typeof fieldPathId.path.at(-1) === 'number' || required
    || props.uiSchema?.['ui:options']?.label === false) {
    return content;
  }
  const present = formData !== undefined && formData !== null;
  return (
    <div aria-labelledby={labelId} className='schema-form__optional' role='group'>
      <div className='schema-form__optional-header'>
        <div className='schema-form__label'>
          <span className='schema-form__object-label' id={labelId}>{localizedLabel(schema, name, i18n.language, t)}</span>
          <SchemaFieldHelp schema={schema} label={localizedLabel(schema, name, i18n.language, t)} />
        </div>
        <Button disabled={disabled || readonly} onClick={() => onChange(present ? undefined : {}, fieldPathId.path)}
          size='sm' type='button' variant={present ? 'destructive' : 'outline'}>
          {present ? <X aria-hidden data-icon='inline-start' /> : <Plus aria-hidden data-icon='inline-start' />}
          {t(present ? 'configuration.general.remove' : 'configuration.general.configure')}
        </Button>
      </div>
      {present && (
        <DefaultObjectField {...props} onChange={handleChange} schema={objectSchemaForForm(schema, props.registry)}
          uiSchema={{ ...props.uiSchema, 'ui:options': { ...props.uiSchema?.['ui:options'], label: false } }} />
      )}
    </div>
  );
}

export const panelRJSFFields = { ArrayField: PanelArrayField, ObjectField: PanelObjectField };

function PanelBaseInputTemplate(props: BaseInputTemplateProps) {
  const { i18n, t } = useTranslation();
  const method = use(ShadowsocksMethodContext);
  const {
    autofocus, disabled, htmlName, id, onBlur, onChange, onChangeOverride, onFocus,
    options, readonly, schema, type, value,
  } = props;
  const inputProps = getInputProps(schema, type, options);
  const inputValue = value === undefined || value === null ? '' : String(value);
  const pathMode = readConfigurationPathMode(schema);
  if (pathMode) {
    return (
      <ServerPathInput
        mode={pathMode} value={inputValue}
        aria-label={localizedLabel(schema, props.label, i18n.language, t)}
        aria-invalid={props.rawErrors !== undefined && props.rawErrors.length > 0 ? true : undefined}
        autoFocus={autofocus} disabled={disabled} readOnly={readonly} id={id} name={htmlName ?? id}
        onBlur={event => onBlur(id, event.currentTarget.value)}
        onFocus={event => onFocus(id, event.currentTarget.value)}
        onValueChange={next => onChange(next === '' ? options.emptyValue : next)}
      />
    );
  }
  const generator = credentialGenerator(schema, method);
  const InputComponent = generator ? InputGroupInput : Input;
  const input = (
    <InputComponent
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
      // Configuration credentials are already visible in the authenticated JSON editor.
      type={inputProps.type === 'password' ? 'text' : inputProps.type}
      value={inputValue}
      autoComplete={generator ? 'off' : undefined}
      spellCheck={generator ? false : undefined}
    />
  );
  if (!generator) return input;
  return (
    <InputGroup>
      {input}
      <InputGroupAddon align='inline-end'>
        <InputGroupButton disabled={disabled || readonly} size='icon-sm'
          aria-label={t('configuration.credentials.generate', { field: localizedLabel(schema, props.label, i18n.language, t) })}
          title={t('configuration.credentials.generate', { field: localizedLabel(schema, props.label, i18n.language, t) })}
          onClick={() => {
            try {
              onChange(generateCredential(generator));
            } catch {
              toast.add({ title: t('configuration.credentials.failed'), type: 'error' });
            }
          }}>
          <Dices aria-hidden />
        </InputGroupButton>
      </InputGroupAddon>
    </InputGroup>
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
      <SelectContent className='schema-form__select-popup'>
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
  const { i18n, t } = useTranslation();
  const dialogLayout = use(SchemaDialogContext);
  const choiceId = `schema-choice-${useId()}`;
  const discriminator = (schema.discriminator as { propertyName?: string } | undefined)?.propertyName;
  const choice = <SchemaChoiceContext value={{ id: choiceId, label: discriminator }}>{selector}</SchemaChoiceContext>;
  const rootChoice = isValidElement<{ fieldPathId: { path: unknown[] } }>(optionSchemaField)
    && optionSchemaField.props.fieldPathId?.path.every((part) => part === 'XxxOf');
  if (discriminator) {
    const discriminatorSchema = withConfigurationFieldHelp({}, discriminator, []);
    const label = localizedLabel(discriminatorSchema, discriminator, i18n.language, t);
    return (
      <div className='schema-form__discriminated'>
        <Field className='schema-form__field' hidden={rootChoice && dialogLayout !== null && dialogLayout.groupForField(discriminator) !== dialogLayout.active}>
          <div className='schema-form__label'>
            <FieldLabel htmlFor={`${choiceId}-variant`}>{label}</FieldLabel>
            <SchemaFieldHelp schema={discriminatorSchema} label={label} />
          </div>
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
};
