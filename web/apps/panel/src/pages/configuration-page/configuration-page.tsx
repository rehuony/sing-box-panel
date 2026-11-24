import { Package } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@shared/components/ui/button';
import { toast } from '@shared/components/ui/toast-manager';
import { lazy, Suspense, useEffect, useRef, useState } from 'react';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@shared/components/ui/tabs';
import { Tooltip, TooltipContent, TooltipTrigger } from '@shared/components/ui/tooltip';
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@shared/components/ui/select';

import { useHashTab } from '@/hooks/use-hash-tab';
import { messageLine } from '@/utils/message-line';
import { useApiClient } from '@/api/api-client-context';
import { LoadingState } from '@/components/loading-state';
import { describeRequestError } from '@/components/error-notice';
import { RegionError, RegionState } from '@/components/region-state';
import { CardToolbar, StripDivider } from '@/components/title-strip';
import { useOptionalSharedTelemetry } from '@/components/node-workspace';
import { useConfigurationSessionStore } from '@/stores/configuration-session.store';

import { DynamicGeneralEditor } from './dynamic-general-editor';
import { useConfigurationSchema } from './use-configuration-schema';
import { ConfigurationCopyDialog } from './configuration-copy-dialog';
import { useCanonicalConfiguration } from './use-canonical-configuration';
import { AppliedConfigurationsDialog } from './applied-configurations-dialog';
import { useInstalledConfigurationVersions } from './use-installed-configuration-versions';

const AdvancedConfigurationEditor = lazy(() => import('./advanced-configuration-editor').then(module => ({ default: module.AdvancedConfigurationEditor })));

export function ConfigurationPage() {
  const { t } = useTranslation();
  const client = useApiClient();
  const runtime = useOptionalSharedTelemetry(s => s.runtimeStatus);
  const canonical = useCanonicalConfiguration();
  const installed = useInstalledConfigurationVersions();
  const selectedSchemaVersion = useConfigurationSessionStore(state => state.selectedVersion);
  const reconcileVersion = useConfigurationSessionStore(state => state.reconcileVersion);
  const setSelectedSchemaVersion = useConfigurationSessionStore(state => state.setSelectedVersion);
  const enabledVersion = installed.status === 'ready'
    ? installed.runtime?.enabled_core?.exact_core_version
    : undefined;
  const schemaVersion = installed.status === 'ready'
    ? selectedSchemaVersion !== null && installed.versions.includes(selectedSchemaVersion)
      ? selectedSchemaVersion
      : enabledVersion !== undefined && installed.versions.includes(enabledVersion)
        ? enabledVersion
        : installed.versions[0] ?? ''
    : '';
  const schemaArtifact = installed.status === 'ready'
    ? installed.artifactsByVersion.get(schemaVersion) ?? null
    : null;
  const schema = useConfigurationSchema(schemaArtifact);
  const [checking, setChecking] = useState(false);
  const [linkedInbound] = useState(() => new URLSearchParams(window.location.search).get('inbound'));
  const [selectedEditor, setSelectedEditor] = useHashTab('configuration-', ['visual', 'advanced'] as const, 'visual');
  const checkControllerRef = useRef<AbortController | null>(null);
  useEffect(() => () => checkControllerRef.current?.abort(), []);
  useEffect(() => {
    if (installed.status === 'ready') reconcileVersion(installed.versions, enabledVersion);
  }, [enabledVersion, installed.status, installed.versions, reconcileVersion]);

  async function check() {
    if (
      canonical.dirty
      || canonical.editorError !== null
      || checking
      || installed.status !== 'ready'
      || schemaVersion === ''
    ) {
      return;
    }
    const controller = new AbortController();
    checkControllerRef.current = controller;
    setChecking(true);
    try {
      const runtime = await client.getRuntimeStatus(controller.signal);
      const candidates = installed.artifacts.filter(artifact => artifact.exact_version === schemaVersion);
      const enabledID = runtime.enabled_core?.exact_core_version === schemaVersion
        ? runtime.enabled_core.core_artifact_id
        : undefined;
      const core = candidates.find(item => item.id === enabledID) ?? candidates[0];
      if (core === undefined) throw new Error(t('configuration.file.noCore'));
      const result = await client.compileConfiguration({ coreArtifactID: core.id }, controller.signal);
      if (result.artifact.state !== 'ready') throw new Error(t('configuration.file.checkFailed'));
      if (!controller.signal.aborted) toast.add({ title: t('configuration.file.checked'), type: 'success' });
    } catch (error) {
      if (!controller.signal.aborted) toast.add({ title: error instanceof Error ? error.message : t('configuration.file.checkFailed'), type: 'error' });
    } finally {
      if (!controller.signal.aborted) setChecking(false);
    }
  }

  const fileReady = canonical.state.status === 'ready';
  const invalid = fileReady && canonical.editorError !== null;
  const noInstalledVersions = installed.status === 'ready' && installed.versions.length === 0;
  const visualUnavailable = invalid || (
    installed.status === 'ready'
    && !noInstalledVersions
    && (schema.status === 'unavailable' || schema.status === 'error')
  );
  const editor = visualUnavailable ? 'advanced' : selectedEditor;
  const loading = canonical.state.status === 'loading' || (
    fileReady
    && editor === 'visual'
    && (installed.status === 'loading' || (!noInstalledVersions && schema.status === 'loading'))
  );
  const locked = !fileReady || canonical.saving || checking;
  const file = canonical.state.file;
  const fileStatus = !fileReady
    ? ''
    : canonical.dirty
      ? t('configuration.unsaved')
      : invalid
        ? t('configuration.file.savedInvalid')
        : runtime?.observation_state === 'running' && runtime.loaded_canonical_revision_id !== undefined
          ? t(runtime.loaded_canonical_revision_id === file?.canonical_revision_id
              ? 'configuration.file.loaded'
              : 'configuration.file.restartRequired')
          : runtime?.observation_state === 'stopped'
            ? t('configuration.file.nextStart')
            : t('configuration.file.saved');
  const loadingEditor = (
    <LoadingState className='h-full' label={t('configuration.loading')} fullScreen={false} />
  );
  return (
    <div className='@container flex min-h-0 min-w-0 flex-1 flex-col gap-4'>
      <h1 className='sr-only'>{t('configuration.title')}</h1>
      <section className='flex min-h-0 flex-1 flex-col overflow-hidden rounded-[var(--radius-card)] border bg-[var(--color-surface-translucent)] [--configuration-inset:24px] @max-[40rem]:[--configuration-inset:16px]' aria-label={t('configuration.title')} aria-busy={loading}>
        <Tabs className='min-h-0 flex-1 gap-0' value={editor} onValueChange={setSelectedEditor}>
          <CardToolbar className='px-(--configuration-inset)'>
            <TabsList aria-label={t('configuration.sections')} className='flex-none' variant='pill'>
              {installed.status === 'ready' && !noInstalledVersions && schema.status === 'unavailable'
                ? (
                    <Tooltip>
                      <TooltipTrigger
                        render={<span className='inline-flex rounded-[var(--radius-control)]' tabIndex={0} />}
                      >
                        <TabsTrigger disabled value='visual'>{t('configuration.file.visual')}</TabsTrigger>
                      </TooltipTrigger>
                      <TooltipContent>{t('configuration.schema.unsupported', { version: schemaVersion })}</TooltipContent>
                    </Tooltip>
                  )
                : (
                    <TabsTrigger
                      disabled={
                        !fileReady
                        || invalid
                        || installed.status !== 'ready'
                        || (!noInstalledVersions && schema.status !== 'ready')
                      }
                      value='visual'
                    >
                      {t('configuration.file.visual')}
                    </TabsTrigger>
                  )}
              <TabsTrigger disabled={!fileReady} value='advanced'>{t('configuration.tab.advanced')}</TabsTrigger>
            </TabsList>
            <div className='ms-auto flex max-w-full min-w-0 items-center gap-1'>
              <AppliedConfigurationsDialog />
              <ConfigurationCopyDialog
                disabled={locked}
                draft={canonical.state.content}
                onReplace={canonical.updateText}
              />
              <StripDivider />
              <Select
                disabled={installed.status !== 'ready' || noInstalledVersions}
                value={schemaVersion || null}
                onValueChange={value => setSelectedSchemaVersion(value)}
              >
                <SelectTrigger className='w-auto max-w-full gap-2 *:data-[slot=select-value]:min-w-14 *:data-[slot=select-value]:max-w-40 *:data-[slot=select-value]:flex-none *:data-[slot=select-value]:font-[family-name:var(--font-data)]' id='configuration-schema-version' aria-labelledby='configuration-schema-version-label' variant='flat'>
                  <span className='min-w-0 truncate text-[var(--color-text-faint)]' id='configuration-schema-version-label'>{t('configuration.schema.version')}</span>
                  <SelectValue placeholder={t('configuration.schema.noInstalledOption')} />
                </SelectTrigger>
                <SelectContent align='end'>
                  <SelectGroup>
                    {installed.versions.map(version => (
                      <SelectItem key={version} value={version}>{version}</SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
            </div>
          </CardToolbar>
          {schema.status === 'error' && (
            <div className='flex items-center gap-3 border-b px-(--configuration-inset) py-2' role='alert'>
              <p className='m-0 min-w-0 flex-1 text-xs text-[var(--color-text-secondary)] wrap-anywhere'>
                {t('configuration.schema.failClosed')}
                :
                {' '}
                {describeRequestError(schema.error)}
              </p>
              <Button onClick={schema.retry} size='sm' type='button' variant='outline'>{t('shell.error.retry')}</Button>
            </div>
          )}
          {canonical.state.status === 'error'
            ? <div className='min-h-0 flex-1 overflow-hidden px-(--configuration-inset) pt-2'><RegionError className='h-full' error={canonical.state.error} reload={canonical.reload} title={t('configuration.error.unavailable')} /></div>
            : (
                <>
                  <TabsContent className='min-h-0 flex-1 overflow-hidden px-(--configuration-inset) pt-2 has-[>[data-configuration-general]]:pe-0 has-[>[data-configuration-general]]:pt-0' value='visual'>
                    {installed.status === 'error'
                      ? <RegionError className='h-full' error={installed.error} reload={installed.retry} title={t('configuration.error.versionsUnavailable')} />
                      : noInstalledVersions
                        ? <RegionState className='h-full' icon={Package} title={t('configuration.schema.noInstalledTitle')} />
                        : loading
                          ? loadingEditor
                          : schema.status === 'ready' && canonical.draft !== null
                            ? (
                                <DynamicGeneralEditor
                                  disabled={locked} draft={canonical.draft}
                                  linkedInbound={linkedInbound ?? undefined}
                                  onChange={canonical.update} resolution={schema.resolution}
                                />
                              )
                            : null}
                  </TabsContent>
                  <TabsContent className='min-h-0 flex-1 overflow-hidden px-(--configuration-inset) pt-2' value='advanced'>
                    {editor === 'advanced' && (fileReady
                      ? (
                          <Suspense fallback={loadingEditor}>
                            <AdvancedConfigurationEditor
                              disabled={locked} text={canonical.state.content}
                              error={canonical.editorError} onChange={canonical.updateText}
                            />
                          </Suspense>
                        )
                      : loadingEditor)}
                  </TabsContent>
                </>
              )}
        </Tabs>
        <footer className='flex flex-none flex-nowrap items-center gap-x-3 gap-y-2 border-t px-(--configuration-inset) py-2 *:data-[slot=button]:shrink-0 @max-[40rem]:flex-wrap @max-[40rem]:gap-1.5 @max-[40rem]:*:data-[slot=button]:flex-auto'>
          <span className='mr-auto min-w-0 flex-1 truncate text-sm text-muted-foreground @max-[40rem]:basis-full @max-[40rem]:text-xs' role='status' title={messageLine(fileStatus)}>{messageLine(fileStatus)}</span>
          <Button variant='outline' disabled={locked || canonical.dirty || invalid || file?.revision === 0 || schemaVersion === ''} onClick={() => void check()} title={canonical.dirty ? t('configuration.file.saveFirst') : undefined} type='button'>{checking ? t('configuration.file.checking') : t('configuration.file.check')}</Button>
          <Button disabled={locked || (!canonical.dirty && (file?.revision ?? 0) > 0)} onClick={() => void canonical.save()} type='button'>{canonical.saving ? t('configuration.saving') : t('configuration.file.save')}</Button>
        </footer>
      </section>
    </div>
  );
}
