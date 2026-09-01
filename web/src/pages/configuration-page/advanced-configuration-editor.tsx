import { useTranslation } from 'react-i18next';
import { useEffect, useRef, useState } from 'react';
import { Braces, CheckCircle2, TriangleAlert } from 'lucide-react';

import type { CanonicalDraft } from './use-canonical-configuration';

import {
  encodeCanonicalDraft,
  localizeCanonicalDraftError,
  parseCanonicalDraft,
} from './use-canonical-configuration';

export interface AdvancedConfigurationEditorProps {
  disabled: boolean;
  draft: CanonicalDraft;
  onValidityChange: (valid: boolean) => void;
  onChange: (change: (draft: CanonicalDraft) => CanonicalDraft) => void;
}

interface EditorState {
  documentJSON: string;
  validationError: string | null;
}

function editorText(draft: CanonicalDraft): string {
  return encodeCanonicalDraft(draft, 2);
}

export function AdvancedConfigurationEditor({
  disabled,
  draft,
  onChange,
  onValidityChange,
}: AdvancedConfigurationEditorProps) {
  const { t } = useTranslation();
  const [editorState, setEditorState] = useState<EditorState>(() => ({
    documentJSON: editorText(draft),
    validationError: null,
  }));
  const applyingEditorChangeRef = useRef(false);

  useEffect(() => {
    if (applyingEditorChangeRef.current) {
      applyingEditorChangeRef.current = false;
      return;
    }
    // External structured edits, restores and resets replace the local JSON buffer.
    // eslint-disable-next-line react/set-state-in-effect
    setEditorState({ documentJSON: editorText(draft), validationError: null });
    onValidityChange(true);
  }, [draft, onValidityChange]);

  function updateDocumentJSON(nextDocumentJSON: string) {
    try {
      const nextDraft = parseCanonicalDraft(nextDocumentJSON);
      setEditorState({ documentJSON: nextDocumentJSON, validationError: null });
      onValidityChange(true);
      applyingEditorChangeRef.current = true;
      onChange(() => nextDraft);
    } catch (error) {
      const localizedError = localizeCanonicalDraftError(error, t);
      setEditorState({
        documentJSON: nextDocumentJSON,
        validationError: localizedError instanceof Error
          ? localizedError.message
          : t('configuration.advanced.invalidJSON'),
      });
      onValidityChange(false);
    }
  }

  const { documentJSON, validationError } = editorState;
  const valid = validationError === null;

  return (
    <section className='editor-panel advanced-configuration-editor' aria-labelledby='advanced-configuration-title'>
      <header className='advanced-configuration-editor__summary'>
        <span className='advanced-configuration-editor__heading'>
          <span className='advanced-configuration-editor__icon'><Braces aria-hidden='true' /></span>
          <span aria-level={2} className='advanced-configuration-editor__title' id='advanced-configuration-title' role='heading'>
            {t('configuration.advanced.title')}
          </span>
        </span>
        <span className={`advanced-configuration-editor__state${valid ? '' : ' advanced-configuration-editor__state--error'}`}>
          {valid ? t('configuration.advanced.valid') : t('configuration.advanced.attention')}
        </span>
      </header>

      <div className='advanced-configuration-editor__body'>
        <p className='source-note'>{t('configuration.advanced.description')}</p>
        <div className='field-group'>
          <label htmlFor='configuration-json'>{t('configuration.advanced.document')}</label>
          <textarea
            aria-describedby='configuration-json-status'
            aria-invalid={!valid}
            autoCapitalize='off'
            autoCorrect='off'
            className='advanced-configuration-editor__textarea'
            disabled={disabled}
            id='configuration-json'
            onChange={(event) => updateDocumentJSON(event.target.value)}
            rows={18}
            spellCheck={false}
            value={documentJSON}
          />
        </div>
        <div
          aria-live='polite'
          className={`advanced-configuration-editor__validation${valid ? '' : ' advanced-configuration-editor__validation--error'}`}
          id='configuration-json-status'
          role={valid ? 'status' : 'alert'}
        >
          {valid ? <CheckCircle2 aria-hidden='true' /> : <TriangleAlert aria-hidden='true' />}
          <span>{valid ? t('configuration.advanced.synchronized') : validationError}</span>
        </div>
      </div>
    </section>
  );
}
