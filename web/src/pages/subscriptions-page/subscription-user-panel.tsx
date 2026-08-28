import { useCallback, useEffect, useMemo, useState } from 'react';

import type { SubscriptionNodeCatalog, SubscriptionUser } from '@/api/api-client';

import { useApiClient } from '@/api/api-client-context';
import { ActionError } from '@/components/action-error';
import { describeRequestError, ErrorNotice } from '@/components/error-notice';

export function SubscriptionUserPanel() {
  const client = useApiClient();
  const [users, setUsers] = useState<SubscriptionUser[] | null>(null);
  const [catalog, setCatalog] = useState<SubscriptionNodeCatalog | null>(null);
  const [selectedUser, setSelectedUser] = useState<SubscriptionUser | null>(null);
  const [grants, setGrants] = useState<Set<string>>(() => new Set());
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [loadError, setLoadError] = useState<unknown>(null);
  const [actionError, setActionError] = useState('');
  const [busy, setBusy] = useState(false);

  const load = useCallback(async (signal?: AbortSignal) => {
    try {
      setLoadError(null);
      const userPage = await client.listSubscriptionUsers({ limit: 100 }, signal);
      if (!signal?.aborted) {
        setUsers(userPage.items);
        try {
          setCatalog(await client.getSubscriptionNodeCatalog(signal));
        } catch {
          setCatalog({ applied_bundle_id: '', nodes: [], diagnostics: [] });
        }
      }
    } catch (error) {
      if (!signal?.aborted) setLoadError(error);
    }
  }, [client]);

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, [load]);

  const sourceGroups = useMemo(() => {
    const groups = new Map<string, string[]>();
    for (const node of catalog?.nodes ?? []) {
      groups.set(node.source_id, [...(groups.get(node.source_id) ?? []), node.key]);
    }
    return groups;
  }, [catalog]);

  async function create() {
    if (name.trim() === '') {
      setActionError('Enter a user name.');
      return;
    }
    try {
      setBusy(true);
      setActionError('');
      await client.createSubscriptionUser({ name: name.trim(), description: description.trim(), enabled: true });
      setName('');
      setDescription('');
      await load();
    } catch (error) {
      setActionError(describeRequestError(error));
    } finally {
      setBusy(false);
    }
  }

  async function selectUser(user: SubscriptionUser) {
    try {
      setBusy(true);
      setActionError('');
      const current = await client.getSubscriptionUserGrants(user.id);
      setSelectedUser(current.user);
      setGrants(new Set(current.grants));
    } catch (error) {
      setActionError(describeRequestError(error));
    } finally {
      setBusy(false);
    }
  }

  async function saveGrants() {
    if (!selectedUser) return;
    try {
      setBusy(true);
      setActionError('');
      const saved = await client.replaceSubscriptionUserGrants(
        selectedUser.id,
        [...grants].sort(),
        selectedUser.updated_at,
      );
      setSelectedUser(saved.user);
      setGrants(new Set(saved.grants));
      await load();
    } catch (error) {
      setActionError(describeRequestError(error));
    } finally {
      setBusy(false);
    }
  }

  async function toggleUser(user: SubscriptionUser) {
    try {
      setBusy(true);
      await client.updateSubscriptionUser(user.id, {
        name: user.name,
        description: user.description,
        enabled: !user.enabled,
      }, user.updated_at);
      await load();
    } catch (error) {
      setActionError(describeRequestError(error));
    } finally {
      setBusy(false);
    }
  }

  async function remove(user: SubscriptionUser) {
    try {
      setBusy(true);
      setActionError('');
      await client.deleteSubscriptionUser(user.id, user.updated_at);
      if (selectedUser?.id === user.id) {
        setSelectedUser(null);
        setGrants(new Set());
      }
      await load();
    } catch (error) {
      setActionError(describeRequestError(error));
    } finally {
      setBusy(false);
    }
  }

  function toggleKeys(keys: string[]) {
    const next = new Set(grants);
    const enable = keys.some((key) => !next.has(key));
    for (const key of keys) enable ? next.add(key) : next.delete(key);
    setGrants(next);
  }

  return (
    <section className='subscription-panel' aria-labelledby='subscription-users-title'>
      <div className='subscription-panel__heading'>
        <div>
          <p className='eyebrow'>01 / Authorization</p>
          <h2 id='subscription-users-title'>Users and node grants</h2>
          <p>
            Default deny. Selecting a whole source copies only its current node keys.
            {' '}
            Deleting a user also deletes its tokens.
          </p>
        </div>
        <span className='count-label'>
          {users?.length ?? 0}
          {' '}
          users
        </span>
      </div>
      <ActionError message={actionError} title='User or grant change failed' />
      {loadError === null ? null : <ErrorNotice error={loadError} title='Could not load authorization state' />}

      <form className='token-issuer' onSubmit={(event) => {
        event.preventDefault();
        void create();
      }}>
        <div className='field-group'>
          <label htmlFor='subscription-user-name'>Name</label>
          <input id='subscription-user-name' onChange={(event) => setName(event.target.value)} value={name} />
        </div>
        <div className='field-group'>
          <label htmlFor='subscription-user-description'>Description</label>
          <input id='subscription-user-description' onChange={(event) => setDescription(event.target.value)} value={description} />
        </div>
        <button className='button button--primary' disabled={busy} type='submit'>Create user</button>
      </form>

      {users && users.length > 0
        ? (
            <div className='entity-table-wrap'>
              <table className='data-table'>
                <thead>
                  <tr>
                    <th>User</th>
                    <th>State</th>
                    <th>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {users.map((user) => (
                    <tr key={user.id}>
                      <td>
                        <strong>{user.name}</strong>
                        <small className='table-subline'>{user.description || user.id}</small>
                      </td>
                      <td>{user.enabled ? 'Enabled' : 'Disabled'}</td>
                      <td>
                        <div className='table-actions'>
                          <button className='text-button' disabled={busy} onClick={() => void selectUser(user)} type='button'>Permissions</button>
                          <button className='text-button' disabled={busy} onClick={() => void toggleUser(user)} type='button'>{user.enabled ? 'Disable' : 'Enable'}</button>
                          <button className='text-button text-button--danger' disabled={busy} onClick={() => void remove(user)} type='button'>Delete</button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )
        : null}

      {selectedUser && catalog
        ? (
            <div className='control-form'>
              <div className='section-heading'>
                <div>
                  <p className='eyebrow'>Permission matrix</p>
                  <h3>{selectedUser.name}</h3>
                </div>
                <span>
                  {grants.size}
                  {' '}
                  granted
                </span>
              </div>
              {[...sourceGroups].map(([sourceID, keys]) => (
                <div className='field-group' key={sourceID}>
                  <button className='text-button' onClick={() => toggleKeys(keys)} type='button'>
                    Toggle current source:
                    {sourceID}
                  </button>
                  {catalog.nodes.filter((node) => node.source_id === sourceID).map((node) => (
                    <label className='check-field' key={node.key}>
                      <input checked={grants.has(node.key)} onChange={() => toggleKeys([node.key])} type='checkbox' />
                      <span>
                        <strong>{node.tag}</strong>
                        <small>
                          {node.type}
                          {node.credential ? ` / ${node.credential}` : ''}
                        </small>
                      </span>
                    </label>
                  ))}
                </div>
              ))}
              <div className='inline-actions'>
                <button className='button button--primary' disabled={busy} onClick={() => void saveGrants()} type='button'>Save permissions</button>
                <button className='button button--secondary' onClick={() => setSelectedUser(null)} type='button'>Close</button>
              </div>
            </div>
          )
        : null}
    </section>
  );
}
