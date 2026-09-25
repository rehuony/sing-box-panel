import { useTranslation } from 'react-i18next';
import { useEffect, useMemo, useRef, useState } from 'react';
import { ArrowDownToLine, Eraser, Pause, Play, SearchX, SquareTerminal, Trash2 } from 'lucide-react';

import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { ErrorNotice } from '@/components/error-notice';
import { SelectField } from '@/components/select-field';
import { ToolbarActions } from '@/components/workspace-toolbar';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from '@/components/ui/empty';
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog';

import { useCoreLogs } from './use-core-logs';
import { coreLevels, parseCoreLines } from './core-log-lines';

export function CoreLogsPanel({ active = true, toolbarTarget }: {
  active?: boolean;
  toolbarTarget?: HTMLElement | null;
} = {}) {
  const { t } = useTranslation();
  const log = useCoreLogs();
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
      parseCoreLines(log.text).filter(
        (line) =>
          (!level || line.level === level)
          && `${line.prefix}${line.message}`.toLowerCase().includes(search.toLowerCase()),
      ),
    [log.text, level, search],
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
    <div className='log-workspace'>
      <ToolbarActions active={active} target={toolbarTarget}>
        <div className='log-toolbar workspace-toolbar-content'>
          <Input
            aria-label={t('productLogs.search')}
            placeholder={t('productLogs.search')}
            value={search}
            onChange={(event) => setSearch(event.target.value)}
          />
          <div className='log-toolbar__filters'>
            <SelectField
              aria-label={t('productLogs.file')}
              value={log.file}
              disabled={mutating}
              onValueChange={log.selectFile}
              items={log.files.length ? log.files.map((file) => ({ value: file.name, label: `${file.name.slice(0, 10)} · ${(file.size / 1048576).toFixed(1)} MB` })) : [{ value: '', label: t('productLogs.noFiles') }]}
            />
            <SelectField
              aria-label={t('productLogs.level')}
              value={level}
              onValueChange={(value) => {
                setLevel(value);
              }}
              items={[{ value: '', label: 'ALL' }, ...coreLevels.map((value) => ({ value, label: value.toUpperCase() }))]}
            />
          </div>

        </div>
      </ToolbarActions>
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
      <div className='native-log'>
        {lines.length > 0 && (
          <div className='floating-toolbar' role='group' aria-label={t('productLogs.actions')}>
            <Tooltip>
              <TooltipTrigger render={(
                <span
                  className='native-log__status'
                  role='status'
                  aria-label={t(`productLogs.${state}`)}
                  tabIndex={0}
                />
              )}>
                <i aria-hidden='true' />
              </TooltipTrigger>
              <TooltipContent>{t(`productLogs.${state}`)}</TooltipContent>
            </Tooltip>
            <Tooltip>
              <TooltipTrigger render={(
                <Button
                  size='icon-sm'
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
                  size='icon-sm'
                  variant='ghost'
                  aria-label={t('productLogs.scrollToBottom')}
                  disabled={!lines.length}
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
                  size='icon-sm'
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
                    size='icon-sm'
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
        <div
          ref={viewportRef}
          className='native-log__output'
          data-empty={!lines.length || undefined}
          tabIndex={0}
          role='region'
          aria-label={t('productLogs.core')}
          onScroll={(event) => {
            const node = event.currentTarget;
            followRef.current = node.scrollHeight - node.scrollTop - node.clientHeight < 48;
          }}
        >
          {!lines.length
            ? (
                <Empty className='native-log__empty' role='status'>
                  <EmptyHeader>
                    {!log.loading && (
                      <EmptyMedia className='native-log__empty-icon'>
                        {!log.files.length
                          ? <SquareTerminal aria-hidden='true' strokeWidth={1.5} />
                          : <SearchX aria-hidden='true' strokeWidth={1.5} />}
                      </EmptyMedia>
                    )}
                    <EmptyTitle>
                      {t(log.loading ? 'productLogs.loading' : !log.files.length ? 'productLogs.emptyCore' : cleared && !log.text ? 'productLogs.cleared' : 'productLogs.empty')}
                    </EmptyTitle>
                    {!log.loading && (
                      <EmptyDescription>
                        {t(!log.files.length ? 'productLogs.emptyCoreDescription' : cleared && !log.text ? 'productLogs.clearedDescription' : 'productLogs.emptyDescription')}
                      </EmptyDescription>
                    )}
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
                <pre>
                  {lines.map((line) => (
                    <span className={`native-log__line native-log__line--${line.level}`} key={line.id}>
                      <span className='native-log__timestamp'>{line.prefix}</span>
                      {line.message}
                      {'\n'}
                    </span>
                  ))}
                </pre>
              )}
        </div>
      </div>
    </div>
  );
}
