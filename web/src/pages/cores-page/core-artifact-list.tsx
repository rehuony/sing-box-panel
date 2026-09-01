import { useTranslation } from 'react-i18next';

import type { CoreArtifact } from '@/api/api-client';

import { Button } from '@/components/ui/button';

interface CoreArtifactListProps {
  artifacts: CoreArtifact[];
  onInspect: (artifact: CoreArtifact) => void;
}

function compactDigest(digest: string) {
  return digest.length > 20 ? `${digest.slice(0, 17)}…` : digest;
}

export function CoreArtifactList({ artifacts, onInspect }: CoreArtifactListProps) {
  const { t } = useTranslation();

  return (
    <div className='core-card-list'>
      {artifacts.map((artifact) => {
        const label = `${artifact.exact_version} ${artifact.arch}/${artifact.variant} (${artifact.id})`;
        return (
          <article className='core-card' key={artifact.id}>
            <span className={`core-card__state core-card__state--${artifact.verification_state}`} aria-hidden='true' />
            <div>
              <strong>{`${artifact.arch} · ${artifact.variant}`}</strong>
              <span>
                {t(`cores.state.${artifact.verification_state}`, { defaultValue: artifact.verification_state })}
                {' · '}
                {t(`cores.source.${artifact.source_kind}`, { defaultValue: artifact.source_kind })}
              </span>
              <code>{compactDigest(artifact.archive_sha256)}</code>
            </div>
            <Button aria-label={t('cores.detail.viewLabel', { artifact: label })} onClick={() => onInspect(artifact)} variant='outline'>
              {t('cores.details', { defaultValue: 'Details' })}
            </Button>
          </article>
        );
      })}
    </div>
  );
}
