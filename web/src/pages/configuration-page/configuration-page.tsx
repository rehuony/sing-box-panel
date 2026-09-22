import { Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { lazy, Suspense, useEffect, useRef, useState } from 'react';

import { Button } from '@/components/ui/button';
import { useHashTab } from '@/hooks/use-hash-tab';
import { toast } from '@/components/ui/toast-manager';
import { useApiClient } from '@/api/api-client-context';
import { buttonVariants } from '@/components/ui/button-variants';
import { describeRequestError, ErrorNotice } from '@/components/error-notice';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { useConfigurationSessionStore } from '@/stores/configuration-session.store';
import { useOptionalSharedTelemetry } from '@/components/app-shell/telemetry-context';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';

import { DynamicGeneralEditor } from './dynamic-general-editor';
import { useConfigurationSchema } from './use-configuration-schema';
import { useCanonicalConfiguration } from './use-canonical-configuration';
import { useInstalledConfigurationVersions } from './use-installed-configuration-versions';
import './configuration-page.css';

const AdvancedConfigurationEditor = lazy(() => import('./advanced-configuration-editor').then(module => ({ default: module.AdvancedConfigurationEditor })));

export function ConfigurationPage() {
  const { t } = useTranslation();
  const client = useApiClient();
  const telemetry = useOptionalSharedTelemetry();
  const canonical = useCanonicalConfiguration();
  const installed = useInstalledConfigurationVersions();
  const selectedSchemaVersion = useConfigurationSessionStore(state => state.selectedVersion);
  const reconcileVersion = useConfigurationSessionStore(state => state.reconcileVersion);
  const setSelectedSchemaVersion = useConfigurationSessionStore(state => state.setSelectedVersion);
  const enabledVersion = installed.status === 'ready'
    ? installed.runtime.enabled_core?.exact_core_version
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
  const runtime = telemetry?.runtimeStatus;
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
    <div className='configuration-editor-loading' role='status'>
      {t('configuration.loading')}
    </div>
  );
  return (
    <div className='configuration-page panel-page'>
      <h1 className='sr-only'>{t('configuration.title')}</h1>
      <section className='configuration-workspace' aria-label={t('configuration.title')} aria-busy={loading}>
        <Tabs className='configuration-tabs' value={editor} onValueChange={setSelectedEditor}>
          <div className='configuration-tabs__rail'>
            <TabsList aria-label={t('configuration.sections')}>
              {installed.status === 'ready' && !noInstalledVersions && schema.status === 'unavailable'
                ? (
                    <Tooltip>
                      <TooltipTrigger
                        render={<span className='configuration-disabled-tab' tabIndex={0} />}
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
            <div className='configuration-schema-version'>
              <label htmlFor='configuration-schema-version'>{t('configuration.schema.version')}</label>
              <Select
                disabled={installed.status !== 'ready' || noInstalledVersions}
                value={schemaVersion || null}
                onValueChange={value => setSelectedSchemaVersion(value)}
              >
                <SelectTrigger id='configuration-schema-version'>
                  <SelectValue placeholder={t('configuration.schema.noInstalledOption')} />
                </SelectTrigger>
                <SelectContent>
                  {installed.versions.map(version => <SelectItem key={version} value={version}>{version}</SelectItem>)}
                </SelectContent>
              </Select>
            </div>
          </div>
          {schema.status === 'error' && (
            <p className='configuration-schema-notice' role='alert'>
              {t('configuration.schema.failClosed')}
              :
              {' '}
              {describeRequestError(schema.error)}
            </p>
          )}
          {canonical.state.status === 'error'
            ? <div className='configuration-tabs__content'><ErrorNotice error={canonical.state.error} title={t('configuration.error.unavailable')} /></div>
            : (
                <>
                  <TabsContent className='configuration-tabs__content' value='visual'>
                    {installed.status === 'error'
                      ? <ErrorNotice error={installed.error} title={t('configuration.error.versionsUnavailable')} />
                      : noInstalledVersions
                        ? (
                            <div className='configuration-empty-versions' role='status'>
                              <h2>{t('configuration.schema.noInstalledTitle')}</h2>
                              <p>{t('configuration.schema.noInstalledDescription')}</p>
                              <Link className={buttonVariants()} data-slot='button' data-size='default' data-variant='default' to='/cores#cores-catalog'>
                                {t('configuration.schema.manageVersions')}
                              </Link>
                            </div>
                          )
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
                  <TabsContent className='configuration-tabs__content' value='advanced'>
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
        <footer className='configuration-footer'>
          <span className='configuration-file-state' role='status' title={fileStatus}>{fileStatus}</span>
          <Button variant='outline' disabled={locked || canonical.dirty || invalid || file?.revision === 0 || schemaVersion === ''} onClick={() => void check()} title={canonical.dirty ? t('configuration.file.saveFirst') : undefined} type='button'>{checking ? t('configuration.file.checking') : t('configuration.file.check')}</Button>
          <Button disabled={locked || (!canonical.dirty && (file?.revision ?? 0) > 0)} onClick={() => void canonical.save()} type='button'>{canonical.saving ? t('configuration.saving') : t('configuration.file.save')}</Button>
        </footer>
      </section>
    </div>
  );
}
