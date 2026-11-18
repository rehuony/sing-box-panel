import { cn } from '@shared/lib/utils';
import { useTranslation } from 'react-i18next';
import { Input } from '@shared/components/ui/input';
import { Button } from '@shared/components/ui/button';
import { useEffect, useMemo, useRef, useState } from 'react';
import { ScrollArea } from '@shared/components/ui/scroll-area';
import { Tooltip, TooltipContent, TooltipTrigger } from '@shared/components/ui/tooltip';
import { ArrowDownToLine, Eraser, Pause, Play, SearchX, SquareTerminal, Trash2 } from 'lucide-react';
import { Empty, EmptyContent, EmptyHeader, EmptyMedia, EmptyTitle } from '@shared/components/ui/empty';
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@shared/components/ui/alert-dialog';

import { ErrorNotice } from '@/components/error-notice';
import { SelectField } from '@/components/select-field';
import { LoadingState } from '@/components/loading-state';

import type { CoreLevel } from './core-log-lines';

import { useCoreLogs } from './use-core-logs';
import { coreLevels, parseCoreLines } from './core-log-lines';

const levelText: Record<CoreLevel | 'unknown', string> = {
  trace: 'text-[var(--color-text-faint)]',
  debug: 'text-muted-foreground',
  info: 'text-[var(--color-info-text)]',
  warn: 'text-[var(--color-warning-text)]',
  error: 'text-[var(--color-danger-text)]',
  fatal: 'text-[var(--color-danger-text)]',
  panic: 'text-[var(--color-danger-text)]',
  unknown: 'text-[var(--color-text-faint)]',
};

/** The node's core log: today's file follows live, earlier files read as history. */
export function NodeLogsPage() {
  const { t } = useTranslation();
  const log = useCoreLogs(true);
  const [level, setLevel] = useState('');
  const [search, setSearch] = useState('');
  const [deleteTarget, setDeleteTarget] = useState('');
  const [deleting, setDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState<unknown>(null);
  const [clearTarget, setClearTarget] = useState('');
  const [clearError, setClearError] = useState<unknown>(null);
  const [clearedFile, setClearedFile] = useState('');
  const cleared = clearedFile === log.file && log.file !== '';
  const viewportRef = useRef<HTMLDivElement>(null);
  const followRef = useRef(true);
  const lines = useMemo(
    () =>
      parseCoreLines({ first: log.firstLine, text: log.text }).filter(
        (line) =>
          (!level || line.level === level)
          && `${line.prefix}${line.message}`.toLowerCase().includes(search.toLowerCase()),
      ),
    [log.firstLine, log.text, level, search],
  );
  useEffect(() => {
    followRef.current = true;
  }, [log.file]);
  useEffect(() => {
    const target = viewportRef.current;
    if (target && followRef.current) target.scrollTop = target.scrollHeight;
  }, [lines]);
  const state = log.paused
    ? 'paused'
    : log.connected
      ? 'live'
      : log.current
        ? 'connecting'
        : 'archive';
  const canToggle = log.current || log.paused;
  const mutating = deleting || log.clearing;
  async function clearFile() {
    setClearError(null);
    try {
      await log.clear(clearTarget);
      setClearedFile(clearTarget);
      setClearTarget('');
      followRef.current = true;
    } catch (reason) {
      setClearError(reason);
    }
  }
  async function deleteFile() {
    setDeleting(true);
    setDeleteError(null);
    try {
      await log.deleteFile(deleteTarget);
      setDeleteTarget('');
    } catch (reason) {
      setDeleteError(reason);
    } finally {
      setDeleting(false);
    }
  }
  return (
    <section className='flex min-h-0 min-w-0 flex-1 flex-col gap-[var(--workspace-content-gap)]'>
      <h1 className='sr-only'>{t('node.tabs.logs')}</h1>
      <div className='flex flex-wrap items-center gap-3'>
        <Input
          aria-label={t('productLogs.search')}
          className='w-full sm:w-70'
          placeholder={t('productLogs.search')}
          value={search}
          onChange={(event) => setSearch(event.target.value)}
        />
        <SelectField
          aria-label={t('productLogs.file')}
          className='min-w-0 flex-1 sm:w-auto sm:flex-none'
          value={log.file}
          disabled={mutating}
          onValueChange={log.selectFile}
          items={log.files.length ? log.files.map((file) => ({ value: file.name, label: `${file.name.slice(0, 10)} · ${(file.size / 1048576).toFixed(1)} MB` })) : [{ value: '', label: t('productLogs.noFiles') }]}
        />
        <SelectField
          aria-label={t('productLogs.level')}
          className='min-w-28'
          value={level}
          onValueChange={setLevel}
          items={[{ value: '', label: t('productLogs.allLevels') }, ...coreLevels.map((value) => ({ value, label: value.toUpperCase() }))]}
        />
      </div>
      {log.error != null && <ErrorNotice error={log.error} title={t('productLogs.unavailable')} />}
      {deleteError != null && <ErrorNotice error={deleteError} title={t('productLogs.deleteFailed')} />}
      {clearError != null && <ErrorNotice error={clearError} title={t('productLogs.clearFailed')} />}
      <AlertDialog open={Boolean(clearTarget)} onOpenChange={(open) => {
        if (!open && !log.clearing) setClearTarget('');
      }}>
        <AlertDialogContent showCloseButton={!log.clearing}>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('productLogs.clear')}</AlertDialogTitle>
            <AlertDialogDescription>{t('productLogs.clearConfirmation', { file: clearTarget })}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={log.clearing}>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction variant='destructive' disabled={log.clearing} onClick={() => void clearFile()}>
              {t(log.clearing ? 'productLogs.clearing' : 'productLogs.clear')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
      <AlertDialog open={Boolean(deleteTarget)} onOpenChange={(open) => {
        if (!open && !deleting) setDeleteTarget('');
      }}>
        <AlertDialogContent showCloseButton={!deleting}>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('productLogs.deleteFile')}</AlertDialogTitle>
            <AlertDialogDescription>{t('productLogs.deleteDescription', { file: deleteTarget })}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleting}>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction variant='destructive' disabled={deleting} onClick={() => void deleteFile()}>
              {t(deleting ? 'productLogs.deleting' : 'productLogs.deleteFile')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
      <div className='relative flex min-h-80 flex-1 flex-col overflow-hidden rounded-[var(--radius-card)] border bg-[var(--color-surface-raised)]'>
        {lines.length > 0 && (
          <div className='floating-toolbar' role='group' aria-label={t('productLogs.actions')}>
            <Tooltip>
              <TooltipTrigger render={(
                <span
                  className='inline-flex h-[var(--control-height-xs)] w-6 items-center justify-center rounded-full text-muted-foreground'
                  role='status'
                  aria-label={t(`productLogs.${state}`)}
                  tabIndex={0}
                />
              )}>
                <i aria-hidden='true' className='size-2 rounded-full bg-current' />
              </TooltipTrigger>
              <TooltipContent>{t(`productLogs.${state}`)}</TooltipContent>
            </Tooltip>
            <Tooltip>
              <TooltipTrigger render={(
                <Button
                  size='icon-xs'
                  variant='ghost'
                  aria-label={t('productLogs.liveUpdates')}
                  aria-pressed={canToggle && !log.paused}
                  disabled={!canToggle || mutating}
                  onClick={() => log.setPaused(!log.paused)}
                />
              )}>
                {canToggle && !log.paused ? <Pause aria-hidden='true' /> : <Play aria-hidden='true' />}
              </TooltipTrigger>
              <TooltipContent>{t(log.paused || !canToggle ? 'productLogs.resume' : 'productLogs.pause')}</TooltipContent>
            </Tooltip>
            <Tooltip>
              <TooltipTrigger render={(
                <Button
                  size='icon-xs'
                  variant='ghost'
                  aria-label={t('productLogs.scrollToBottom')}
                  onClick={() => {
                    followRef.current = true;
                    const target = viewportRef.current;
                    if (target) target.scrollTop = target.scrollHeight;
                  }}
                />
              )}>
                <ArrowDownToLine aria-hidden='true' />
              </TooltipTrigger>
              <TooltipContent>{t('productLogs.scrollToBottom')}</TooltipContent>
            </Tooltip>
            <Tooltip>
              <TooltipTrigger render={(
                <Button
                  size='icon-xs'
                  variant='ghost'
                  aria-label={t('productLogs.clear')}
                  disabled={!log.file || mutating}
                  onClick={() => {
                    setClearError(null);
                    setClearTarget(log.file);
                  }}
                />
              )}>
                <Eraser aria-hidden='true' />
              </TooltipTrigger>
              <TooltipContent>{t('productLogs.clearDescription')}</TooltipContent>
            </Tooltip>
            {log.deletable && (
              <Tooltip>
                <TooltipTrigger render={(
                  <Button
                    size='icon-xs'
                    variant='ghost'
                    aria-label={t('productLogs.deleteFile')}
                    disabled={mutating}
                    onClick={() => setDeleteTarget(log.file)}
                  />
                )}>
                  <Trash2 aria-hidden='true' />
                </TooltipTrigger>
                <TooltipContent>{t('productLogs.deleteFile')}</TooltipContent>
              </Tooltip>
            )}
          </div>
        )}
        <ScrollArea
          className='flex-1'
          viewportClassName={cn('flex flex-col', lines.length ? 'px-4 pt-3 pb-4' : 'p-12')}
          viewportProps={{
            'aria-label': t('productLogs.core'),
            'onScroll': (event) => {
              const node = event.currentTarget;
              followRef.current = node.scrollHeight - node.scrollTop - node.clientHeight < 48;
            },
            'role': 'region',
            'tabIndex': 0,
          }}
          viewportRef={viewportRef}
        >
          {!lines.length
            ? log.loading
              ? <LoadingState label={t('productLogs.loading')} fullScreen={false} />
              : (
                  <Empty className='p-0' role='status'>
                    <EmptyHeader>
                      <EmptyMedia className='mb-3 size-16 rounded-[var(--radius-window)] border bg-card text-muted-foreground [&_svg]:size-8'>
                        {!log.files.length
                          ? <SquareTerminal aria-hidden='true' strokeWidth={1.5} />
                          : <SearchX aria-hidden='true' strokeWidth={1.5} />}
                      </EmptyMedia>
                      <EmptyTitle>
                        {t(!log.files.length ? 'productLogs.emptyCore' : cleared && !log.text ? 'productLogs.cleared' : 'productLogs.empty')}
                      </EmptyTitle>
                    </EmptyHeader>
                    {log.paused && !log.text.trim() && (
                      <EmptyContent>
                        <Button size='sm' variant='ghost' disabled={mutating} onClick={() => log.setPaused(false)}>
                          <Play aria-hidden='true' data-icon='inline-start' />
                          {t('productLogs.resume')}
                        </Button>
                      </EmptyContent>
                    )}
                  </Empty>
                )
            : (
                <pre className='m-0 flex-none font-mono text-compact [tab-size:2] whitespace-pre'>
                  {lines.map((line) => (
                    <span className={levelText[line.level]} key={line.id}>
                      <span className='text-muted-foreground'>{line.prefix}</span>
                      {line.message}
                      {'\n'}
                    </span>
                  ))}
                </pre>
              )}
        </ScrollArea>
      </div>
    </section>
  );
}
