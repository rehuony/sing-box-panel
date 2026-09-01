import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Braces, Clock3, FileCheck2, Rocket, Save, Settings2, Undo2 } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { ErrorNotice } from '@/components/error-notice';
import { useControlPlane } from '@/stores/control-plane.store';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';

import { RevisionHistory } from './revision-history';
import { StartupWorkflow } from './startup-workflow';
import { DynamicGeneralEditor } from './dynamic-general-editor';
import { useConfigurationSchema } from './use-configuration-schema';
import { ManagedCollectionsEditor } from './managed-collections-editor';
import { visibleStructuredConfiguration } from './structured-validation';
import { AdvancedConfigurationEditor } from './advanced-configuration-editor';
import { encodeCanonicalValue, useCanonicalConfiguration } from './use-canonical-configuration';
import './configuration-page.css';

function formatTimestamp(timestamp: string, locale: string): string {
  return new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'short' })
    .format(new Date(timestamp));
}

export function ConfigurationPage() {
  const { i18n, t } = useTranslation();
  const controlPlane = useControlPlane();
  const canonical = useCanonicalConfiguration();
  const [advancedDocumentValid, setAdvancedDocumentValid] = useState(true);
  const exactVersion = controlPlane.viewVersion;
  const schema = useConfigurationSchema(exactVersion);
  const schemaResolution = schema.status === 'ready' ? schema.resolution : null;
  const schemaValidator = useMemo(() => {
    if (schemaResolution === null) return null;
    try {
      return schemaResolution.createValidator(schemaResolution.schema);
    } catch {
      return null;
    }
  }, [schemaResolution]);
  const currentRevisionID = canonical.state.status === 'ready' ? canonical.state.snapshot.id : '';
  const structuredDocumentValid = useMemo(() => {
    if (schemaResolution === null || canonical.state.status !== 'ready') return true;
    if (schemaValidator === null) return false;
    try {
      const configuration = JSON.parse(encodeCanonicalValue(canonical.state.draft)) as unknown;
      const data = visibleStructuredConfiguration(schemaResolution.schema, configuration);
      const validation = schemaValidator.rawValidation<Record<string, unknown>>(
        schemaResolution.schema,
        data,
      );
      if (validation.errors !== undefined) throw new Error(JSON.stringify(validation.errors));
      return true;
    } catch {
      return false;
    }
  }, [canonical.state, schemaResolution, schemaValidator]);
  const documentValid = advancedDocumentValid && structuredDocumentValid;
  const structuredDisabled = canonical.saving || !advancedDocumentValid;
  const nonAdvancedMutationsDisabled = canonical.saving || !documentValid;

  return (
    <div className='configuration-page'>
      <header className='configuration-toolbar'>
        <div className='configuration-toolbar__title'>
          <h1>{t('configuration.title')}</h1>
          {canonical.state.status === 'ready'
            ? (
                <p>
                  {t('configuration.revision', {
                    sequence: new Intl.NumberFormat(i18n.language).format(canonical.state.snapshot.sequence),
                  })}
                  <span aria-hidden='true'> · </span>
                  {formatTimestamp(canonical.state.snapshot.created_at, i18n.language)}
                </p>
              )
            : null}
        </div>

        <div className='configuration-toolbar__actions'>
          <Button disabled={canonical.saving || canonical.state.status !== 'ready'} onClick={canonical.reset} type='button' variant='outline'>
            <Undo2 aria-hidden data-icon='inline-start' />
            {t('configuration.discard')}
          </Button>
          <Button
            aria-describedby={documentValid ? undefined : 'canonical-document-validation-status'}
            disabled={canonical.saving || canonical.state.status !== 'ready' || !documentValid}
            onClick={() => void canonical.save()}
            type='button'
          >
            <Save aria-hidden data-icon='inline-start' />
            {canonical.saving ? t('configuration.saving') : t('configuration.save')}
          </Button>
        </div>
      </header>

      {canonical.state.status === 'error'
        ? <ErrorNotice error={canonical.state.error} title={t('configuration.error.unavailable')} />
        : null}
      {canonical.saveError === null
        ? null
        : <ErrorNotice error={canonical.saveError} title={t('configuration.error.notSaved')} />}
      {canonical.message === '' ? null : <p className='configuration-save-status' role='status'>{canonical.message}</p>}
      {!structuredDocumentValid
        ? (
            <p className='configuration-save-status' id='canonical-document-validation-status' role='alert'>
              {t('configuration.schema.invalidDraft')}
            </p>
          )
        : null}

      {canonical.state.status === 'loading'
        ? (
            <div className='inline-loading configuration-loading' aria-busy='true'>{t('configuration.loading')}</div>
          )
        : null}

      {canonical.state.status === 'ready'
        ? (
            <Tabs
              className='configuration-tabs'
              defaultValue={schema.status === 'ready' ? 'general' : 'advanced'}
              key={`${exactVersion}:${schema.status === 'ready' ? 'structured' : 'json'}`}
            >
              <div className='configuration-tabs__rail'>
                <TabsList aria-label={t('configuration.sections')} variant='line'>
                  {schema.status === 'ready'
                    ? (
                        <>
                          <TabsTrigger value='general'>
                            <Settings2 aria-hidden />
                            {t('configuration.tab.general')}
                          </TabsTrigger>
                          <TabsTrigger value='managed'>
                            <FileCheck2 aria-hidden />
                            {t('configuration.tab.managed')}
                          </TabsTrigger>
                        </>
                      )
                    : null}
                  <TabsTrigger value='advanced'>
                    <Braces aria-hidden />
                    {t('configuration.tab.advanced')}
                  </TabsTrigger>
                  <TabsTrigger value='history'>
                    <Clock3 aria-hidden />
                    {t('configuration.tab.history')}
                  </TabsTrigger>
                  <TabsTrigger value='deploy'>
                    <Rocket aria-hidden />
                    {t('configuration.tab.deploy')}
                  </TabsTrigger>
                </TabsList>
              </div>

              {schema.status === 'loading'
                ? (
                    <div className='configuration-schema-state inline-loading' aria-busy='true'>
                      {t('configuration.schema.loading')}
                    </div>
                  )
                : null}
              {schema.status === 'error'
                ? (
                    <div className='configuration-schema-state'>
                      <ErrorNotice error={schema.error} title={t('configuration.schema.failClosed')} />
                      <p>{t('configuration.schema.jsonAvailable')}</p>
                    </div>
                  )
                : null}
              {schema.status === 'unavailable'
                ? (
                    <p className='configuration-schema-state' role='status'>
                      {t('configuration.schema.nativeUnavailable', { version: exactVersion })}
                    </p>
                  )
                : null}

              <TabsContent className='configuration-tabs__content' value='general'>
                {schema.status === 'ready'
                  ? (
                      <DynamicGeneralEditor
                        disabled={structuredDisabled}
                        draft={canonical.state.draft}
                        onChange={canonical.update}
                        resolution={schema.resolution}
                      />
                    )
                  : null}
              </TabsContent>
              <TabsContent className='configuration-tabs__content' value='managed'>
                {schema.status === 'ready'
                  ? (
                      <ManagedCollectionsEditor
                        disabled={structuredDisabled}
                        draft={canonical.state.draft}
                        onChange={canonical.update}
                        resolution={schema.resolution}
                      />
                    )
                  : null}
              </TabsContent>
              <TabsContent className='configuration-tabs__content' value='advanced'>
                <AdvancedConfigurationEditor
                  disabled={canonical.saving}
                  draft={canonical.state.draft}
                  onChange={canonical.update}
                  onValidityChange={setAdvancedDocumentValid}
                />
              </TabsContent>
              <TabsContent className='configuration-tabs__content' value='history'>
                <RevisionHistory
                  currentRevisionID={currentRevisionID}
                  loadingOlderRevisions={canonical.loadingOlderRevisions}
                  mutationsDisabled={nonAdvancedMutationsDisabled}
                  onLoadOlderRevisions={canonical.loadOlderRevisions}
                  onRestore={canonical.restore}
                  revisionError={canonical.revisionError}
                  revisions={canonical.revisions}
                />
              </TabsContent>
              <TabsContent className='configuration-tabs__content' value='deploy'>
                <StartupWorkflow
                  exactVersion={exactVersion}
                  key={exactVersion}
                  mutationsDisabled={nonAdvancedMutationsDisabled}
                />
              </TabsContent>
            </Tabs>
          )
        : null}
    </div>
  );
}
