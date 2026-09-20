import { Pause, Play } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useEffect, useMemo, useRef, useState } from 'react';

import { Badge } from '@/components/ui/badge';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { ErrorNotice } from '@/components/error-notice';
import { SelectField } from '@/components/select-field';
import { ToolbarActions } from '@/components/workspace-toolbar';

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
              onValueChange={log.selectFile}
              items={log.files.length ? log.files.map((file) => ({ value: file.name, label: `${file.name.replace('.log', '')} · ${(file.size / 1048576).toFixed(1)} MB` })) : [{ value: '', label: t('productLogs.noFiles') }]}
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
      <div className='native-log'>
        <div className='native-log__status'>
          {log.current && (
            <Button
              aria-label={t(log.paused ? 'productLogs.resume' : 'productLogs.pause')}
              size='icon'
              variant='ghost'
              onClick={() => log.setPaused(!log.paused)}
            >
              {log.paused ? <Play /> : <Pause />}
            </Button>
          )}
          <Badge className='native-log__badge' variant={state === 'live' ? 'success' : state === 'paused' ? 'warning' : state === 'connecting' ? 'info' : 'secondary'}>
            <i aria-hidden='true' />
            {t(`productLogs.${state}`)}
          </Badge>
        </div>
        <div
          ref={viewportRef}
          className='native-log__output'
          tabIndex={0}
          role='region'
          aria-label={t('productLogs.core')}
          onScroll={(event) => {
            const node = event.currentTarget;
            followRef.current = node.scrollHeight - node.scrollTop - node.clientHeight < 48;
          }}
        >
          {!lines.length && (
            <p className='log-empty'>
              {t(
                log.loading
                  ? 'productLogs.loading'
                  : !log.files.length
                      ? 'productLogs.emptyCore'
                      : 'productLogs.empty',
              )}
            </p>
          )}
          <pre>
            {lines.map((line) => (
              <span className={`native-log__line native-log__line--${line.level}`} key={line.id}>
                <span className='native-log__timestamp'>{line.prefix}</span>
                {line.message}
                {'\n'}
              </span>
            ))}
          </pre>
        </div>
      </div>
    </div>
  );
}
