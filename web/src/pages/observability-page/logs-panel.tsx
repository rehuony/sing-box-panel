import { useTranslation } from 'react-i18next';
import { useSearchParams } from 'react-router-dom';
import { useCallback, useEffect, useRef, useState } from 'react';
import {
  ChevronRight,
  ListFilter,
  RefreshCw,
  Trash2,
} from 'lucide-react';

import type { LogEntry, LogLevel, LogSource } from '@/api/api-client';

import { useApiClient } from '@/api/api-client-context';
import { ActionError } from '@/components/action-error';
import { AnimatedIcon } from '@/components/animated-icon';
import { describeRequestError, ErrorNotice } from '@/components/error-notice';
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';

export interface LogsPanelProps {
  refreshKey: number;
}

type StreamState = 'off' | 'connecting' | 'live' | 'ended' | 'error';

interface StreamFilters {
  code?: string;
  since?: string;
  level?: LogLevel;
  source?: LogSource;
  appliedRevision: number;
  appliedFingerprint: string;
}

interface LogFilterDraft {
  code: string;
  since: string;
  until: string;
  appliedRevision: number;
}

type DestructiveAction
  = { kind: 'clear' }
    | { entry: LogEntry; kind: 'delete' }
    | null;

function formatLogTime(value: string, locale: string): string {
  return new Intl.DateTimeFormat(locale, {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  }).format(new Date(value));
}

function formatLogDate(value: string, locale: string): string {
  return new Intl.DateTimeFormat(locale, {
    dateStyle: 'medium',
    timeStyle: 'medium',
  }).format(new Date(value));
}

function normalizedLogBoundary(value: string): string | undefined {
  const timestamp = new Date(value);
  return Number.isNaN(timestamp.valueOf()) ? undefined : timestamp.toISOString();
}

function streamStatusLabel(
  state: StreamState,
  translate: (key: string, options: { defaultValue: string }) => string,
): string {
  switch (state) {
    case 'off':
      return translate('observability.logs.stream.off', { defaultValue: 'Live stream off' });
    case 'connecting':
      return translate('observability.logs.stream.connecting', { defaultValue: 'Connecting to live events…' });
    case 'live':
      return translate('observability.logs.stream.live', { defaultValue: 'Receiving live events' });
    case 'ended':
      return translate('observability.logs.stream.ended', { defaultValue: 'Live stream ended' });
    case 'error':
      return translate('observability.logs.stream.error', { defaultValue: 'Live stream interrupted' });
  }
}

function initialLogSource(value: string | null): LogSource | '' {
  return value === 'core' || value === 'panel' || value === 'security' || value === 'task'
    ? value
    : '';
}

function initialLogLevel(value: string | null): LogLevel | '' {
  return value === 'debug' || value === 'error' || value === 'fatal'
    || value === 'info' || value === 'trace' || value === 'warn'
    ? value
    : '';
}

export function LogsPanel({ refreshKey }: LogsPanelProps) {
  const { i18n, t } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language ?? 'en';
  const client = useApiClient();
  const [searchParams, setSearchParams] = useSearchParams();
  const source = initialLogSource(searchParams.get('source'));
  const level = initialLogLevel(searchParams.get('level'));
  const appliedCode = searchParams.get('code') ?? '';
  const appliedSince = searchParams.get('from') ?? '';
  const appliedUntil = searchParams.get('to') ?? '';
  const appliedFilterFingerprint = [source, level, appliedCode, appliedSince, appliedUntil].join('\u0000');
  const appliedFilterStateRef = useRef({ fingerprint: appliedFilterFingerprint, revision: 0 });
  if (appliedFilterStateRef.current.fingerprint !== appliedFilterFingerprint) {
    appliedFilterStateRef.current = {
      fingerprint: appliedFilterFingerprint,
      revision: appliedFilterStateRef.current.revision + 1,
    };
  }
  const appliedFilterRevision = appliedFilterStateRef.current.revision;
  const [logs, setLogs] = useState<LogEntry[] | null>(null);
  const [nextCursor, setNextCursor] = useState<{ id: string; time: string } | undefined>();
  const [selected, setSelected] = useState<LogEntry | null>(null);
  const [loadError, setLoadError] = useState<unknown>(null);
  const [detailError, setDetailError] = useState('');
  const [actionError, setActionError] = useState('');
  const [actionMessage, setActionMessage] = useState('');
  const [destructiveAction, setDestructiveAction] = useState<DestructiveAction>(null);
  const [mutating, setMutating] = useState(false);
  const [live, setLive] = useState(false);
  const [streamState, setStreamState] = useState<StreamState>('off');
  const [streamError, setStreamError] = useState('');
  const [streamFilters, setStreamFilters] = useState<StreamFilters | null>(null);
  const [refreshing, setRefreshing] = useState(false);
  const [loadingOlder, setLoadingOlder] = useState(false);
  const [detailLoading, setDetailLoading] = useState(false);
  const loadGenerationRef = useRef(0);
  const loadingOlderRef = useRef(false);
  const detailGenerationRef = useRef(0);
  const streamControllerRef = useRef<AbortController | null>(null);
  const [filterDraft, setFilterDraft] = useState<LogFilterDraft>(() => ({
    appliedRevision: appliedFilterRevision,
    code: appliedCode,
    since: appliedSince,
    until: appliedUntil,
  }));
  const draftMatchesURL = filterDraft.appliedRevision === appliedFilterRevision;
  const code = draftMatchesURL ? filterDraft.code : appliedCode;
  const since = draftMatchesURL ? filterDraft.since : appliedSince;
  const until = draftMatchesURL ? filterDraft.until : appliedUntil;
  const streamMatchesURL = streamFilters?.appliedFingerprint === appliedFilterFingerprint
    && streamFilters.appliedRevision === appliedFilterRevision;
  const streamActive = live && streamMatchesURL;
  const streamConnecting = streamState === 'connecting' && streamMatchesURL;
  const streamControlsLocked = streamActive || streamConnecting;
  const displayedStreamState = streamMatchesURL ? streamState : 'off';

  const replaceLogSearchParams = useCallback((values: Partial<Record<'code' | 'from' | 'level' | 'source' | 'to', string>>) => {
    setSearchParams((current) => {
      const next = new URLSearchParams(current);
      for (const key of ['code', 'from', 'level', 'source', 'to'] as const) {
        const value = values[key];
        if (value === undefined) continue;
        if (value === '') next.delete(key);
        else next.set(key, value);
      }
      return next;
    }, { replace: true });
  }, [setSearchParams]);

  function updateFilterDraft(values: Partial<Pick<LogFilterDraft, 'code' | 'since' | 'until'>>) {
    setFilterDraft({
      appliedRevision: appliedFilterRevision,
      code,
      since,
      until,
      ...values,
    });
  }

  useEffect(() => {
    const nextCode = code.trim();
    const nextSince = since.trim();
    const nextUntil = until.trim();
    if (
      nextCode === appliedCode
      && nextSince === appliedSince
      && nextUntil === appliedUntil
    ) {
      return undefined;
    }
    const scheduledRevision = appliedFilterRevision;
    const timeout = window.setTimeout(() => {
      if (scheduledRevision !== appliedFilterStateRef.current.revision) return;
      replaceLogSearchParams({ code: nextCode, from: nextSince, to: nextUntil });
    }, 300);
    return () => window.clearTimeout(timeout);
  }, [
    appliedCode,
    appliedFilterRevision,
    appliedSince,
    appliedUntil,
    code,
    replaceLogSearchParams,
    since,
    until,
  ]);

  const load = useCallback(async (
    signal?: AbortSignal,
    cursor?: { id: string; time: string },
    append = false,
  ) => {
    if (append && loadingOlderRef.current) return;
    const generation = ++loadGenerationRef.current;
    if (append) {
      loadingOlderRef.current = true;
      setLoadingOlder(true);
    } else {
      loadingOlderRef.current = false;
      setLoadingOlder(false);
      detailGenerationRef.current += 1;
      setSelected(null);
      setDetailError('');
      setDetailLoading(false);
      setDestructiveAction(null);
      setActionError('');
      setActionMessage('');
      setLogs(null);
      setNextCursor(undefined);
      setRefreshing(true);
    }
    try {
      setLoadError(null);
      const page = await client.listLogs({
        afterID: cursor?.id,
        afterTime: cursor?.time,
        code: appliedCode || undefined,
        source: source || undefined,
        level: level || undefined,
        since: appliedSince || undefined,
        until: appliedUntil || undefined,
        limit: 50,
      }, signal);
      if (!signal?.aborted && generation === loadGenerationRef.current) {
        setLogs((current) => append && current !== null ? [...current, ...page.items] : page.items);
        setNextCursor(page.next);
      }
    } catch (error) {
      if (!signal?.aborted && generation === loadGenerationRef.current) setLoadError(error);
    } finally {
      if (!signal?.aborted && generation === loadGenerationRef.current) {
        loadingOlderRef.current = false;
        setRefreshing(false);
        setLoadingOlder(false);
      }
    }
  }, [appliedCode, appliedSince, appliedUntil, client, level, source]);

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, [load, refreshKey]);

  useEffect(() => {
    if (!streamActive || streamFilters === null) return undefined;

    const activeFilters = streamFilters;
    const controller = new AbortController();
    let active = true;
    streamControllerRef.current = controller;

    async function consumeStream() {
      try {
        for await (const event of client.streamLogs({
          code: activeFilters.code,
          source: activeFilters.source,
          level: activeFilters.level,
          since: activeFilters.since,
          limit: 50,
        }, controller.signal)) {
          if (!active || controller.signal.aborted) return;
          setStreamState('live');
          setLogs((current) => {
            const withoutDuplicate = (current ?? []).filter((entry) => entry.id !== event.entry.id);
            return [event.entry, ...withoutDuplicate].slice(0, 50);
          });
          setSelected((current) => current?.id === event.entry.id ? event.entry : current);
        }
        if (active && !controller.signal.aborted) {
          setStreamState('ended');
          setLive(false);
        }
      } catch (error) {
        if (!active || controller.signal.aborted) return;
        setStreamError(describeRequestError(error));
        setStreamState('error');
        setLive(false);
      }
    }

    void consumeStream();
    return () => {
      active = false;
      controller.abort();
      if (streamControllerRef.current === controller) streamControllerRef.current = null;
    };
  }, [appliedFilterFingerprint, client, streamActive, streamFilters]);

  function toggleLiveStream() {
    if (streamControlsLocked) {
      streamControllerRef.current?.abort();
      setLive(false);
      setStreamFilters(null);
      setStreamState('off');
      return;
    }
    const nextCode = code.trim();
    const nextSince = since.trim();
    const nextUntil = until.trim();
    const nextFingerprint = [source, level, nextCode, nextSince, nextUntil].join('\u0000');
    setStreamError('');
    setStreamState('connecting');
    replaceLogSearchParams({
      code: nextCode,
      from: nextSince,
      to: nextUntil,
    });
    setStreamFilters({
      appliedFingerprint: nextFingerprint,
      appliedRevision: appliedFilterRevision + (nextFingerprint === appliedFilterFingerprint ? 0 : 1),
      code: nextCode || undefined,
      level: level || undefined,
      since: nextSince || undefined,
      source: source || undefined,
    });
    setLive(true);
  }

  function refreshLogs() {
    const nextCode = code.trim();
    const nextSince = since.trim();
    const nextUntil = until.trim();
    if (
      nextCode !== appliedCode
      || nextSince !== appliedSince
      || nextUntil !== appliedUntil
    ) {
      replaceLogSearchParams({ code: nextCode, from: nextSince, to: nextUntil });
      return;
    }
    void load();
  }

  async function inspect(entry: LogEntry) {
    const generation = ++detailGenerationRef.current;
    setSelected(entry);
    setDetailError('');
    setDetailLoading(true);
    setDestructiveAction(null);
    try {
      const detail = await client.getLog(entry.id);
      if (generation === detailGenerationRef.current) setSelected(detail);
    } catch (error) {
      if (generation === detailGenerationRef.current) {
        setDetailError(describeRequestError(error));
      }
    } finally {
      if (generation === detailGenerationRef.current) setDetailLoading(false);
    }
  }

  function closeDetail() {
    detailGenerationRef.current += 1;
    setSelected(null);
    setDetailError('');
    setDetailLoading(false);
    if (destructiveAction?.kind === 'delete') setDestructiveAction(null);
  }

  async function deleteSelectedEvent() {
    if (destructiveAction?.kind !== 'delete') return;
    const { entry } = destructiveAction;
    setMutating(true);
    setActionError('');
    setActionMessage('');
    try {
      await client.deleteLog(entry.id);
      setLogs((current) => current?.filter((item) => item.id !== entry.id) ?? current);
      setSelected((current) => current?.id === entry.id ? null : current);
      setDestructiveAction(null);
      setActionMessage(t('observability.logs.deleted', {
        defaultValue: 'Deleted event {{code}}.',
        code: entry.code,
      }));
    } catch (error) {
      setActionError(describeRequestError(error));
    } finally {
      setMutating(false);
    }
  }

  const clearBefore = until.trim();
  const clearBeforeTimestamp = clearBefore === '' ? undefined : normalizedLogBoundary(clearBefore);
  const hasUnsupportedClearFilter = level !== '' || code.trim() !== '' || since.trim() !== ''
    || (clearBefore !== '' && clearBeforeTimestamp === undefined);
  const clearSourceScope = source === ''
    ? t('observability.logs.clearScopeAll', { defaultValue: 'all sources' })
    : t(`observability.logs.sourceOption.${source}`, { defaultValue: source });
  const clearScope = clearBeforeTimestamp === undefined
    ? clearSourceScope
    : t('observability.logs.clearScopeBefore', {
        before: formatLogDate(clearBeforeTimestamp, locale),
        defaultValue: '{{scope}} before {{before}}',
        scope: clearSourceScope,
      });

  async function clearFilteredEvents() {
    if (destructiveAction?.kind !== 'clear' || hasUnsupportedClearFilter) return;
    streamControllerRef.current?.abort();
    setLive(false);
    setStreamState('off');
    setMutating(true);
    setActionError('');
    setActionMessage('');
    try {
      const result = await client.clearLogs({
        ...(clearBeforeTimestamp === undefined ? {} : { before: clearBeforeTimestamp }),
        ...(source === '' ? {} : { source }),
      });
      setLogs([]);
      setSelected(null);
      setDestructiveAction(null);
      setActionMessage(t('observability.logs.cleared', {
        defaultValue: 'Cleared {{count}} filtered events.',
        count: new Intl.NumberFormat(locale).format(result.deleted),
      }));
    } catch (error) {
      setActionError(describeRequestError(error));
    } finally {
      setMutating(false);
    }
  }

  return (
    <section className='observability-panel logs-panel' aria-labelledby='durable-logs-title'>
      <div className='observability-panel__heading'>
        <div>
          <h2 id='durable-logs-title'>{t('observability.logs.title', { defaultValue: 'Event log' })}</h2>
          <p>{t('observability.logs.description', { defaultValue: 'Filter persisted events, then inspect one record.' })}</p>
        </div>
        <span className='observability-panel__count' role='status'>
          {new Intl.NumberFormat(locale).format(logs?.length ?? 0)}
          {' '}
          {t('observability.logs.shown', { defaultValue: 'events shown' })}
        </span>
      </div>

      <div className='log-filters' role='group' aria-label={t('observability.logs.filters', { defaultValue: 'Log filters' })}>
        <span className='log-filters__icon'>
          <ListFilter aria-hidden='true' size={17} />
          {t('observability.logs.filtersShort', { defaultValue: 'Filters' })}
        </span>
        <label>
          {t('observability.logs.source', { defaultValue: 'Source' })}
          <select
            aria-label={t('observability.logs.source', { defaultValue: 'Log source' })}
            disabled={mutating || streamControlsLocked}
            onChange={(event) => replaceLogSearchParams({
              code: code.trim(),
              from: since.trim(),
              source: event.target.value,
              to: until.trim(),
            })}
            value={source}
          >
            <option value=''>{t('observability.logs.allSources', { defaultValue: 'All sources' })}</option>
            <option value='panel'>{t('observability.logs.sourceOption.panel', { defaultValue: 'Panel' })}</option>
            <option value='core'>{t('observability.logs.sourceOption.core', { defaultValue: 'Core' })}</option>
            <option value='task'>{t('observability.logs.sourceOption.task', { defaultValue: 'Task' })}</option>
            <option value='security'>{t('observability.logs.sourceOption.security', { defaultValue: 'Security' })}</option>
          </select>
        </label>
        <label>
          {t('observability.logs.level', { defaultValue: 'Level' })}
          <select
            aria-label={t('observability.logs.level', { defaultValue: 'Log level' })}
            disabled={mutating || streamControlsLocked}
            onChange={(event) => replaceLogSearchParams({
              code: code.trim(),
              from: since.trim(),
              level: event.target.value,
              to: until.trim(),
            })}
            value={level}
          >
            <option value=''>{t('observability.logs.allLevels', { defaultValue: 'All levels' })}</option>
            <option value='trace'>{t('observability.logs.levelOption.trace', { defaultValue: 'Trace' })}</option>
            <option value='debug'>{t('observability.logs.levelOption.debug', { defaultValue: 'Debug' })}</option>
            <option value='info'>{t('observability.logs.levelOption.info', { defaultValue: 'Info' })}</option>
            <option value='warn'>{t('observability.logs.levelOption.warn', { defaultValue: 'Warn' })}</option>
            <option value='error'>{t('observability.logs.levelOption.error', { defaultValue: 'Error' })}</option>
            <option value='fatal'>{t('observability.logs.levelOption.fatal', { defaultValue: 'Fatal' })}</option>
          </select>
        </label>
        <label>
          {t('observability.filter.code', { defaultValue: 'Code' })}
          <input
            aria-label={t('observability.filter.code', { defaultValue: 'Code' })}
            disabled={mutating || streamControlsLocked}
            onChange={(event) => updateFilterDraft({ code: event.target.value })}
            placeholder={t('observability.filter.anyCode', { defaultValue: 'Any code' })}
            type='search'
            value={code}
          />
        </label>
        <label>
          {t('observability.filter.from', { defaultValue: 'From' })}
          <input
            aria-label={t('observability.filter.logFrom', { defaultValue: 'Log start time' })}
            disabled={mutating || streamControlsLocked}
            onChange={(event) => updateFilterDraft({ since: event.target.value })}
            placeholder={t('observability.filter.logFromPlaceholder')}
            type='text'
            value={since}
          />
        </label>
        <label>
          {t('observability.filter.to', { defaultValue: 'To' })}
          <input
            aria-label={t('observability.filter.logTo', { defaultValue: 'Log end time' })}
            disabled={mutating || streamControlsLocked}
            onChange={(event) => updateFilterDraft({ until: event.target.value })}
            placeholder={t('observability.filter.logToPlaceholder')}
            type='text'
            value={until}
          />
        </label>
        <button
          className='button button--secondary button--small'
          disabled={refreshing || mutating || streamControlsLocked}
          onClick={refreshLogs}
          type='button'
        >
          <RefreshCw aria-hidden='true' size={15} />
          {refreshing
            ? t('observability.action.refreshing', { defaultValue: 'Refreshing…' })
            : t('observability.logs.refresh', { defaultValue: 'Refresh logs' })}
        </button>
        <button
          aria-pressed={streamControlsLocked}
          className='button button--secondary button--small log-stream-toggle'
          data-live={streamControlsLocked || undefined}
          disabled={mutating}
          onClick={toggleLiveStream}
          type='button'
        >
          <AnimatedIcon active={streamControlsLocked} name='subscriptions' size={16} />
          {streamControlsLocked
            ? t('observability.logs.stopStream', { defaultValue: 'Stop live stream' })
            : t('observability.logs.startStream', { defaultValue: 'Start live stream' })}
        </button>
        <button
          aria-describedby={hasUnsupportedClearFilter ? 'log-clear-scope-note' : undefined}
          className='button button--quiet button--small log-clear-trigger'
          disabled={hasUnsupportedClearFilter || mutating}
          onClick={() => {
            setActionError('');
            setActionMessage('');
            setDestructiveAction({ kind: 'clear' });
          }}
          title={hasUnsupportedClearFilter
            ? t('observability.logs.clearDisabled', { defaultValue: 'Remove code, level, and start-time filters before clearing; the server can clear only by source and before time' })
            : t('observability.logs.clearTitle', { defaultValue: 'Clear persisted events in the exact selected source and before-time scope' })}
          type='button'
        >
          <Trash2 aria-hidden='true' size={15} />
          {t('observability.logs.clear', { defaultValue: 'Clear filtered events' })}
        </button>
      </div>

      <div className='log-feedback'>
        <span className='log-stream-status' data-state={displayedStreamState} role='status'>
          <span aria-hidden='true' />
          {streamStatusLabel(displayedStreamState, t)}
        </span>
        {!hasUnsupportedClearFilter
          ? null
          : (
              <span className='log-filter-note' id='log-clear-scope-note'>
                {t('observability.logs.clearNote', { defaultValue: 'Clear is unavailable because the server can safely scope this action only by source and before time.' })}
              </span>
            )}
        {actionMessage === '' ? null : <span role='status'>{actionMessage}</span>}
      </div>

      {destructiveAction?.kind === 'clear'
        ? (
            <div
              aria-label={t('observability.logs.confirmClearLabel', { defaultValue: 'Confirm clearing filtered events' })}
              className='log-confirmation'
              role='group'
            >
              <span>
                {t('observability.logs.confirmClear', {
                  defaultValue: 'Clear every persisted event in {{scope}}, including records outside this 50-item view?',
                  scope: clearScope,
                })}
              </span>
              <button
                autoFocus
                className='button button--secondary button--small'
                disabled={mutating}
                onClick={() => setDestructiveAction(null)}
                type='button'
              >
                {t('observability.logs.keep', { defaultValue: 'Keep events' })}
              </button>
              <button
                className='button button--danger button--small'
                disabled={mutating}
                onClick={() => void clearFilteredEvents()}
                type='button'
              >
                {mutating
                  ? t('observability.logs.clearing', { defaultValue: 'Clearing…' })
                  : t('observability.logs.confirmClearAction', { defaultValue: 'Confirm clear' })}
              </button>
            </div>
          )
        : null}

      {loadError === null
        ? null
        : <ErrorNotice error={loadError} title={t('observability.error.logs', { defaultValue: 'Durable log API unavailable' })} />}
      <ActionError message={streamError} title={t('observability.error.logStream', { defaultValue: 'Live log stream failed' })} />
      <ActionError message={actionError} title={t('observability.error.logAction', { defaultValue: 'Log action failed' })} />

      <div className='observability-list-shell'>
        <div className='observability-master'>
          {logs === null
            ? (
                <div className='observability-skeleton' aria-busy='true'>
                  <span />
                  <span />
                  <span />
                  <p>{t('observability.logs.loading', { defaultValue: 'Loading durable events…' })}</p>
                </div>
              )
            : null}
          {logs?.length === 0
            ? (
                <div className='empty-state'>
                  <strong>{t('observability.logs.empty', { defaultValue: 'No matching events.' })}</strong>
                  <p>{t('observability.logs.emptyDetail', { defaultValue: 'The panel does not synthesize routine log entries.' })}</p>
                </div>
              )
            : null}
          {logs && logs.length > 0
            ? (
                <ol className='log-list'>
                  {logs.map((entry) => {
                    const isSelected = selected?.id === entry.id;
                    return (
                      <li data-selected={isSelected || undefined} key={entry.id}>
                        <button
                          aria-expanded={isSelected}
                          aria-haspopup='dialog'
                          onClick={() => void inspect(entry)}
                          type='button'
                        >
                          <span className={`log-level log-level--${entry.level}`}>
                            {t(`observability.logs.levelOption.${entry.level}`, { defaultValue: entry.level })}
                          </span>
                          <span className='log-list__content'>
                            <strong>{entry.message}</strong>
                            <small>
                              {t(`observability.logs.sourceOption.${entry.source}`, { defaultValue: entry.source })}
                              {' · '}
                              {entry.code}
                            </small>
                          </span>
                          <time dateTime={entry.time}>{formatLogTime(entry.time, locale)}</time>
                          <ChevronRight
                            aria-hidden='true'
                            className='log-list__chevron'
                            size={17}
                          />
                        </button>
                      </li>
                    );
                  })}
                </ol>
              )
            : null}
          {nextCursor === undefined
            ? null
            : (
                <button
                  className='button button--quiet log-load-more'
                  disabled={loadingOlder}
                  onClick={() => void load(undefined, nextCursor, true)}
                  type='button'
                >
                  {loadingOlder
                    ? t('observability.logs.loadingOlder', { defaultValue: 'Loading…' })
                    : t('observability.logs.loadOlder', { defaultValue: 'Load older' })}
                </button>
              )}
        </div>
      </div>

      <Sheet
        onOpenChange={(open) => {
          if (!open) closeDetail();
        }}
        open={selected !== null}
      >
        <SheetContent className='observability-detail-sheet' side='right'>
          <SheetHeader className='observability-detail-sheet__header'>
            <SheetTitle>{selected?.code ?? t('observability.logs.detailLabel', { defaultValue: 'Log detail' })}</SheetTitle>
            <SheetDescription>
              {selected === null
                ? t('observability.logs.selectDetail', { defaultValue: 'Source, severity, time, and sanitized metadata appear here.' })
                : `${t(`observability.logs.sourceOption.${selected.source}`, { defaultValue: selected.source })} · ${formatLogDate(selected.time, locale)}`}
            </SheetDescription>
          </SheetHeader>
          <div
            aria-busy={detailLoading}
            aria-live='polite'
            className='observability-detail-sheet__body'
          >
            {detailLoading
              ? <p className='observability-detail-sheet__status' role='status'>{t('observability.logs.loadingDetail', { defaultValue: 'Loading complete event…' })}</p>
              : null}
            <ActionError message={detailError} title={t('observability.error.logDetail', { defaultValue: 'Log detail failed' })} />
            {selected === null
              ? null
              : (
                  <div className='observability-detail' key={selected.id}>
                    <div className='observability-detail__heading'>
                      <p className='observability-detail__message'>{selected.message}</p>
                      <span className={`log-level log-level--${selected.level}`}>
                        {t(`observability.logs.levelOption.${selected.level}`, { defaultValue: selected.level })}
                      </span>
                    </div>
                    <dl>
                      <div>
                        <dt>{t('observability.logs.eventID', { defaultValue: 'Event ID' })}</dt>
                        <dd>{selected.id}</dd>
                      </div>
                      <div>
                        <dt>{t('observability.logs.source', { defaultValue: 'Source' })}</dt>
                        <dd>{t(`observability.logs.sourceOption.${selected.source}`, { defaultValue: selected.source })}</dd>
                      </div>
                      <div>
                        <dt>{t('observability.logs.recorded', { defaultValue: 'Recorded' })}</dt>
                        <dd>{formatLogDate(selected.time, locale)}</dd>
                      </div>
                    </dl>
                    <details>
                      <summary>{t('observability.logs.metadata', { defaultValue: 'Sanitized metadata' })}</summary>
                      <pre>{JSON.stringify(selected.metadata, null, 2)}</pre>
                    </details>
                    <div className='observability-detail__actions'>
                      {destructiveAction?.kind === 'delete'
                        && destructiveAction.entry.id === selected.id
                        ? (
                            <div
                              aria-label={t('observability.logs.confirmDeleteLabel', {
                                defaultValue: 'Confirm deletion of {{code}}',
                                code: selected.code,
                              })}
                              className='log-confirmation log-confirmation--detail'
                              role='group'
                            >
                              <span>{t('observability.logs.confirmDelete', { defaultValue: 'Delete this persisted event?' })}</span>
                              <button
                                autoFocus
                                className='button button--secondary button--small'
                                disabled={mutating}
                                onClick={() => setDestructiveAction(null)}
                                type='button'
                              >
                                {t('observability.logs.keepOne', { defaultValue: 'Keep event' })}
                              </button>
                              <button
                                className='button button--danger button--small'
                                disabled={mutating}
                                onClick={() => void deleteSelectedEvent()}
                                type='button'
                              >
                                {mutating
                                  ? t('observability.logs.deleting', { defaultValue: 'Deleting…' })
                                  : t('observability.logs.confirmDeleteAction', { defaultValue: 'Confirm delete' })}
                              </button>
                            </div>
                          )
                        : (
                            <button
                              className='button button--quiet button--small log-delete-trigger'
                              disabled={mutating}
                              onClick={() => {
                                setActionError('');
                                setActionMessage('');
                                setDestructiveAction({ entry: selected, kind: 'delete' });
                              }}
                              type='button'
                            >
                              <Trash2 aria-hidden='true' size={15} />
                              {t('observability.logs.delete', { defaultValue: 'Delete event' })}
                            </button>
                          )}
                    </div>
                  </div>
                )}
          </div>
        </SheetContent>
      </Sheet>
    </section>
  );
}
