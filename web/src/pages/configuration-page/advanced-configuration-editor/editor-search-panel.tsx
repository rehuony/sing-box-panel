import { useTranslation } from 'react-i18next';
import { useLayoutEffect, useMemo, useRef, useState, useSyncExternalStore } from 'react';
import { ArrowDown, ArrowUp, CaseSensitive, ChevronDown, ChevronRight, ListFilter, Regex, Replace, ReplaceAll, WholeWord, X } from 'lucide-react';
import { closeSearchPanel, findNext, findPrevious, getSearchQuery, replaceAll, replaceNext, SearchQuery, selectMatches, setSearchQuery } from '@codemirror/search';

import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';

import type { SearchPanelHost } from './search-panel-host';

export function EditorSearchPanel({ host }: { host: SearchPanelHost }) {
  const { t } = useTranslation();
  const state = useSyncExternalStore(host.subscribe, host.getSnapshot);
  const query = getSearchQuery(state);
  const [showReplace, setShowReplace] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const doc = state.doc;
  const matches = useMemo(() => {
    const ranges: { from: number; to: number }[] = [];
    if (query.valid) {
      const cursor = query.getCursor(doc);
      for (let match = cursor.next(); !match.done && ranges.length <= 10_000; match = cursor.next()) {
        ranges.push(match.value);
      }
    }
    return ranges;
  }, [query, doc]); // Selection changes do not require another document scan.
  const { from, to } = state.selection.main;
  const current = matches.findIndex(match => match.from === from && match.to === to) + 1;
  const invalid = query.search !== '' && !query.valid;
  const actionable = query.valid && matches.length > 0;

  useLayoutEffect(() => {
    inputRef.current?.focus({ preventScroll: true });
    inputRef.current?.select();
  }, []);

  function update(patch: Partial<Pick<SearchQuery, 'search' | 'replace' | 'caseSensitive' | 'wholeWord' | 'regexp'>>) {
    host.view.dispatch({ effects: setSearchQuery.of(new SearchQuery({ ...query, ...patch })) });
  }

  return (
    <section className='editor-search' aria-label={t('configuration.advanced.search')} onKeyDown={event => {
      if (event.key === 'Escape') {
        event.preventDefault();
        event.stopPropagation();
        closeSearchPanel(host.view);
      }
    }}>
      <Button className='editor-search__expand' size='icon-sm' variant='ghost' type='button' aria-label={t('configuration.advanced.toggleReplace')} aria-expanded={showReplace} onClick={() => setShowReplace(!showReplace)}>
        {showReplace ? <ChevronDown /> : <ChevronRight />}
      </Button>
      <div className='editor-search__row'>
        <div className='editor-search__field'>
          <Input ref={inputRef} main-field='true' aria-label={t('configuration.advanced.find')} placeholder={t('configuration.advanced.find')} aria-invalid={invalid} autoComplete='off' spellCheck={false} value={query.search} onChange={event => update({ search: event.target.value })} onKeyDown={event => {
            if (event.key === 'Enter') {
              event.preventDefault();
              if (actionable) (event.shiftKey ? findPrevious : findNext)(host.view);
            }
          }} />
          {([
            ['caseSensitive', CaseSensitive, 'matchCase'],
            ['wholeWord', WholeWord, 'wholeWord'],
            ['regexp', Regex, 'regexp'],
          ] as const).map(([key, Icon, label]) => (
            <Button key={key} size='icon-sm' variant='ghost' type='button' aria-label={t(`configuration.advanced.${label}`)} title={t(`configuration.advanced.${label}`)} aria-pressed={query[key]} onClick={() => update({ [key]: !query[key] })}><Icon /></Button>
          ))}
        </div>
        <div className='editor-search__actions'>
          <span className='editor-search__count' role='status'>
            {invalid ? t('configuration.advanced.invalidRegex') : !query.search ? '' : matches.length === 0 ? t('configuration.advanced.noResults') : matches.length > 10_000 ? '10000+' : current ? `${current} / ${matches.length}` : t('configuration.advanced.matches', { count: matches.length })}
          </span>
          <Button size='icon-sm' variant='ghost' type='button' aria-label={t('configuration.advanced.previous')} title={t('configuration.advanced.previous')} disabled={!actionable} onClick={() => findPrevious(host.view)}><ArrowUp /></Button>
          <Button size='icon-sm' variant='ghost' type='button' aria-label={t('configuration.advanced.next')} title={t('configuration.advanced.next')} disabled={!actionable} onClick={() => findNext(host.view)}><ArrowDown /></Button>
          <Button size='icon-sm' variant='ghost' type='button' aria-label={t('configuration.advanced.selectAll')} title={t('configuration.advanced.selectAll')} disabled={!actionable} onClick={() => selectMatches(host.view)}><ListFilter /></Button>
          <Button size='icon-sm' variant='ghost' type='button' aria-label={t('common.close')} title={t('common.close')} onClick={() => closeSearchPanel(host.view)}><X /></Button>
        </div>
      </div>
      {showReplace && (
        <div className='editor-search__row'>
          <div className='editor-search__field'>
            <Input aria-label={t('configuration.advanced.replace')} placeholder={t('configuration.advanced.replace')} autoComplete='off' spellCheck={false} disabled={state.readOnly} value={query.replace} onChange={event => update({ replace: event.target.value })} onKeyDown={event => {
              if (event.key === 'Enter') {
                event.preventDefault();
                if (actionable && !state.readOnly) replaceNext(host.view);
              }
            }} />
          </div>
          <div className='editor-search__actions'>
            <Button size='icon-sm' variant='ghost' type='button' aria-label={t('configuration.advanced.replace')} title={t('configuration.advanced.replace')} disabled={!actionable || state.readOnly} onClick={() => replaceNext(host.view)}><Replace /></Button>
            <Button size='icon-sm' variant='ghost' type='button' aria-label={t('configuration.advanced.replaceAll')} title={t('configuration.advanced.replaceAll')} disabled={!actionable || state.readOnly} onClick={() => replaceAll(host.view)}><ReplaceAll /></Button>
          </div>
        </div>
      )}
    </section>
  );
}
