import { useTranslation } from 'react-i18next';
import { useEffect, useRef, useState } from 'react';

import { Button } from '@/components/ui/button';
import { toast } from '@/components/ui/toast-manager';
import { useApiClient } from '@/api/api-client-context';
import { ErrorNotice } from '@/components/error-notice';
import { useControlPlane } from '@/stores/control-plane.store';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { useOptionalSharedTelemetry } from '@/components/app-shell/telemetry-context';

import { DynamicGeneralEditor } from './dynamic-general-editor';
import { useConfigurationSchema } from './use-configuration-schema';
import { useCanonicalConfiguration } from './use-canonical-configuration';
import { AdvancedConfigurationEditor } from './advanced-configuration-editor';
import './configuration-page.css';

export function ConfigurationPage() {
  const { t } = useTranslation();
  const client = useApiClient();
  const controlPlane = useControlPlane();
  const telemetry = useOptionalSharedTelemetry();
  const canonical = useCanonicalConfiguration();
  const schema = useConfigurationSchema(controlPlane.viewVersion);
  const [checking, setChecking] = useState(false);
  const [linkedInbound] = useState(() => new URLSearchParams(window.location.search).get('inbound'));
  const [selectedEditor, setSelectedEditor] = useState<string | null>(null);
  const checkControllerRef = useRef<AbortController | null>(null);
  useEffect(() => () => checkControllerRef.current?.abort(), []);

  async function check() {
    if (canonical.dirty || canonical.editorError !== null || checking) return;
    const controller = new AbortController();
    checkControllerRef.current = controller;
    setChecking(true);
    try {
      const cores = await client.listCoreArtifacts({ exactVersion: controlPlane.viewVersion, verificationState: 'verified', limit: 200 }, controller.signal);
      const runtime = await client.getRuntimeStatus(controller.signal);
      const core = cores.items.find(item => item.id === runtime.running?.core_artifact_id) ?? cores.items[0];
      if (core === undefined) throw new Error(t('configuration.file.noCore'));
      const result = await client.compileConfiguration({ coreArtifactID: core.id }, controller.signal);
      let task = result.task;
      const deadline = Date.now() + 60_000;
      while (task.status === 'queued' || task.status === 'running') {
        if (Date.now() >= deadline) throw new Error(t('configuration.file.checkPending'));
        await new Promise<void>((resolve, reject) => {
          let timer = 0;
          const abort = () => {
            window.clearTimeout(timer);
            reject(new DOMException('Aborted', 'AbortError'));
          };
          timer = window.setTimeout(() => {
            controller.signal.removeEventListener('abort', abort);
            resolve();
          }, 750);
          if (controller.signal.aborted) abort();
          else controller.signal.addEventListener('abort', abort, { once: true });
        });
        task = await client.getTask(task.id, controller.signal);
      }
      if (task.status !== 'succeeded') throw new Error(t('configuration.file.checkFailed'));
      if (!controller.signal.aborted) toast.add({ title: t('configuration.file.checked'), type: 'success' });
    } catch (error) {
      if (!controller.signal.aborted) toast.add({ title: error instanceof Error ? error.message : t('configuration.file.checkFailed'), type: 'error' });
    } finally {
      if (!controller.signal.aborted) setChecking(false);
    }
  }

  const invalid = canonical.editorError !== null;
  const visualAvailable = schema.status === 'ready' && !invalid;
  const editor = visualAvailable ? selectedEditor ?? 'visual' : 'advanced';
  const locked = canonical.saving || checking;
  const runtime = telemetry?.runtimeStatus;
  const file = canonical.state.file;
  const fileStatus = canonical.dirty
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
  return (
    <div className='configuration-page'>
      <h1>{t('configuration.title')}</h1>
      {canonical.state.status === 'error' ? <ErrorNotice error={canonical.state.error} title={t('configuration.error.unavailable')} /> : null}
      {canonical.state.status === 'loading' ? <div className='inline-loading' aria-busy='true'>{t('configuration.loading')}</div> : null}
      {canonical.state.status === 'ready'
        ? (
            <section className='configuration-workspace'>
              <Tabs className='configuration-tabs' value={editor} onValueChange={setSelectedEditor}>
                <div className='configuration-tabs__rail'>
                  <TabsList aria-label={t('configuration.sections')}>
                    <TabsTrigger disabled={schema.status !== 'ready' || invalid} value='visual'>{t('configuration.file.visual')}</TabsTrigger>
                    <TabsTrigger value='advanced'>{t('configuration.tab.advanced')}</TabsTrigger>
                  </TabsList>
                  <span className='configuration-filename'>config.json</span>
                </div>
                <TabsContent className='configuration-tabs__content' value='visual'>
                  {schema.status === 'ready' && canonical.draft !== null
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
                  <AdvancedConfigurationEditor
                    disabled={locked} text={canonical.state.content}
                    error={canonical.editorError} onChange={canonical.updateText}
                  />
                </TabsContent>
              </Tabs>
              <footer className='configuration-footer'>
                <span className='configuration-file-state' role='status'>{fileStatus}</span>
                <Button disabled={locked || canonical.dirty || invalid || canonical.state.file.revision === 0} onClick={() => void check()} title={canonical.dirty ? t('configuration.file.saveFirst') : undefined} type='button' variant='ghost'>{checking ? t('configuration.file.checking') : t('configuration.file.check')}</Button>
                <Button disabled={locked || (!canonical.dirty && canonical.state.file.revision > 0)} onClick={() => void canonical.save()} type='button'>{canonical.saving ? t('configuration.saving') : t('configuration.file.save')}</Button>
              </footer>
            </section>
          )
        : null}
    </div>
  );
}
