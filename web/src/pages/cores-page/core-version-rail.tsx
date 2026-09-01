import { useTranslation } from 'react-i18next';

import { Button } from '@/components/ui/button';

export interface CoreVersionEntry {
  version: string;
  catalogCount: number;
  installedCount: number;
}

interface CoreVersionRailProps {
  locale: string;
  loadingOlder: boolean;
  showLoadOlder: boolean;
  onLoadOlder: () => void;
  selectedVersion: string;
  entries: CoreVersionEntry[];
  onSelect: (version: string) => void;
}

export function CoreVersionRail({
  entries,
  loadingOlder,
  locale,
  onLoadOlder,
  onSelect,
  selectedVersion,
  showLoadOlder,
}: CoreVersionRailProps) {
  const { t } = useTranslation();

  return (
    <aside className='version-rail-panel' aria-labelledby='version-rail-title'>
      <h2 id='version-rail-title'>{t('cores.versions', { defaultValue: 'Versions' })}</h2>
      <ol className='version-rail'>
        {entries.map((entry) => (
          <li className={entry.version === selectedVersion ? 'is-selected' : undefined} key={entry.version}>
            <button aria-current={entry.version === selectedVersion ? 'true' : undefined} onClick={() => onSelect(entry.version)} type='button'>
              <span className='version-rail__node' aria-hidden='true' />
              <strong>{entry.version}</strong>
              <span>
                {new Intl.NumberFormat(locale).format(entry.installedCount)}
                {' '}
                {t('cores.installed.short', { defaultValue: 'installed' })}
                {' · '}
                {new Intl.NumberFormat(locale).format(entry.catalogCount)}
                {' '}
                {t('cores.catalog.short', { defaultValue: 'catalog' })}
              </span>
            </button>
          </li>
        ))}
      </ol>
      {showLoadOlder
        ? (
            <Button disabled={loadingOlder} onClick={onLoadOlder} variant='outline'>
              {t('cores.loadOlder', { defaultValue: 'Load older' })}
            </Button>
          )
        : null}
    </aside>
  );
}
