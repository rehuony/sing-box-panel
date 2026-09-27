import type { ComponentProps } from 'react';
import type { RJSFSchema } from '@rjsf/utils';

import { useId } from 'react';
import { useTranslation } from 'react-i18next';

import type { ReviewedSchemaResolution } from '@/schemas/resolve-reviewed-schema';

import { InfoTooltip } from '@/components/info-tooltip';
import { SelectField } from '@/components/select-field';
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field';

import { SchemaSectionForm } from '../configuration-page/schema-section-form';
import { encodeCanonicalValue } from '../configuration-page/use-canonical-configuration';
import {
  resolvedSchema,
  schemaDiscriminatorValues,
  schemaProperties,
  uiSchemaFromPanel,
} from '../configuration-page/schema-ui';

const dialFields = new Set([
  'network',
  'detour',
  'bind_interface',
  'inet4_bind_address',
  'inet6_bind_address',
  'bind_address_no_port',
  'routing_mark',
  'reuse_addr',
  'connect_timeout',
  'tcp_fast_open',
  'tcp_multi_path',
  'udp_fragment',
  'domain_resolver',
  'network_strategy',
  'network_type',
  'fallback_network_type',
  'fallback_delay',
  'netns',
  'protect_path',
  'tcp_keep_alive',
  'tcp_keep_alive_interval',
  'tcp_keep_alive_idle',
  'disable_tcp_keep_alive',
]);
const nonNodeTypes = new Set(['direct', 'block', 'dns', 'selector', 'urltest', 'bridge', 'tor']);

interface NodeFormProps {
  disabled: boolean;
  schema: RJSFSchema;
  candidates: string[];
  data: Record<string, unknown>;
  onChange: (raw: string) => void;
  resolution: ReviewedSchemaResolution;
}

export function SubscriptionNodeForm({
  data,
  candidates,
  disabled,
  schema,
  resolution,
  onChange,
}: NodeFormProps) {
  const { t } = useTranslation();
  const protocol = String(data.type ?? '');
  const quic = ['hysteria', 'hysteria2', 'tuic'].includes(protocol);
  const tlsRequired = quic || ['anytls', 'naive'].includes(protocol);
  const resolved = resolvedSchema(schema, resolution.schema, data);
  const properties = schemaProperties(schema, resolution.schema, data);
  const tls = data.tls && typeof data.tls === 'object' ? (data.tls as Record<string, unknown>) : {};
  if (properties.tls) {
    const source = properties.tls;
    const tlsProperties = schemaProperties(source, resolution.schema, tls);
    // Unsupported existing options remain in raw JSON; hiding an inapplicable
    // control must not silently delete imported values.
    if (quic) {
      for (const key of [
        'reality',
        'utls',
        'fragment',
        'fragment_fallback_delay',
        'record_fragment',
      ]) delete tlsProperties[key];
    }
    properties.tls = { type: 'object', properties: tlsProperties };
  }
  const types = schemaDiscriminatorValues(schema, resolution.schema, 'type').filter(
    (type) => !nonNodeTypes.has(type),
  );
  const endpointMode = data.realm ? 'realm' : data.server_ports !== undefined ? 'range' : 'single';
  const sshMode
    = data.private_key_path !== undefined
      ? 'file'
      : data.private_key !== undefined
        ? 'key'
        : 'password';
  const keys = Object.keys(properties).filter((key) => {
    if (key === 'type') return false;
    if (protocol === 'hysteria2') {
      if (key === 'realm' && endpointMode !== 'realm') return false;
      if (key === 'server_ports' && endpointMode !== 'range') return false;
      if (key === 'server_port' && endpointMode !== 'single') return false;
      if (key === 'server' && endpointMode === 'realm') return false;
    }
    if (protocol === 'ssh') {
      if (key === 'password' && sshMode !== 'password') return false;
      if (key === 'private_key' && sshMode !== 'key') return false;
      if (key === 'private_key_path' && sshMode !== 'file') return false;
      if (key === 'private_key_passphrase' && sshMode === 'password') return false;
    }
    return true;
  });
  function update(value: Record<string, unknown>) {
    onChange(
      encodeCanonicalValue(
        tlsRequired
          ? { ...value, tls: { ...tls, ...((value.tls as object) ?? {}), enabled: true } }
          : value,
        2,
      ),
    );
  }
  // Keep the protocol in the schema for protocol-specific rules and generators.
  const sectionKeys = properties.type ? ['type', ...keys] : keys;
  const formSchema: RJSFSchema = {
    type: 'object',
    properties: Object.fromEntries(sectionKeys.map((key) => [key, properties[key]])),
    required: resolved.required?.filter((key) => sectionKeys.includes(key)),
  };
  const schemaUI = uiSchemaFromPanel(formSchema, [], resolution.schema, data);
  return (
    <SchemaSectionForm
      dialogLayout
      basePointer='/outbounds/0'
      data={data}
      disabled={disabled}
      onChange={(change) => {
        const changed = change({ outbounds: [data] });
        update((changed.outbounds as Record<string, unknown>[])[0]);
      }}
      resolution={resolution}
      schema={formSchema}
      uiSchema={{
        ...schemaUI,
        ...Object.fromEntries(keys
          .filter((key) => dialFields.has(key) && key !== 'detour' && key !== 'network')
          .map((key) => [key, { ...schemaUI[key], 'ui:disabled': disabled || Boolean(data.detour) }])),
        type: { 'ui:widget': 'hidden' },
        detour: { 'ui:widget': 'hidden' },
        ...(tlsRequired && properties.tls
          ? {
              tls: {
                ...schemaUI.tls,
                'ui:order': ['enabled', 'server_name', 'insecure', 'alpn', '*'],
                'enabled': { 'ui:disabled': true, 'ui:help': t('subscriptions.nodes.tlsRequired') },
              },
            }
          : {}),
        ...(optionEnabled(data.multiplex) ? { udp_over_tcp: { 'ui:disabled': true } } : {}),
        ...(protocol === 'shadowsocks' && optionEnabled(data.udp_over_tcp)
          ? { multiplex: { 'ui:disabled': true } }
          : {}),
      }}
      dialogSectionContent={{
        basic: (
          <FieldGroup className='schema-form schema-form__grid subscription-node-form__controls'>
            <NodeSelectField
              label={t('subscriptions.nodes.protocol')}
              disabled={disabled}
              onValueChange={(type) => {
                const updated: Record<string, unknown> = { ...data, type };
                const nextProperties = schemaProperties(schema, resolution.schema, updated);
                for (const key of Object.keys(properties)) {
                  if (!nextProperties[key]) delete updated[key];
                }
                if (['hysteria', 'hysteria2', 'tuic', 'anytls', 'naive'].includes(type)) {
                  const nextTLS: Record<string, unknown> = { ...tls, enabled: true };
                  if (['hysteria', 'hysteria2', 'tuic'].includes(type)) {
                    for (const key of [
                      'reality',
                      'utls',
                      'fragment',
                      'fragment_fallback_delay',
                      'record_fragment',
                    ]) delete nextTLS[key];
                  }
                  updated.tls = nextTLS;
                }
                onChange(encodeCanonicalValue(updated, 2));
              }}
              value={String(data.type ?? '')}
              items={[
                ...(!types.includes(String(data.type)) ? [String(data.type ?? '')] : []),
                ...types,
              ].map((value) => ({ value, label: value }))}
            />
            {protocol === 'hysteria2' && (
              <NodeSelectField
                label={t('subscriptions.nodes.endpointMode')}
                disabled={disabled}
                value={endpointMode}
                onValueChange={(mode) => {
                  const value = { ...data };
                  delete value.server_port;
                  delete value.server_ports;
                  delete value.realm;
                  if (mode === 'realm') {
                    delete value.server;
                    value.realm = {};
                  } else if (mode === 'range') {
                    value.server_ports = ['443:8443'];
                  } else {
                    value.server_port = 443;
                  }
                  update(value);
                }}
                items={[
                  { value: 'single', label: t('subscriptions.nodes.singlePort') },
                  { value: 'range', label: t('subscriptions.nodes.portHopping') },
                  { value: 'realm', label: 'Realm' },
                ]}
              />
            )}
          </FieldGroup>
        ),
        authentication: protocol === 'ssh'
          ? (
              <FieldGroup className='schema-form schema-form__grid subscription-node-form__controls'>
                <NodeSelectField
                  label={t('subscriptions.nodes.authentication')}
                  disabled={disabled}
                  value={sshMode}
                  onValueChange={(mode) => {
                    const value = { ...data };
                    delete value.password;
                    delete value.private_key;
                    delete value.private_key_path;
                    if (mode === 'key') {
                      value.private_key = [''];
                    } else if (mode === 'file') {
                      value.private_key_path = '';
                    } else {
                      value.password = '';
                      delete value.private_key_passphrase;
                    }
                    update(value);
                  }}
                  items={[
                    { value: 'password', label: t('subscriptions.nodes.passwordAuth') },
                    { value: 'key', label: t('subscriptions.nodes.privateKeyAuth') },
                    { value: 'file', label: t('subscriptions.nodes.privateKeyFile') },
                  ]}
                />
              </FieldGroup>
            )
          : undefined,
        connection: keys.includes('detour')
          ? (
              <FieldGroup className='schema-form schema-form__grid subscription-node-form__controls'>
                <NodeSelectField
                  label={t('subscriptions.nodes.detour')}
                  help={t('subscriptions.nodes.detourHelp')}
                  disabled={disabled}
                  value={String(data.detour ?? '')}
                  onValueChange={(value) => {
                    const updated = { ...data };
                    if (value) updated.detour = value;
                    else delete updated.detour;
                    onChange(encodeCanonicalValue(updated, 2));
                  }}
                  items={[
                    { value: '', label: t('subscriptions.nodes.noDetour') },
                    ...(data.detour && !candidates.includes(String(data.detour))
                      ? [{ value: String(data.detour), label: String(data.detour) }]
                      : []),
                    ...candidates.filter((value) => value !== data.tag).map((value) => ({ value, label: value })),
                  ]}
                />
              </FieldGroup>
            )
          : undefined,
      }}
    />
  );
}

function NodeSelectField({ label, help, ...props }: ComponentProps<typeof SelectField<string>> & {
  label: string;
  help?: string;
}) {
  const id = useId();
  const { t } = useTranslation();
  return (
    <Field className='schema-form__field' data-disabled={props.disabled || undefined}>
      <div className='schema-form__label'>
        <FieldLabel htmlFor={id}>{label}</FieldLabel>
        {help && <InfoTooltip label={t('configuration.general.fieldHelp', { field: label })}>{help}</InfoTooltip>}
      </div>
      <div className='schema-form__control'>
        <SelectField {...props} className='w-full' id={id} aria-label={label} />
      </div>
    </Field>
  );
}

function optionEnabled(value: unknown): boolean {
  return (
    value === true
    || Boolean(value && typeof value === 'object' && 'enabled' in value && value.enabled === true)
  );
}
