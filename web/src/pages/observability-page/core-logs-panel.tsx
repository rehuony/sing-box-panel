import { Pause, Play } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useEffect, useMemo, useRef, useState } from 'react';

import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { ErrorNotice } from '@/components/error-notice';

import { useCoreLogs } from './use-core-logs';
import { coreLevels, parseCoreLines } from './core-log-lines';

export function CoreLogsPanel() {
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
      <div className='log-toolbar'>
        <Input
          aria-label={t('productLogs.search')}
          placeholder={t('productLogs.search')}
          value={search}
          onChange={(event) => setSearch(event.target.value)}
        />
        <div className='log-toolbar__filters'>
          <select
            aria-label={t('productLogs.file')}
            value={log.file}
            onChange={(event) => log.selectFile(event.target.value)}
          >
            {!log.files.length && <option value=''>{t('productLogs.noFiles')}</option>}
            {log.files.map((file) => (
              <option key={file.name} value={file.name}>
                {file.name.replace('.log', '')}
                {' '}
                ·
                {(file.size / 1048576).toFixed(1)}
                {' '}
                MB
              </option>
            ))}
          </select>
          <select
            aria-label={t('productLogs.level')}
            value={level}
            onChange={(event) => setLevel(event.target.value)}
          >
            <option value=''>ALL</option>
            {coreLevels.map((value) => (
              <option key={value} value={value}>
                {value.toUpperCase()}
              </option>
            ))}
          </select>
        </div>
      </div>
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
          <span className={`native-log__badge native-log__badge--${state}`}>
            <i />
            {t(`productLogs.${state}`)}
          </span>
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
