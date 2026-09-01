import { useTranslation } from 'react-i18next';
import { useEffect, useMemo, useRef, useState } from 'react';

import type { SubscriptionNodeCatalog, SubscriptionUser } from '@/api/api-client';

import { useApiClient } from '@/api/api-client-context';
import { ActionError } from '@/components/action-error';
import { describeRequestError, ErrorNotice } from '@/components/error-notice';
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';

interface SubscriptionUserPanelProps {
  catalogError: unknown;
  users: SubscriptionUser[] | null;
  catalog: SubscriptionNodeCatalog | null;
  reloadUsers: (signal?: AbortSignal) => Promise<void>;
  reloadCatalog: (signal?: AbortSignal) => Promise<void>;
}

export function SubscriptionUserPanel({
  catalog,
  catalogError,
  reloadCatalog,
  reloadUsers,
  users,
}: SubscriptionUserPanelProps) {
  const { i18n, t } = useTranslation();
  const client = useApiClient();
  const [selectedUser, setSelectedUser] = useState<SubscriptionUser | null>(null);
  const [grants, setGrants] = useState<Set<string>>(() => new Set());
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [actionError, setActionError] = useState('');
  const [deleteCandidateID, setDeleteCandidateID] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const selectionControllerRef = useRef<AbortController | null>(null);
  const selectionGenerationRef = useRef(0);

  const sourceGroups = useMemo(() => {
    const groups = new Map<string, string[]>();
    for (const node of catalog?.nodes ?? []) {
      groups.set(node.source_id, [...(groups.get(node.source_id) ?? []), node.key]);
    }
    return groups;
  }, [catalog]);
  const numberFormatter = useMemo(
    () => new Intl.NumberFormat(i18n.resolvedLanguage ?? i18n.language ?? 'en'),
    [i18n.language, i18n.resolvedLanguage],
  );

  useEffect(() => () => selectionControllerRef.current?.abort(), []);

  function closeSelectedUser() {
    if (busy) return;
    selectionGenerationRef.current += 1;
    selectionControllerRef.current?.abort();
    selectionControllerRef.current = null;
    setSelectedUser(null);
    setGrants(new Set());
  }

  async function create() {
    if (name.trim() === '') {
      setActionError(t('subscriptions.user.validation.name'));
      return;
    }
    try {
      setBusy(true);
      setActionError('');
      await client.createSubscriptionUser({ name: name.trim(), description: description.trim(), enabled: true });
      setName('');
      setDescription('');
      await reloadUsers();
    } catch (error) {
      setActionError(describeRequestError(error));
    } finally {
      setBusy(false);
    }
  }

  async function selectUser(user: SubscriptionUser) {
    selectionControllerRef.current?.abort();
    const controller = new AbortController();
    const generation = ++selectionGenerationRef.current;
    selectionControllerRef.current = controller;
    try {
      setBusy(true);
      setActionError('');
      const [exactUser, current] = await Promise.all([
        client.getSubscriptionUser(user.id, controller.signal),
        client.getSubscriptionUserGrants(user.id, controller.signal),
      ]);
      if (controller.signal.aborted || generation !== selectionGenerationRef.current) return;
      setSelectedUser(exactUser);
      setGrants(new Set(current.grants));
    } catch (error) {
      if (!controller.signal.aborted && generation === selectionGenerationRef.current) {
        setActionError(describeRequestError(error));
      }
    } finally {
      if (!controller.signal.aborted && generation === selectionGenerationRef.current) {
        selectionControllerRef.current = null;
        setBusy(false);
      }
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
      await reloadUsers();
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
      await reloadUsers();
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
      setDeleteCandidateID(null);
      await reloadUsers();
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
    <section className='subscription-panel' id='subscription-users' aria-labelledby='subscription-users-title'>
      <div className='subscription-panel__heading'>
        <div>
          <h2 id='subscription-users-title'>{t('subscriptions.user.title')}</h2>
          <p>{t('subscriptions.user.description')}</p>
        </div>
        <span className='count-label'>
          {t('subscriptions.user.loadedCount', {
            count: numberFormatter.format(users?.length ?? 0),
          })}
        </span>
      </div>
      <ActionError message={actionError} title={t('subscriptions.user.actionFailed')} />
      {catalogError === null
        ? null
        : (
            <div>
              <ErrorNotice error={catalogError} title={t('subscriptions.user.loadFailed')} />
              <button className='button button--secondary' onClick={() => void reloadCatalog()} type='button'>
                {t('common.retry')}
              </button>
            </div>
          )}

      <form className='token-issuer' onSubmit={(event) => {
        event.preventDefault();
        void create();
      }}>
        <div className='field-group'>
          <label htmlFor='subscription-user-name'>{t('subscriptions.common.name')}</label>
          <input id='subscription-user-name' onChange={(event) => setName(event.target.value)} value={name} />
        </div>
        <div className='field-group'>
          <label htmlFor='subscription-user-description'>{t('subscriptions.user.descriptionLabel')}</label>
          <input id='subscription-user-description' onChange={(event) => setDescription(event.target.value)} value={description} />
        </div>
        <button className='button button--primary' disabled={busy} type='submit'>{t('subscriptions.user.create')}</button>
      </form>

      {users === null
        ? <div className='inline-loading' aria-busy='true'>{t('subscriptions.common.loading')}</div>
        : null}
      {users && users.length > 0
        ? (
            <div className='entity-table-wrap'>
              <table className='data-table'>
                <thead>
                  <tr>
                    <th>{t('subscriptions.user.column.user')}</th>
                    <th>{t('subscriptions.common.state')}</th>
                    <th>{t('subscriptions.common.actions')}</th>
                  </tr>
                </thead>
                <tbody>
                  {users.map((user) => (
                    <tr key={user.id}>
                      <td data-label={t('subscriptions.user.column.user')}>
                        <strong>{user.name}</strong>
                        <small className='table-subline'>{user.description || user.id}</small>
                      </td>
                      <td data-label={t('subscriptions.common.state')}>
                        {user.enabled ? t('common.enabled') : t('common.disabled')}
                      </td>
                      <td data-label={t('subscriptions.common.actions')}>
                        {deleteCandidateID === user.id
                          ? (
                              <div
                                aria-label={t('subscriptions.user.delete.confirmationAria', { name: user.name })}
                                className='inline-confirmation'
                                role='group'
                              >
                                <span className='inline-confirmation__prompt'>{t('subscriptions.user.delete.prompt')}</span>
                                <button
                                  aria-label={t('subscriptions.user.delete.confirmAria', { name: user.name })}
                                  className='button button--danger button--small'
                                  disabled={busy}
                                  onClick={() => void remove(user)}
                                  type='button'
                                >
                                  {busy ? t('subscriptions.common.deleting') : t('subscriptions.common.confirmDelete')}
                                </button>
                                <button
                                  aria-label={t('subscriptions.user.delete.keepAria', { name: user.name })}
                                  autoFocus
                                  className='text-button'
                                  disabled={busy}
                                  onClick={() => setDeleteCandidateID(null)}
                                  type='button'
                                >
                                  {t('subscriptions.user.delete.keep')}
                                </button>
                              </div>
                            )
                          : (
                              <div className='table-actions'>
                                <button className='text-button' disabled={busy} onClick={() => void selectUser(user)} type='button'>{t('subscriptions.user.permissionsAction')}</button>
                                <button className='text-button' disabled={busy} onClick={() => void toggleUser(user)} type='button'>
                                  {user.enabled ? t('subscriptions.common.disable') : t('subscriptions.common.enable')}
                                </button>
                                <button
                                  aria-label={t('subscriptions.user.delete.actionAria', { name: user.name })}
                                  className='text-button text-button--danger'
                                  disabled={busy}
                                  onClick={() => setDeleteCandidateID(user.id)}
                                  type='button'
                                >
                                  {t('common.delete')}
                                </button>
                              </div>
                            )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )
        : null}

      <Sheet open={selectedUser !== null} onOpenChange={(open) => {
        if (!open) closeSelectedUser();
      }}>
        <SheetContent className='subscription-detail-sheet' side='right'>
          <SheetHeader>
            <SheetTitle>{selectedUser?.name ?? t('subscriptions.user.permissions')}</SheetTitle>
            <SheetDescription>
              {selectedUser === null
                ? t('subscriptions.user.permissionsDescription')
                : t('subscriptions.user.permissionsSummary', {
                    count: numberFormatter.format(grants.size),
                    id: selectedUser.id,
                  })}
            </SheetDescription>
          </SheetHeader>
          {selectedUser === null || catalog === null
            ? null
            : (
                <div className='subscription-detail-sheet__body subscription-grants'>
                  {[...sourceGroups].map(([sourceID, keys]) => (
                    <fieldset className='subscription-grant-group' key={sourceID}>
                      <legend>{sourceID}</legend>
                      <button className='text-button' onClick={() => toggleKeys(keys)} type='button'>
                        {t('subscriptions.user.toggleSource')}
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
                    </fieldset>
                  ))}
                </div>
              )}
          <SheetFooter>
            <button className='button button--primary' disabled={busy || selectedUser === null || catalog === null} onClick={() => void saveGrants()} type='button'>
              {busy ? t('subscriptions.common.saving') : t('subscriptions.user.savePermissions')}
            </button>
            <button className='button button--secondary' disabled={busy} onClick={closeSelectedUser} type='button'>
              {t('subscriptions.detail.close')}
            </button>
          </SheetFooter>
        </SheetContent>
      </Sheet>
    </section>
  );
}
