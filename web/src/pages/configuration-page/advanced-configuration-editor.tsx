import { useTranslation } from 'react-i18next';

export interface AdvancedConfigurationEditorProps {
  text: string;
  error: unknown;
  disabled: boolean;
  onChange: (text: string) => void;
}

export function AdvancedConfigurationEditor({ disabled, text, error, onChange }: AdvancedConfigurationEditorProps) {
  const { t } = useTranslation();
  return (
    <div className='advanced-configuration-editor'>
      <label className='sr-only' htmlFor='configuration-json'>{t('configuration.advanced.document')}</label>
      <textarea
        aria-describedby={error === null ? undefined : 'configuration-json-error'}
        aria-invalid={error !== null}
        autoCapitalize='off'
        autoCorrect='off'
        className='advanced-configuration-editor__textarea'
        disabled={disabled}
        id='configuration-json'
        onChange={event => onChange(event.target.value)}
        spellCheck={false}
        value={text}
      />
      {error === null ? null : <p className='configuration-json-error' id='configuration-json-error' role='alert'>{error instanceof Error ? error.message : t('configuration.advanced.invalidJSON')}</p>}
    </div>
  );
}
