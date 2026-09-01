import { Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';

import type { CoreArtifact, StartupArtifactSummary } from '@/api/api-client';

import { Button } from '@/components/ui/button';
import { ErrorNotice } from '@/components/error-notice';
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';

import type { CoreArtifactInspection } from './use-core-library-state';

type ArtifactAction = 'quarantined' | 'remove' | 'revoked';

interface CoreArtifactDetailProps {
  locale: string;
  onClose: () => void;
  inspectionID: string | null;
  pending: ReadonlySet<string>;
  inspection: CoreArtifactInspection | null;
  acceptedStartupTasks: Readonly<Record<string, string>>;
  onCheckStartup: (artifact: StartupArtifactSummary) => void;
  onRequestAction: (action: ArtifactAction, artifact: CoreArtifact) => void;
}

function formatTimestamp(timestamp: string, locale: string) {
  return new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'short' })
    .format(new Date(timestamp));
}

export function CoreArtifactDetail({
  acceptedStartupTasks,
  inspection,
  inspectionID,
  locale,
  onCheckStartup,
  onClose,
  onRequestAction,
  pending,
}: CoreArtifactDetailProps) {
  const { t } = useTranslation();
  const artifact = inspection?.artifact;
  const startupArtifacts = inspection?.startupArtifacts;
  const startupError = inspection?.startupError;

  return (
    <Sheet open={inspectionID !== null} onOpenChange={(open) => {
      if (!open) onClose();
    }}>
      <SheetContent className='core-detail-sheet' side='right'>
        <SheetHeader>
          <SheetTitle>{artifact?.exact_version ?? t('cores.details', { defaultValue: 'Artifact details' })}</SheetTitle>
          <SheetDescription>{inspectionID}</SheetDescription>
        </SheetHeader>
        {inspection === null
          ? <div className='inline-loading'>{t('cores.detail.loading', { defaultValue: 'Reading immutable evidence…' })}</div>
          : null}
        {inspection?.artifactError === undefined
          ? null
          : (
              <ErrorNotice
                error={inspection.artifactError}
                title={t('cores.error.detail', { defaultValue: 'Artifact details are unavailable' })}
              />
            )}
        {artifact === undefined
          ? null
          : (
              <div className='core-detail-sheet__body'>
                <dl>
                  <div>
                    <dt>{t('cores.detail.id')}</dt>
                    <dd>{artifact.id}</dd>
                  </div>
                  <div>
                    <dt>{t('cores.detail.reportedVersion', { defaultValue: 'Reported version' })}</dt>
                    <dd>{artifact.reported_version}</dd>
                  </div>
                  <div>
                    <dt>{t('cores.detail.installed', { defaultValue: 'Installed' })}</dt>
                    <dd>{formatTimestamp(artifact.created_at, locale)}</dd>
                  </div>
                  <div>
                    <dt>{t('cores.detail.archiveSha')}</dt>
                    <dd><code>{artifact.archive_sha256}</code></dd>
                  </div>
                  <div>
                    <dt>{t('cores.detail.binarySha', { defaultValue: 'Binary SHA-256' })}</dt>
                    <dd><code>{artifact.binary_sha256}</code></dd>
                  </div>
                  <div>
                    <dt>{t('cores.detail.path', { defaultValue: 'Binary path' })}</dt>
                    <dd><code>{artifact.binary_path}</code></dd>
                  </div>
                  <div>
                    <dt>{t('cores.detail.features', { defaultValue: 'Features' })}</dt>
                    <dd><code>{JSON.stringify(artifact.feature_fingerprint)}</code></dd>
                  </div>
                </dl>
                <div className='core-detail-sheet__actions'>
                  {artifact.verification_state === 'verified'
                    ? (
                        <Button
                          disabled={pending.has(`quarantined:${artifact.id}`)}
                          onClick={() => onRequestAction('quarantined', artifact)}
                          variant='outline'
                        >
                          {t('cores.action.quarantine', { defaultValue: 'Quarantine' })}
                        </Button>
                      )
                    : null}
                  {artifact.verification_state !== 'revoked'
                    ? (
                        <Button
                          disabled={pending.has(`revoked:${artifact.id}`)}
                          onClick={() => onRequestAction('revoked', artifact)}
                          variant='destructive'
                        >
                          {t('cores.action.revoke', { defaultValue: 'Revoke' })}
                        </Button>
                      )
                    : null}
                  <Button
                    disabled={pending.has(`remove:${artifact.id}`)}
                    onClick={() => onRequestAction('remove', artifact)}
                    variant='destructive'
                  >
                    {t('cores.action.remove', { defaultValue: 'Remove' })}
                  </Button>
                </div>
                <section>
                  <h3>{t('cores.startup.title', { defaultValue: 'Startup artifacts' })}</h3>
                  {startupError === undefined
                    ? null
                    : (
                        <ErrorNotice
                          error={startupError}
                          title={t('cores.error.startup', { defaultValue: 'Startup artifacts are unavailable' })}
                        />
                      )}
                  <ul className='startup-artifact-list'>
                    {startupArtifacts?.map((startup) => (
                      <li key={startup.id}>
                        <div>
                          <strong>{startup.id}</strong>
                          <span>{startup.exact_core_version}</span>
                        </div>
                        <span>{t(`cores.startup.state.${startup.state}`, { defaultValue: startup.state })}</span>
                        {startup.state === 'pending' && artifact.verification_state === 'verified'
                          ? (
                              <Button
                                aria-label={t('cores.check')}
                                disabled={pending.has(`check:${startup.id}`)}
                                onClick={() => onCheckStartup(startup)}
                                variant='outline'
                              >
                                {t('cores.check', { defaultValue: 'Run startup check' })}
                              </Button>
                            )
                          : (
                              <small>
                                {startup.state === 'pending'
                                  ? t('cores.startup.blocked')
                                  : startup.checked_at
                                    ? formatTimestamp(startup.checked_at, locale)
                                    : ''}
                              </small>
                            )}
                        {acceptedStartupTasks[startup.id] === undefined
                          ? null
                          : (
                              <div role='status'>
                                <span>{t('cores.startup.taskAccepted', { defaultValue: 'Task accepted' })}</span>
                                <code>{acceptedStartupTasks[startup.id]}</code>
                                <Link className='text-link' to='/tasks'>{t('cores.openTasks', { defaultValue: 'Open tasks' })}</Link>
                              </div>
                            )}
                      </li>
                    ))}
                  </ul>
                </section>
              </div>
            )}
      </SheetContent>
    </Sheet>
  );
}
