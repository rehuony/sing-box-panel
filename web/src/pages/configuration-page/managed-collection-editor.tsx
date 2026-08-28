import type { ManagedConfigurationEntry } from '@/api/api-client';

import type { CanonicalDraft } from './use-canonical-configuration';

type ManagedCollection = 'inbounds' | 'outbounds';

export interface ManagedCollectionEditorProps {
  draft: CanonicalDraft;
  collection: ManagedCollection;
  onChange: (change: (draft: CanonicalDraft) => CanonicalDraft) => void;
}

const inboundTypes = [
  'mixed', 'socks', 'http', 'shadowsocks', 'vmess', 'trojan', 'naive',
  'hysteria', 'shadowtls', 'vless', 'tuic', 'hysteria2', 'anytls', 'snell',
] as const;
const outboundTypes = [
  'direct', 'block', 'dns', 'socks', 'http', 'shadowsocks', 'vmess', 'trojan',
  'wireguard', 'hysteria', 'shadowtls', 'vless', 'tuic', 'hysteria2', 'anytls',
  'tor', 'ssh', 'selector', 'urltest', 'naive',
] as const;

function entries(value: unknown): ManagedConfigurationEntry[] {
  return Array.isArray(value) ? value as ManagedConfigurationEntry[] : [];
}

function text(value: unknown): string {
  return value === undefined || value === null ? '' : String(value);
}

function nextIdentifier(items: ManagedConfigurationEntry[], prefix: string): string {
  const used = new Set(items.map((item) => item._panel.id));
  for (let suffix = 1; suffix < 10_000; suffix += 1) {
    const candidate = `${prefix}-${suffix}`;
    if (!used.has(candidate)) return candidate;
  }
  throw new Error(`No available ${prefix} identifier.`);
}

export function ManagedCollectionEditor({ collection, draft, onChange }: ManagedCollectionEditorProps) {
  const items = entries(draft.configuration[collection]);
  const types = collection === 'inbounds' ? inboundTypes : outboundTypes;
  const singular = collection === 'inbounds' ? 'inbound' : 'outbound';

  function replace(next: ManagedConfigurationEntry[]) {
    onChange((current) => ({
      ...current,
      configuration: { ...current.configuration, [collection]: next },
    }));
  }

  function update(index: number, key: string, value: unknown) {
    const next = [...items];
    const item = { ...next[index] };
    if (value === '') delete item[key];
    else item[key] = value;
    next[index] = item;
    replace(next);
  }

  function updateMarker(index: number, key: 'id' | 'enabled', value: string | boolean) {
    const next = [...items];
    next[index] = { ...next[index], _panel: { ...next[index]._panel, [key]: value } };
    replace(next);
  }

  function add() {
    const id = nextIdentifier(items, singular);
    replace([...items, {
      _panel: { id, enabled: true },
      tag: id,
      type: collection === 'inbounds' ? 'mixed' : 'direct',
    }]);
  }

  return (
    <section className='entity-section' aria-labelledby={`${collection}-title`}>
      <div className='section-heading'>
        <div>
          <p className='eyebrow'>Structured collection</p>
          <h2 id={`${collection}-title`}>{collection === 'inbounds' ? 'Inbounds' : 'Outbounds'}</h2>
        </div>
        <button className='button button--secondary' onClick={add} type='button'>
          Add
          {' '}
          {singular}
        </button>
      </div>
      {items.length === 0
        ? (
            <div className='empty-state empty-state--compact'>
              <strong>
                No managed
                {collection}
                .
              </strong>
              <p>The shared revision can remain empty; add one when the runtime needs it.</p>
            </div>
          )
        : (
            <div className='artifact-list'>
              {items.map((item, index) => (
                <article key={item._panel.id}>
                  <div className='form-grid'>
                    <div className='field-group'>
                      <label htmlFor={`${collection}-${index}-id`}>Panel ID</label>
                      <input id={`${collection}-${index}-id`} onChange={(event) => updateMarker(index, 'id', event.target.value)} pattern='[a-z][a-z0-9._-]*' required value={item._panel.id} />
                    </div>
                    <div className='field-group'>
                      <label htmlFor={`${collection}-${index}-tag`}>Tag</label>
                      <input id={`${collection}-${index}-tag`} onChange={(event) => update(index, 'tag', event.target.value)} value={text(item.tag)} />
                    </div>
                    <div className='field-group'>
                      <label htmlFor={`${collection}-${index}-type`}>Type</label>
                      <select id={`${collection}-${index}-type`} onChange={(event) => update(index, 'type', event.target.value)} value={text(item.type)}>
                        {types.map((type) => <option key={type} value={type}>{type}</option>)}
                      </select>
                    </div>
                    <label className='checkbox-field' htmlFor={`${collection}-${index}-enabled`}>
                      <input checked={item._panel.enabled} id={`${collection}-${index}-enabled`} onChange={(event) => updateMarker(index, 'enabled', event.target.checked)} type='checkbox' />
                      <span>Enabled in projections</span>
                    </label>
                    <div className='field-group'>
                      <label htmlFor={`${collection}-${index}-host`}>{collection === 'inbounds' ? 'Listen address' : 'Server address'}</label>
                      <input id={`${collection}-${index}-host`} onChange={(event) => update(index, collection === 'inbounds' ? 'listen' : 'server', event.target.value)} value={text(item[collection === 'inbounds' ? 'listen' : 'server'])} />
                    </div>
                    <div className='field-group'>
                      <label htmlFor={`${collection}-${index}-port`}>{collection === 'inbounds' ? 'Listen port' : 'Server port'}</label>
                      <input id={`${collection}-${index}-port`} max={65535} min={1} onChange={(event) => update(index, collection === 'inbounds' ? 'listen_port' : 'server_port', event.target.value === '' ? '' : Number(event.target.value))} type='number' value={text(item[collection === 'inbounds' ? 'listen_port' : 'server_port'])} />
                    </div>
                    <div className='field-group'>
                      <label htmlFor={`${collection}-${index}-username`}>Username</label>
                      <input autoComplete='off' id={`${collection}-${index}-username`} onChange={(event) => update(index, 'username', event.target.value)} value={text(item.username)} />
                    </div>
                    <div className='field-group'>
                      <label htmlFor={`${collection}-${index}-password`}>Password / credential</label>
                      <input autoComplete='new-password' id={`${collection}-${index}-password`} onChange={(event) => update(index, 'password', event.target.value)} type='password' value={text(item.password)} />
                    </div>
                  </div>
                  <button className='text-button text-button--danger' onClick={() => replace(items.filter((_, itemIndex) => itemIndex !== index))} type='button'>
                    Remove
                    {singular}
                  </button>
                </article>
              ))}
            </div>
          )}
    </section>
  );
}
