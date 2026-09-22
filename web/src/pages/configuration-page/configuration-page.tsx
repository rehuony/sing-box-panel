import { useTranslation } from 'react-i18next';
import { lazy, Suspense, useEffect, useRef, useState } from 'react';

import { Button } from '@/components/ui/button';
import { useHashTab } from '@/hooks/use-hash-tab';
import { toast } from '@/components/ui/toast-manager';
import { useApiClient } from '@/api/api-client-context';
import { reviewedSchemaManifest } from '@/schemas/generated';
import { useControlPlane } from '@/stores/control-plane.store';
import { describeRequestError, ErrorNotice } from '@/components/error-notice';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { useOptionalSharedTelemetry } from '@/components/app-shell/telemetry-context';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';

import { DynamicGeneralEditor } from './dynamic-general-editor';
import { useConfigurationSchema } from './use-configuration-schema';
import { useCanonicalConfiguration } from './use-canonical-configuration';
import './configuration-page.css';

const AdvancedConfigurationEditor = lazy(() => import('./advanced-configuration-editor').then(module => ({ default: module.AdvancedConfigurationEditor })));

export function ConfigurationPage() {
  const { t } = useTranslation();
  const client = useApiClient();
  const controlPlane = useControlPlane();
  const telemetry = useOptionalSharedTelemetry();
  const canonical = useCanonicalConfiguration();
  const [selectedSchemaVersion, setSelectedSchemaVersion] = useState<string | null>(null);
  const runningVersion = controlPlane.context?.view.exactVersion;
  const installedVersion = runningVersion && /^\d+\.\d+\.\d+$/.test(runningVersion) ? runningVersion : '';
  const schemaVersion = selectedSchemaVersion
    ?? (installedVersion || Object.keys(reviewedSchemaManifest)[0] || '');
  const schemaVersions = [...new Set([
    ...Object.keys(reviewedSchemaManifest), installedVersion, schemaVersion,
  ])].filter(Boolean);
  const schema = useConfigurationSchema(schemaVersion);
  const [checking, setChecking] = useState(false);
  const [linkedInbound] = useState(() => new URLSearchParams(window.location.search).get('inbound'));
  const [selectedEditor, setSelectedEditor] = useHashTab('configuration-', ['visual', 'advanced'] as const, 'visual');
  const checkControllerRef = useRef<AbortController | null>(null);
  useEffect(() => () => checkControllerRef.current?.abort(), []);

  async function check() {
    if (canonical.dirty || canonical.editorError !== null || checking) return;
    const controller = new AbortController();
    checkControllerRef.current = controller;
    setChecking(true);
    try {
      const cores = await client.listCoreArtifacts(
        { exactVersion: schemaVersion, limit: 200 }, controller.signal,
      );
      const runtime = await client.getRuntimeStatus(controller.signal);
      const core = cores.items.find(item => item.id === runtime.running?.core_artifact_id) ?? cores.items[0];
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
  const visualUnavailable = invalid || schema.status === 'unavailable' || schema.status === 'error';
  const editor = visualUnavailable ? 'advanced' : selectedEditor;
  const loading = canonical.state.status === 'loading' || (fileReady && editor === 'visual' && schema.status === 'loading');
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
              {schema.status === 'unavailable'
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
                : <TabsTrigger disabled={!fileReady || schema.status !== 'ready' || invalid} value='visual'>{t('configuration.file.visual')}</TabsTrigger>}
              <TabsTrigger disabled={!fileReady} value='advanced'>{t('configuration.tab.advanced')}</TabsTrigger>
            </TabsList>
            <div className='configuration-schema-version'>
              <label htmlFor='configuration-schema-version'>{t('configuration.schema.version')}</label>
              <Select value={schemaVersion} onValueChange={value => {
                if (value) setSelectedSchemaVersion(value);
              }}>
                <SelectTrigger id='configuration-schema-version'><SelectValue /></SelectTrigger>
                <SelectContent>
                  {schemaVersions.map(version => <SelectItem key={version} value={version}>{version}</SelectItem>)}
                </SelectContent>
              </Select>
            </div>
          </div>
          {schema.status === 'ready' && schema.bundled && <p className='configuration-schema-notice' role='status'>{t('configuration.schema.bundled', { version: schemaVersion })}</p>}
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
                    {loading
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
          <Button variant='outline' disabled={locked || canonical.dirty || invalid || file?.revision === 0} onClick={() => void check()} title={canonical.dirty ? t('configuration.file.saveFirst') : undefined} type='button'>{checking ? t('configuration.file.checking') : t('configuration.file.check')}</Button>
          <Button disabled={locked || (!canonical.dirty && (file?.revision ?? 0) > 0)} onClick={() => void canonical.save()} type='button'>{canonical.saving ? t('configuration.saving') : t('configuration.file.save')}</Button>
        </footer>
      </section>
    </div>
  );
}
