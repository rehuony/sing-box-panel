import type { RJSFSchema } from '@rjsf/utils';

import { useTranslation } from 'react-i18next';

import type { ReviewedSchemaResolution } from '@/schemas/resolve-reviewed-schema';

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
const mainFields = new Set([
  'tag', 'server', 'server_port', 'server_ports', 'realm', 'username', 'password', 'uuid',
  'method', 'security', 'flow', 'version', 'alter_id', 'private_key', 'private_key_path',
  'private_key_passphrase', 'psk', 'plugin', 'plugin_opts',
]);
const associatedFields = new Set(['tls', 'transport', 'multiplex', 'obfs', 'udp_over_tcp']);
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
  const basic = keys.filter((key) => !dialFields.has(key) && !associatedFields.has(key));
  const primary = basic.filter((key) => mainFields.has(key) || resolved.required?.includes(key));
  const advanced = basic.filter((key) => !primary.includes(key));
  const ordered = [
    ...['tag', 'server', 'server_port'].filter((key) => primary.includes(key)),
    ...primary.filter((key) => !['tag', 'server', 'server_port'].includes(key)),
  ];
  function fields(names: string[]) {
    const subset: RJSFSchema = {
      type: 'object',
      properties: Object.fromEntries(names.map((key) => [key, properties[key]])),
      required: resolved.required?.filter((key) => names.includes(key)),
    };
    const schemaUI = uiSchemaFromPanel(subset, [], resolution.schema, data);
    return (
      <SchemaSectionForm
        basePointer='/outbounds/0'
        data={data}
        disabled={disabled}
        onChange={(change) => {
          const changed = change({ outbounds: [data] });
          update((changed.outbounds as Record<string, unknown>[])[0]);
        }}
        resolution={resolution}
        schema={subset}
        uiSchema={{
          ...schemaUI,
          'ui:order': names,
          ...(names.includes('tls') && tlsRequired
            ? {
                tls: {
                  ...schemaUI.tls,
                  'ui:order': ['enabled', 'server_name', 'insecure', 'alpn', '*'],
                  'enabled': { 'ui:disabled': true, 'ui:help': t('subscriptions.nodes.tlsRequired') },
                },
              }
            : {}),
          ...(names.includes('udp_over_tcp') && optionEnabled(data.multiplex)
            ? { udp_over_tcp: { 'ui:disabled': true } }
            : {}),
          ...(names.includes('multiplex')
            && protocol === 'shadowsocks'
            && optionEnabled(data.udp_over_tcp)
            ? { multiplex: { 'ui:disabled': true } }
            : {}),
        }}
      />
    );
  }
  return (
    <div className='subscription-node-form'>
      <label className='subscription-node-protocol'>
        <span>{t('subscriptions.nodes.protocol')}</span>
        <select
          disabled={disabled}
          onChange={(event) => {
            const type = event.target.value;
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
        >
          {!types.includes(String(data.type)) && (
            <option value={String(data.type ?? '')}>{String(data.type ?? '')}</option>
          )}
          {types.map((type) => (
            <option key={type} value={type}>
              {type}
            </option>
          ))}
        </select>
      </label>
      {protocol === 'hysteria2' && (
        <label className='subscription-node-protocol'>
          <span>{t('subscriptions.nodes.endpointMode')}</span>
          <select
            disabled={disabled}
            value={endpointMode}
            onChange={(event) => {
              const value = { ...data };
              delete value.server_port;
              delete value.server_ports;
              delete value.realm;
              if (event.target.value === 'realm') {
                delete value.server;
                value.realm = {};
              } else if (event.target.value === 'range') {
                value.server_ports = ['443:8443'];
              } else {
                value.server_port = 443;
              }
              update(value);
            }}
          >
            <option value='single'>{t('subscriptions.nodes.singlePort')}</option>
            <option value='range'>{t('subscriptions.nodes.portHopping')}</option>
            <option value='realm'>Realm</option>
          </select>
        </label>
      )}
      {protocol === 'ssh' && (
        <label className='subscription-node-protocol'>
          <span>{t('subscriptions.nodes.authentication')}</span>
          <select
            disabled={disabled}
            value={sshMode}
            onChange={(event) => {
              const value = { ...data };
              delete value.password;
              delete value.private_key;
              delete value.private_key_path;
              if (event.target.value === 'key') {
                value.private_key = [''];
              } else if (event.target.value === 'file') {
                value.private_key_path = '';
              } else {
                value.password = '';
                delete value.private_key_passphrase;
              }
              update(value);
            }}
          >
            <option value='password'>{t('subscriptions.nodes.passwordAuth')}</option>
            <option value='key'>{t('subscriptions.nodes.privateKeyAuth')}</option>
            <option value='file'>{t('subscriptions.nodes.privateKeyFile')}</option>
          </select>
        </label>
      )}
      {fields(ordered)}
      {advanced.length > 0 && (
        <details className='subscription-node-options'>
          <summary>{t('subscriptions.nodes.options.advanced')}</summary>
          {fields(advanced)}
        </details>
      )}
      {[...associatedFields]
        .filter((key) => keys.includes(key))
        .map((key) => (
          <details
            className='subscription-node-options'
            key={key}
            open={data[key] !== undefined || undefined}
          >
            <summary>{t(`subscriptions.nodes.options.${key}`)}</summary>
            {fields([key])}
          </details>
        ))}
      {keys.some((key) => dialFields.has(key)) && (
        <details className='subscription-node-options'>
          <summary>{t('subscriptions.nodes.options.dial')}</summary>
          <label className='subscription-node-protocol'>
            <span title={t('subscriptions.nodes.detourHelp')}>
              {t('subscriptions.nodes.detour')}
            </span>
            <select
              disabled={disabled}
              value={String(data.detour ?? '')}
              onChange={(event) => {
                const updated = { ...data };
                if (event.target.value) updated.detour = event.target.value;
                else delete updated.detour;
                onChange(encodeCanonicalValue(updated, 2));
              }}
            >
              <option value=''>{t('subscriptions.nodes.noDetour')}</option>
              {Boolean(data.detour) && !candidates.includes(String(data.detour)) && (
                <option value={String(data.detour)}>{String(data.detour)}</option>
              )}
              {candidates
                .filter((value) => value !== data.tag)
                .map((value) => (
                  <option key={value} value={value}>
                    {value}
                  </option>
                ))}
            </select>
          </label>
          <fieldset
            disabled={disabled || Boolean(data.detour)}
            title={data.detour ? t('subscriptions.nodes.detourHelp') : undefined}
          >
            {fields(
              keys.filter((key) => key !== 'detour' && key !== 'network' && dialFields.has(key)),
            )}
          </fieldset>
          {keys.includes('network') && fields(['network'])}
        </details>
      )}
    </div>
  );
}

function optionEnabled(value: unknown): boolean {
  return (
    value === true
    || Boolean(value && typeof value === 'object' && 'enabled' in value && value.enabled === true)
  );
}
