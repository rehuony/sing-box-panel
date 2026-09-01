/* eslint-disable react-refresh/only-export-components */
// RJSF requires one registry module that exports component maps.
import type { ComponentType } from 'react';
import type {
  BaseInputTemplateProps,
  FieldTemplateProps,
  IconButtonProps,
  ObjectFieldTemplateProps,
  RJSFSchema,
  TemplatesType,
  WidgetProps,
} from '@rjsf/utils';

import { useTranslation } from 'react-i18next';
import {
  ArrowDown,
  ArrowUp,
  Copy,
  Plus,
  Trash2,
  X,
} from 'lucide-react';
import {
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

function localizedLabel(schema: RJSFSchema, fallback: string, language: string): string {
  const labels = metadata(schema).label;
  return labels?.[language] ?? labels?.en ?? schema.title ?? fallback;
}

function PanelFieldTemplate(props: FieldTemplateProps) {
  const { i18n } = useTranslation();
  const {
    children, description, disabled, displayLabel, errors, fieldPathId, hidden, id,
    label, rawErrors, required, schema,
  } = props;
  if (hidden) return children;
  if (fieldPathId.path.length === 0 || schema.type === 'object' || schema.type === 'array') {
    return children;
  }
  const resolvedLabel = localizedLabel(schema, label, i18n.language);
  return (
    <Field
      className='schema-form__field'
      data-disabled={disabled || undefined}
      data-invalid={rawErrors !== undefined && rawErrors.length > 0 ? true : undefined}
    >
      {displayLabel === false
        ? null
        : (
            <FieldLabel htmlFor={id}>
              {resolvedLabel}
              {required ? <span aria-hidden='true'>*</span> : null}
            </FieldLabel>
          )}
      {children}
      {description ? <FieldDescription>{description}</FieldDescription> : null}
      {errors ? <FieldError>{errors}</FieldError> : null}
    </Field>
  );
}

function PanelObjectFieldTemplate(props: ObjectFieldTemplateProps) {
  const { i18n } = useTranslation();
  const {
    description, fieldPathId, properties, schema, title,
  } = props;
  const visible = properties.filter((property) => !property.hidden);
  const resolvedTitle = localizedLabel(schema, title, i18n.language);
  const root = fieldPathId.path.length === 0;
  return (
    <fieldset className={root ? 'schema-form__root' : 'schema-form__object'} id={fieldPathId.$id}>
      {!root && resolvedTitle !== '' ? <FieldLegend>{resolvedTitle}</FieldLegend> : null}
      {description ? <FieldDescription>{description}</FieldDescription> : null}
      <FieldGroup className='schema-form__grid'>
        {visible.map((property) => <div key={property.name}>{property.content}</div>)}
      </FieldGroup>
    </fieldset>
  );
}

function PanelBaseInputTemplate(props: BaseInputTemplateProps) {
  const {
    autofocus, disabled, htmlName, id, onBlur, onChange, onChangeOverride, onFocus,
    options, readonly, schema, type, value,
  } = props;
  const inputProps = getInputProps(schema, type, options);
  const inputValue = value === undefined || value === null ? '' : String(value);
  return (
    <Input
      {...inputProps}
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
  const { i18n } = useTranslation();
  const checked = props.value === true;
  return (
    <Switch
      aria-label={localizedLabel(props.schema, props.label, i18n.language)}
      checked={checked}
      disabled={props.disabled || props.readonly}
      id={props.id}
      onCheckedChange={(next) => props.onChange(next)}
    />
  );
}

function PanelSelectWidget(props: WidgetProps) {
  const { i18n } = useTranslation();
  const enumOptions = props.options.enumOptions ?? [];
  const optionValueFormat = getOptionValueFormat(props.options);
  const encodedItems = enumOptions.map((option, index) => ({
    label: option.label,
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
        aria-label={localizedLabel(props.schema, props.label, i18n.language)}
        className='w-full'
        id={props.id}
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
  const { i18n } = useTranslation();
  return (
    <Textarea
      aria-label={localizedLabel(props.schema, props.label, i18n.language)}
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
        variant={destructive ? 'destructive' : 'ghost'}
      >
        <Icon aria-hidden data-icon='inline-start' />
      </Button>
    );
  };
}

export const panelRJSFTemplates: Partial<TemplatesType> = {
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
