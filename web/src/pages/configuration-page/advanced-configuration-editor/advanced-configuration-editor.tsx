import { basicSetup } from 'codemirror';
import { createPortal } from 'react-dom';
import { json } from '@codemirror/lang-json';
import { useTranslation } from 'react-i18next';
import { isolateHistory } from '@codemirror/commands';
import { EditorView, keymap } from '@codemirror/view';
import { useLayoutEffect, useRef, useState } from 'react';
import { openSearchPanel, search } from '@codemirror/search';
import { foldAll, indentUnit, unfoldAll } from '@codemirror/language';
import { Compartment, EditorState, Transaction } from '@codemirror/state';
import { Braces, FoldVertical, Search, UnfoldVertical } from 'lucide-react';

import { Button } from '@/components/ui/button';

import { SearchPanelHost } from './search-panel-host';
import { EditorSearchPanel } from './editor-search-panel';
import { chineseEditorPhrases, editorTheme } from './editor-theme';
import { encodeCanonicalDraft, parseCanonicalDraft } from '../use-canonical-configuration';
import './advanced-configuration-editor.css';

export interface AdvancedConfigurationEditorProps {
  text: string;
  error: unknown;
  disabled: boolean;
  onChange: (text: string) => void;
}

function formatDocument(view: EditorView): boolean {
  if (view.state.readOnly) return false;
  try {
    const text = view.state.doc.toString();
    const formatted = encodeCanonicalDraft(parseCanonicalDraft(text), 2);
    if (formatted !== text) {
      view.dispatch({
        changes: { from: 0, to: view.state.doc.length, insert: formatted },
        annotations: [Transaction.userEvent.of('input.format'), isolateHistory.of('full')],
      });
    }
    return true;
  } catch {
    // Keep incomplete input intact; the canonical editor displays its parse error.
    return false;
  }
}

export function AdvancedConfigurationEditor({ disabled, text, error, onChange }: AdvancedConfigurationEditorProps) {
  const { t, i18n } = useTranslation();
  const hostRef = useRef<HTMLDivElement>(null);
  const [searchPanel, setSearchPanel] = useState<SearchPanelHost | null>(null);
  const editorRef = useRef<EditorView | null>(null);
  const optionsRef = useRef(new Compartment());
  const latestRef = useRef({ text, onChange });
  useLayoutEffect(() => {
    latestRef.current = { text, onChange };
  });

  useLayoutEffect(() => {
    if (!hostRef.current) return;
    let disposed = false;
    const view = new EditorView({
      parent: hostRef.current,
      doc: latestRef.current.text,
      extensions: [
        keymap.of([{ key: 'Mod-Shift-f', run: formatDocument }]),
        search({
          top: true, literal: true,
          createPanel: view => new SearchPanelHost(view, setSearchPanel, () => {
            if (!disposed) setSearchPanel(null);
          }),
        }),
        basicSetup, json(), indentUnit.of('  '), editorTheme,
        optionsRef.current.of([]),
        EditorView.updateListener.of(update => {
          if (update.docChanged) latestRef.current.onChange(update.state.doc.toString());
        }),
      ],
    });
    editorRef.current = view;
    return () => {
      disposed = true;
      editorRef.current = null;
      view.destroy();
    };
  }, []);

  useLayoutEffect(() => {
    const view = editorRef.current;
    if (!view) return;
    view.dispatch({ effects: optionsRef.current.reconfigure([
      EditorState.readOnly.of(disabled),
      EditorView.editable.of(!disabled),
      EditorView.contentAttributes.of({
        'aria-label': t('configuration.advanced.document'),
        'aria-invalid': String(error !== null),
        'aria-disabled': String(disabled),
        ...(error !== null ? { 'aria-describedby': 'configuration-json-error' } : {}),
      }),
      EditorState.phrases.of(i18n.language.startsWith('zh') ? chineseEditorPhrases : {}),
    ]) });
  }, [disabled, error, i18n.language, t]);

  useLayoutEffect(() => {
    const view = editorRef.current;
    if (view && view.state.doc.toString() !== text) {
      view.dispatch({
        changes: { from: 0, to: view.state.doc.length, insert: text },
        annotations: Transaction.addToHistory.of(false),
      });
    }
  }, [text]);

  function run(command: (view: EditorView) => boolean) {
    if (editorRef.current) {
      editorRef.current.focus();
      command(editorRef.current);
    }
  }

  return (
    <div className='advanced-configuration-editor'>
      <div className='advanced-configuration-editor__tools'>
        <Button aria-label={t('configuration.advanced.search')} title={t('configuration.advanced.search')} onClick={() => run(openSearchPanel)} size='icon-sm' type='button' variant='ghost'><Search aria-hidden='true' /></Button>
        <Button aria-label={t('configuration.advanced.format')} title={t('configuration.advanced.format')} disabled={disabled || error !== null} onClick={() => run(formatDocument)} size='icon-sm' type='button' variant='ghost'>
          <Braces aria-hidden='true' />
        </Button>
        <Button aria-label={t('configuration.advanced.fold')} title={t('configuration.advanced.fold')} onClick={() => run(foldAll)} size='icon-sm' type='button' variant='ghost'><FoldVertical aria-hidden='true' /></Button>
        <Button aria-label={t('configuration.advanced.unfold')} title={t('configuration.advanced.unfold')} onClick={() => run(unfoldAll)} size='icon-sm' type='button' variant='ghost'><UnfoldVertical aria-hidden='true' /></Button>
      </div>
      <div className='advanced-configuration-editor__code' ref={hostRef} />
      {searchPanel && createPortal(<EditorSearchPanel host={searchPanel} />, searchPanel.dom)}
      {error === null ? null : <p className='configuration-json-error' id='configuration-json-error' role='alert'>{error instanceof Error ? error.message : t('configuration.advanced.invalidJSON')}</p>}
    </div>
  );
}
