import type { JsonObject } from '@/api/api-client';

import type { CanonicalDraft } from './use-canonical-configuration';

export interface ConfigurationFieldsProps {
  draft: CanonicalDraft;
  onChange: (change: (draft: CanonicalDraft) => CanonicalDraft) => void;
}

function objectValue(value: unknown): JsonObject {
  return value !== null && !Array.isArray(value) && typeof value === 'object'
    ? value as JsonObject
    : {};
}

function stringValue(value: unknown): string {
  return typeof value === 'string' ? value : '';
}

export function ConfigurationFields({ draft, onChange }: ConfigurationFieldsProps) {
  const configuration = draft.configuration;
  const log = objectValue(configuration.log);
  const dns = objectValue(configuration.dns);
  const route = objectValue(configuration.route);
  const ntp = objectValue(configuration.ntp);
  const experimental = objectValue(configuration.experimental);
  const clashAPI = objectValue(experimental.clash_api);

  function updateSection(section: string, key: string, value: unknown) {
    onChange((current) => {
      const nextConfiguration = { ...current.configuration };
      const nextSection = { ...objectValue(nextConfiguration[section]) };
      if (value === '') delete nextSection[key];
      else nextSection[key] = value;
      nextConfiguration[section] = nextSection;
      return { ...current, configuration: nextConfiguration };
    });
  }

  function updateClashAPI(key: string, value: unknown) {
    onChange((current) => {
      const nextConfiguration = { ...current.configuration };
      const nextExperimental = { ...objectValue(nextConfiguration.experimental) };
      const nextClashAPI = { ...objectValue(nextExperimental.clash_api) };
      if (value === '') delete nextClashAPI[key];
      else nextClashAPI[key] = value;
      nextExperimental.clash_api = nextClashAPI;
      nextConfiguration.experimental = nextExperimental;
      return { ...current, configuration: nextConfiguration };
    });
  }

  return (
    <section className='editor-panel' aria-labelledby='global-settings-title'>
      <div className='section-heading'>
        <div>
          <p className='eyebrow'>Structured global settings</p>
          <h2 id='global-settings-title'>Shared configuration intent</h2>
        </div>
        <span>preserves unshown fields</span>
      </div>
      <div className='form-grid'>
        <div className='field-group'>
          <label htmlFor='config-log-level'>Log level</label>
          <select id='config-log-level' onChange={(event) => updateSection('log', 'level', event.target.value)} value={stringValue(log.level)}>
            <option value=''>Core default</option>
            <option value='trace'>Trace</option>
            <option value='debug'>Debug</option>
            <option value='info'>Info</option>
            <option value='warn'>Warn</option>
            <option value='error'>Error</option>
            <option value='fatal'>Fatal</option>
            <option value='panic'>Panic</option>
          </select>
        </div>
        <div className='field-group'>
          <label htmlFor='config-dns-final'>DNS final server tag</label>
          <input id='config-dns-final' onChange={(event) => updateSection('dns', 'final', event.target.value)} value={stringValue(dns.final)} />
        </div>
        <div className='field-group'>
          <label htmlFor='config-route-final'>Route final outbound tag</label>
          <input id='config-route-final' onChange={(event) => updateSection('route', 'final', event.target.value)} value={stringValue(route.final)} />
        </div>
        <div className='field-group'>
          <label htmlFor='config-ntp-server'>NTP server</label>
          <input id='config-ntp-server' onChange={(event) => updateSection('ntp', 'server', event.target.value)} value={stringValue(ntp.server)} />
        </div>
        <label className='checkbox-field' htmlFor='config-ntp-enabled'>
          <input checked={ntp.enabled === true} id='config-ntp-enabled' onChange={(event) => updateSection('ntp', 'enabled', event.target.checked)} type='checkbox' />
          <span>Enable NTP synchronization</span>
        </label>
        <div className='field-group'>
          <label htmlFor='config-clash-controller'>Clash API controller</label>
          <input id='config-clash-controller' onChange={(event) => updateClashAPI('external_controller', event.target.value)} placeholder='127.0.0.1:9090' value={stringValue(clashAPI.external_controller)} />
        </div>
        <div className='field-group'>
          <label htmlFor='config-clash-secret'>Clash API secret</label>
          <input autoComplete='new-password' id='config-clash-secret' onChange={(event) => updateClashAPI('secret', event.target.value)} type='password' value={stringValue(clashAPI.secret)} />
        </div>
      </div>
    </section>
  );
}
