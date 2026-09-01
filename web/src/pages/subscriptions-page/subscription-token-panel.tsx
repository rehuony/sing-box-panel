import { useTranslation } from 'react-i18next';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

import type { SubscriptionChannelSummary, SubscriptionCursor, SubscriptionToken, SubscriptionUser } from '@/api/api-client';

import { useApiClient } from '@/api/api-client-context';
import { ActionError } from '@/components/action-error';
import { describeRequestError, ErrorNotice } from '@/components/error-notice';
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';

import { buildPublicSubscriptionURL } from './public-subscription-url';

interface IssuedSecret {
  token: string;
  kind: 'created' | 'rotated';
}

interface SubscriptionTokenPanelProps {
  users: SubscriptionUser[] | null;
}

function localDateTimeToISO(value: string, invalidMessage: string): string | undefined {
  if (value === '') return undefined;
  const date = new Date(value);
  if (Number.isNaN(date.valueOf())) throw new Error(invalidMessage);
  return date.toISOString();
}

export function SubscriptionTokenPanel({ users }: SubscriptionTokenPanelProps) {
  const { i18n, t } = useTranslation();
  const client = useApiClient();
  const [tokens, setTokens] = useState<SubscriptionToken[] | null>(null);
  const [next, setNext] = useState<SubscriptionCursor>();
  const [loadError, setLoadError] = useState<unknown>(null);
  const [expiresAt, setExpiresAt] = useState('');
  const [userID, setUserID] = useState('');
  const [label, setLabel] = useState('');
  const [secret, setSecret] = useState<IssuedSecret | null>(null);
  const [copied, setCopied] = useState(false);
  const [publicChannels, setPublicChannels] = useState<SubscriptionChannelSummary[] | null>(null);
  const [publicChannelID, setPublicChannelID] = useState('');
  const [publicURLCopied, setPublicURLCopied] = useState(false);
  const [actionError, setActionError] = useState('');
  const [actionCandidate, setActionCandidate] = useState<{
    tokenID: string;
    action: 'delete' | 'revoke' | 'rotate';
  } | null>(null);
  const [busy, setBusy] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [detail, setDetail] = useState<SubscriptionToken | null>(null);
  const [detailLoadingID, setDetailLoadingID] = useState<string | null>(null);
  const copyGenerationRef = useRef(0);
  const loadGenerationRef = useRef(0);
  const loadingMoreRef = useRef(false);
  const publicChannelIDRef = useRef('');
  const safeActionRef = useRef<HTMLButtonElement>(null);
  const dateFormatter = useMemo(
    () => new Intl.DateTimeFormat(i18n.resolvedLanguage ?? i18n.language ?? 'en', { dateStyle: 'medium' }),
    [i18n.language, i18n.resolvedLanguage],
  );
  const dateTimeFormatter = useMemo(
    () => new Intl.DateTimeFormat(i18n.resolvedLanguage ?? i18n.language ?? 'en', {
      dateStyle: 'medium',
      timeStyle: 'short',
    }),
    [i18n.language, i18n.resolvedLanguage],
  );
  const numberFormatter = useMemo(
    () => new Intl.NumberFormat(i18n.resolvedLanguage ?? i18n.language ?? 'en'),
    [i18n.language, i18n.resolvedLanguage],
  );
  const selectedUserID = users?.some((user) => user.id === userID)
    ? userID
    : users?.[0]?.id ?? '';

  const load = useCallback(async (signal?: AbortSignal, cursor?: SubscriptionCursor, append = false) => {
    const generation = ++loadGenerationRef.current;
    if (!append) {
      setTokens(null);
      setNext(undefined);
    }
    try {
      setLoadError(null);
      const page = await client.listSubscriptionTokens({
        limit: 50,
        beforeTime: cursor?.created_at,
        beforeID: cursor?.id,
      }, signal);
      if (!signal?.aborted && generation === loadGenerationRef.current) {
        setTokens((current) => append ? [...(current ?? []), ...page.items] : page.items);
        setNext(page.next);
      }
    } catch (error) {
      if (!signal?.aborted && generation === loadGenerationRef.current) {
        if (!append) setTokens(null);
        setLoadError(error);
      }
    }
  }, [client]);

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, [load]);

  useEffect(() => {
    if (actionCandidate !== null) safeActionRef.current?.focus();
  }, [actionCandidate]);

  function invalidatePendingCopies() {
    copyGenerationRef.current += 1;
  }

  function replacePublicChannelID(nextChannelID: string) {
    publicChannelIDRef.current = nextChannelID;
    invalidatePendingCopies();
    setPublicChannelID(nextChannelID);
  }

  function replaceSecret(nextSecret: IssuedSecret | null) {
    invalidatePendingCopies();
    setSecret(nextSecret);
  }

  async function loadPublicChannels() {
    try {
      const loaded: SubscriptionChannelSummary[] = [];
      let cursor: SubscriptionCursor | undefined;
      do {
        const page = await client.listSubscriptionChannels({
          beforeID: cursor?.id,
          beforeTime: cursor?.created_at,
          limit: 100,
        });
        loaded.push(...page.items);
        cursor = page.next;
      } while (cursor !== undefined);

      const enabled = loaded.filter((channel) => channel.enabled);
      setPublicChannels(enabled);
      replacePublicChannelID(enabled.some((channel) => channel.id === publicChannelIDRef.current)
        ? publicChannelIDRef.current
        : enabled[0]?.id ?? '');
    } catch (error) {
      setPublicChannels([]);
      replacePublicChannelID('');
      setActionError(t('subscriptions.token.channelLoadFailed', {
        error: describeRequestError(error),
      }));
    }
  }

  async function create() {
    try {
      if (selectedUserID === '') throw new Error(t('subscriptions.token.validation.user'));
      if (label.trim() === '') throw new Error(t('subscriptions.token.validation.label'));
      invalidatePendingCopies();
      setBusy(true);
      setActionError('');
      const issued = await client.createSubscriptionToken({
        userID: selectedUserID,
        label: label.trim(),
        expiresAt: localDateTimeToISO(expiresAt, t('subscriptions.token.validation.expiry')),
      });
      replaceSecret({ kind: 'created', token: issued.token });
      setCopied(false);
      setPublicChannels(null);
      replacePublicChannelID('');
      setPublicURLCopied(false);
      await Promise.all([load(), loadPublicChannels()]);
    } catch (error) {
      setActionError(describeRequestError(error));
    } finally {
      setBusy(false);
    }
  }

  async function toggle(token: SubscriptionToken) {
    try {
      setBusy(true);
      setActionError('');
      await client.setSubscriptionTokenEnabled(token.id, !token.enabled);
      await load();
    } catch (error) {
      setActionError(describeRequestError(error));
    } finally {
      setBusy(false);
    }
  }

  async function inspect(token: SubscriptionToken) {
    try {
      setDetailLoadingID(token.id);
      setActionError('');
      setDetail(await client.getSubscriptionToken(token.id));
    } catch (error) {
      setActionError(describeRequestError(error));
    } finally {
      setDetailLoadingID(null);
    }
  }

  async function remove(token: SubscriptionToken) {
    try {
      setBusy(true);
      setActionError('');
      await client.deleteSubscriptionToken(token.id);
      setActionCandidate(null);
      await load();
    } catch (error) {
      setActionError(describeRequestError(error));
    } finally {
      setBusy(false);
    }
  }

  async function rotate(token: SubscriptionToken) {
    try {
      invalidatePendingCopies();
      setBusy(true);
      setActionError('');
      const rotated = await client.rotateSubscriptionToken(token.id);
      replaceSecret({ kind: 'rotated', token: rotated.token });
      setCopied(false);
      setPublicChannels(null);
      replacePublicChannelID('');
      setPublicURLCopied(false);
      setActionCandidate(null);
      await Promise.all([load(), loadPublicChannels()]);
    } catch (error) {
      setActionError(describeRequestError(error));
    } finally {
      setBusy(false);
    }
  }

  async function revoke(token: SubscriptionToken) {
    try {
      setBusy(true);
      setActionError('');
      await client.revokeSubscriptionToken(token.id);
      setActionCandidate(null);
      await load();
    } catch (error) {
      setActionError(describeRequestError(error));
    } finally {
      setBusy(false);
    }
  }

  async function copySecret() {
    if (secret === null) return;
    const generation = ++copyGenerationRef.current;
    const token = secret.token;
    setActionError('');
    setCopied(false);
    try {
      const clipboard = navigator.clipboard;
      if (typeof clipboard?.writeText !== 'function') {
        setActionError(t('subscriptions.token.secret.clipboardUnavailable'));
        return;
      }
      await clipboard.writeText(token);
      if (generation !== copyGenerationRef.current) return;
      setCopied(true);
    } catch (error) {
      if (generation !== copyGenerationRef.current) return;
      setActionError(t('subscriptions.token.secret.copyFailed', {
        error: describeRequestError(error),
      }));
    }
  }

  async function copyPublicURL(publicURL: string) {
    const generation = ++copyGenerationRef.current;
    setActionError('');
    setPublicURLCopied(false);
    try {
      const clipboard = navigator.clipboard;
      if (typeof clipboard?.writeText !== 'function') {
        setActionError(t('subscriptions.token.secret.clipboardUnavailable'));
        return;
      }
      await clipboard.writeText(publicURL);
      if (generation !== copyGenerationRef.current) return;
      setPublicURLCopied(true);
    } catch (error) {
      if (generation !== copyGenerationRef.current) return;
      setActionError(t('subscriptions.token.secret.copyFailed', {
        error: describeRequestError(error),
      }));
    }
  }

  async function loadMore() {
    if (next === undefined || loadingMoreRef.current) return;
    loadingMoreRef.current = true;
    setLoadingMore(true);
    try {
      await load(undefined, next, true);
    } finally {
      loadingMoreRef.current = false;
      setLoadingMore(false);
    }
  }

  const publicURL = secret !== null && publicChannelID !== ''
    ? buildPublicSubscriptionURL(secret.token, publicChannelID)
    : '';

  function tokenState(token: SubscriptionToken): string {
    if (token.active) return t('subscriptions.token.state.active');
    if (token.revoked_at) return t('subscriptions.token.state.revoked');
    if (token.enabled) return t('subscriptions.token.state.expired');
    return t('subscriptions.token.state.disabled');
  }

  function candidateActionName(action: 'delete' | 'revoke' | 'rotate'): string {
    return t(`subscriptions.token.action.${action}.name`);
  }

  return (
    <section className='subscription-panel' id='subscription-tokens' aria-labelledby='subscription-tokens-title'>
      <div className='subscription-panel__heading'>
        <div>
          <h2 id='subscription-tokens-title'>{t('subscriptions.token.title')}</h2>
          <p>{t('subscriptions.token.description')}</p>
        </div>
        <span className='count-label'>
          {t('subscriptions.token.activeCount', {
            count: numberFormatter.format(tokens?.filter((token) => token.active).length ?? 0),
          })}
        </span>
      </div>
      <ActionError message={actionError} title={t('subscriptions.token.actionFailed')} />
      {loadError === null ? null : <ErrorNotice error={loadError} title={t('subscriptions.token.loadFailed')} />}

      {secret === null
        ? null
        : (
            <div className='one-time-secret' role='status'>
              <div>
                <h3>
                  {secret.kind === 'created'
                    ? t('subscriptions.token.secret.saveCreated')
                    : t('subscriptions.token.secret.saveRotated')}
                </h3>
                <p>{t('subscriptions.token.secret.oneTime')}</p>
              </div>
              <code>{secret.token}</code>
              <div className='one-time-secret__delivery'>
                <div className='field-group'>
                  <label htmlFor='public-subscription-channel'>{t('subscriptions.token.secret.deliveryChannel')}</label>
                  <select
                    id='public-subscription-channel'
                    disabled={publicChannels === null || publicChannels.length === 0}
                    onChange={(event) => {
                      replacePublicChannelID(event.target.value);
                      setPublicURLCopied(false);
                    }}
                    value={publicChannelID}
                  >
                    {publicChannels?.map((channel) => (
                      <option key={channel.id} value={channel.id}>
                        {channel.name}
                        {' · '}
                        {channel.format}
                      </option>
                    ))}
                  </select>
                </div>
                {publicChannels === null
                  ? <p>{t('subscriptions.token.secret.loadingChannels')}</p>
                  : publicChannels.length === 0
                    ? <p>{t('subscriptions.token.secret.noChannels')}</p>
                    : (
                        <div className='one-time-secret__url'>
                          <span>{t('subscriptions.token.secret.clientURL')}</span>
                          <code aria-label={t('subscriptions.token.secret.clientURL')}>{publicURL}</code>
                        </div>
                      )}
              </div>
              <div className='inline-actions'>
                <button className='button button--primary' onClick={() => void copySecret()} type='button'>
                  {copied ? t('subscriptions.token.secret.copied') : t('subscriptions.token.secret.copy')}
                </button>
                <button
                  className='button button--secondary'
                  disabled={publicURL === ''}
                  onClick={() => void copyPublicURL(publicURL)}
                  type='button'
                >
                  {publicURLCopied ? t('subscriptions.token.secret.urlCopied') : t('subscriptions.token.secret.copyURL')}
                </button>
                <button className='button button--secondary' onClick={() => {
                  replaceSecret(null);
                  setCopied(false);
                  setPublicChannels(null);
                  replacePublicChannelID('');
                  setPublicURLCopied(false);
                }} type='button'>
                  {t('subscriptions.token.secret.saved')}
                </button>
              </div>
            </div>
          )}

      <form className='token-issuer' onSubmit={(event) => {
        event.preventDefault();
        void create();
      }}>
        <div className='field-group'>
          <label htmlFor='token-user'>{t('subscriptions.token.field.user')}</label>
          <select
            disabled={busy || users === null || users.length === 0}
            id='token-user'
            onChange={(event) => setUserID(event.target.value)}
            value={selectedUserID}
          >
            <option value=''>{t('subscriptions.common.selectUser')}</option>
            {(users ?? []).map((user) => <option key={user.id} value={user.id}>{user.name}</option>)}
          </select>
        </div>
        <div className='field-group'>
          <label htmlFor='token-label'>{t('subscriptions.token.field.label')}</label>
          <input id='token-label' onChange={(event) => setLabel(event.target.value)} placeholder={t('subscriptions.token.field.labelPlaceholder')} value={label} />
        </div>
        <div className='field-group'>
          <label htmlFor='token-expiry'>{t('subscriptions.token.field.expiresAt')}</label>
          <input id='token-expiry' onChange={(event) => setExpiresAt(event.target.value)} type='datetime-local' value={expiresAt} />
          <span>{t('subscriptions.token.field.expiryHelp')}</span>
        </div>
        <button className='button button--primary' disabled={busy || actionCandidate !== null || users === null || users.length === 0} type='submit'>
          {busy ? t('subscriptions.token.issuing') : t('subscriptions.token.issue')}
        </button>
      </form>

      {tokens === null ? <div className='inline-loading' aria-busy='true'>{t('subscriptions.token.loading')}</div> : null}
      {tokens?.length === 0
        ? (
            <div className='empty-state'>
              <strong>{t('subscriptions.token.empty.title')}</strong>
              <p>{t('subscriptions.token.empty.description')}</p>
            </div>
          )
        : null}
      {tokens && tokens.length > 0
        ? (
            <div className='entity-table-wrap'>
              <table className='data-table'>
                <thead>
                  <tr>
                    <th>{t('subscriptions.token.column.id')}</th>
                    <th>{t('subscriptions.common.state')}</th>
                    <th>{t('subscriptions.token.column.usage')}</th>
                    <th>{t('subscriptions.common.actions')}</th>
                  </tr>
                </thead>
                <tbody>
                  {tokens.map((token) => (
                    <tr key={token.id}>
                      <td data-label={t('subscriptions.token.column.token')}>
                        <code>{token.id}</code>
                        <strong className='table-subline'>{token.label}</strong>
                        <small className='table-subline'>
                          {t('subscriptions.token.issuedSummary', {
                            date: dateFormatter.format(new Date(token.created_at)),
                            user: token.user_id,
                          })}
                        </small>
                      </td>
                      <td data-label={t('subscriptions.common.state')}>
                        <span className={`state-label ${token.active ? 'state-label--success' : 'state-label--warning'}`}>
                          <span aria-hidden='true' />
                          {tokenState(token)}
                        </span>
                      </td>
                      <td data-label={t('subscriptions.token.column.usage')}>
                        <strong>
                          {t('subscriptions.token.usage.requests', {
                            count: numberFormatter.format(token.successful_request_count),
                          })}
                        </strong>
                        <small className='table-subline'>
                          {t('subscriptions.token.usage.bodiesBytes', {
                            bodies: numberFormatter.format(token.body_response_count),
                            bytes: numberFormatter.format(token.bytes_served),
                          })}
                        </small>
                      </td>
                      <td data-label={t('subscriptions.common.actions')}>
                        {actionCandidate?.tokenID === token.id
                          ? (
                              <div
                                aria-label={t('subscriptions.token.confirmation.aria', {
                                  action: candidateActionName(actionCandidate.action),
                                  label: token.label,
                                })}
                                className='inline-confirmation'
                                role='group'
                              >
                                <span className='inline-confirmation__prompt'>
                                  {actionCandidate.action === 'rotate'
                                    ? t('subscriptions.token.action.rotate.prompt')
                                    : actionCandidate.action === 'revoke'
                                      ? t('subscriptions.token.action.revoke.prompt')
                                      : t('subscriptions.token.action.delete.prompt')}
                                </span>
                                <button
                                  aria-label={t('subscriptions.token.confirmation.confirmAria', {
                                    action: candidateActionName(actionCandidate.action),
                                    label: token.label,
                                  })}
                                  className={`button ${actionCandidate.action === 'rotate' ? 'button--primary' : 'button--danger'} button--small`}
                                  disabled={busy}
                                  onClick={() => {
                                    if (actionCandidate.action === 'rotate') void rotate(token);
                                    else if (actionCandidate.action === 'revoke') void revoke(token);
                                    else void remove(token);
                                  }}
                                  type='button'
                                >
                                  {busy
                                    ? actionCandidate.action === 'rotate'
                                      ? t('subscriptions.token.action.rotate.pending')
                                      : actionCandidate.action === 'revoke'
                                        ? t('subscriptions.token.action.revoke.pending')
                                        : t('subscriptions.common.deleting')
                                    : t('subscriptions.token.confirmation.confirm', {
                                        action: candidateActionName(actionCandidate.action),
                                      })}
                                </button>
                                <button
                                  aria-label={t('subscriptions.token.confirmation.cancelAria', {
                                    action: candidateActionName(actionCandidate.action),
                                    label: token.label,
                                  })}
                                  autoFocus
                                  className='text-button'
                                  disabled={busy}
                                  onClick={() => setActionCandidate(null)}
                                  ref={safeActionRef}
                                  type='button'
                                >
                                  {actionCandidate.action === 'delete'
                                    ? t('subscriptions.token.confirmation.keep')
                                    : t('subscriptions.token.confirmation.keepCurrent')}
                                </button>
                              </div>
                            )
                          : (
                              <div className='table-actions'>
                                <button
                                  aria-label={t('subscriptions.token.action.inspectAria', { label: token.label })}
                                  aria-pressed={detail?.id === token.id}
                                  className='text-button'
                                  disabled={busy || actionCandidate !== null || detailLoadingID !== null}
                                  onClick={() => void inspect(token)}
                                  type='button'
                                >
                                  {detailLoadingID === token.id ? t('subscriptions.common.loading') : t('subscriptions.common.inspect')}
                                </button>
                                <button
                                  aria-label={t('subscriptions.token.action.rotate.aria', { label: token.label })}
                                  className='text-button'
                                  disabled={busy || actionCandidate !== null || !token.active}
                                  onClick={() => setActionCandidate({ action: 'rotate', tokenID: token.id })}
                                  type='button'
                                >
                                  {t('subscriptions.token.action.rotate.label')}
                                </button>
                                <button className='text-button' disabled={busy || actionCandidate !== null || token.revoked_at !== undefined} onClick={() => void toggle(token)} type='button'>
                                  {token.enabled ? t('subscriptions.common.disable') : t('subscriptions.common.enable')}
                                </button>
                                <button
                                  aria-label={t('subscriptions.token.action.revoke.aria', { label: token.label })}
                                  className='text-button text-button--danger'
                                  disabled={busy || actionCandidate !== null || !token.active}
                                  onClick={() => setActionCandidate({ action: 'revoke', tokenID: token.id })}
                                  type='button'
                                >
                                  {t('subscriptions.token.action.revoke.label')}
                                </button>
                                <button
                                  aria-label={t('subscriptions.token.action.delete.aria', { label: token.label })}
                                  className='text-button text-button--danger'
                                  disabled={busy || actionCandidate !== null}
                                  onClick={() => setActionCandidate({ action: 'delete', tokenID: token.id })}
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
      {next
        ? (
            <button
              aria-busy={loadingMore}
              className='button button--secondary'
              disabled={busy || loadingMore || actionCandidate !== null}
              onClick={() => void loadMore()}
              type='button'
            >
              {loadingMore ? t('subscriptions.token.loadingMore') : t('subscriptions.token.loadMore')}
            </button>
          )
        : null}
      <Sheet open={detail !== null} onOpenChange={(open) => {
        if (!open) setDetail(null);
      }}>
        <SheetContent className='subscription-detail-sheet' side='right'>
          <SheetHeader>
            <SheetTitle>{detail?.label ?? t('subscriptions.token.detail')}</SheetTitle>
            <SheetDescription>{detail?.id ?? t('subscriptions.token.detailDescription')}</SheetDescription>
          </SheetHeader>
          {detail === null
            ? null
            : (
                <div className='subscription-detail-sheet__body'>
                  <dl className='subscription-detail__grid'>
                    <div>
                      <dt>{t('subscriptions.token.column.id')}</dt>
                      <dd><code>{detail.id}</code></dd>
                    </div>
                    <div>
                      <dt>{t('subscriptions.token.field.user')}</dt>
                      <dd><code>{detail.user_id}</code></dd>
                    </div>
                    <div>
                      <dt>{t('subscriptions.common.state')}</dt>
                      <dd>{tokenState(detail)}</dd>
                    </div>
                    <div>
                      <dt>{t('subscriptions.token.detailField.created')}</dt>
                      <dd>{dateTimeFormatter.format(new Date(detail.created_at))}</dd>
                    </div>
                    <div>
                      <dt>{t('subscriptions.token.detailField.expires')}</dt>
                      <dd>{detail.expires_at ? dateTimeFormatter.format(new Date(detail.expires_at)) : t('subscriptions.common.never')}</dd>
                    </div>
                    <div>
                      <dt>{t('subscriptions.token.detailField.lastUsed')}</dt>
                      <dd>{detail.last_used_at ? dateTimeFormatter.format(new Date(detail.last_used_at)) : t('subscriptions.common.never')}</dd>
                    </div>
                    <div>
                      <dt>{t('subscriptions.token.detailField.successfulRequests')}</dt>
                      <dd>{numberFormatter.format(detail.successful_request_count)}</dd>
                    </div>
                    <div>
                      <dt>{t('subscriptions.token.detailField.bytesServed')}</dt>
                      <dd>{numberFormatter.format(detail.bytes_served)}</dd>
                    </div>
                  </dl>
                </div>
              )}
        </SheetContent>
      </Sheet>
    </section>
  );
}
