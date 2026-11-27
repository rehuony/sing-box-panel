import { basicSetup } from 'codemirror';
import { json } from '@codemirror/lang-json';
import { yaml } from '@codemirror/lang-yaml';
import { EditorView } from '@codemirror/view';
import { useTranslation } from 'react-i18next';
import { useLayoutEffect, useRef } from 'react';
import { indentUnit } from '@codemirror/language';
import { Compartment, EditorState, Transaction } from '@codemirror/state';

import { chineseEditorPhrases, editorTheme } from './editor-theme';

/** The languages the editor highlights; plain text has line numbers alone. */
export type CodeEditorLanguage = 'json' | 'yaml' | 'plain';

const languages = { json: () => [json()], yaml: () => [yaml()], plain: () => [] };

/**
 * A code editor in the panel's editor theme: line numbers, history, search
 * and the language's highlighting. Its text follows value, which it reports
 * on every change.
 */
export function CodeEditor({ value, onChange, language, label, disabled = false }: {
  value: string;
  onChange: (value: string) => void;
  language: CodeEditorLanguage;
  /** The editor's accessible name. */
  label: string;
  disabled?: boolean;
}) {
  const { i18n } = useTranslation();
  const hostRef = useRef<HTMLDivElement>(null);
  const editorRef = useRef<EditorView | null>(null);
  const optionsRef = useRef(new Compartment());
  const latestRef = useRef({ value, onChange });
  useLayoutEffect(() => {
    latestRef.current = { value, onChange };
  });

  useLayoutEffect(() => {
    if (!hostRef.current) return;
    const view = new EditorView({
      parent: hostRef.current,
      doc: latestRef.current.value,
      extensions: [
        EditorView.cspNonce.of(document.querySelector<HTMLMetaElement>('meta[name="sing-box-panel-style-nonce"]')?.content ?? ''),
        basicSetup, indentUnit.of('  '), editorTheme,
        optionsRef.current.of([]),
        EditorView.updateListener.of(update => {
          if (update.docChanged) latestRef.current.onChange(update.state.doc.toString());
        }),
      ],
    });
    editorRef.current = view;
    return () => {
      editorRef.current = null;
      view.destroy();
    };
  }, []);

  useLayoutEffect(() => {
    editorRef.current?.dispatch({ effects: optionsRef.current.reconfigure([
      languages[language](),
      EditorState.readOnly.of(disabled),
      EditorView.editable.of(!disabled),
      EditorView.contentAttributes.of({ 'aria-label': label, 'aria-disabled': String(disabled) }),
      EditorState.phrases.of(i18n.language.startsWith('zh') ? chineseEditorPhrases : {}),
    ]) });
  }, [disabled, i18n.language, label, language]);

  // A value set from outside, such as the default restored, replaces the text without an undo step.
  useLayoutEffect(() => {
    const view = editorRef.current;
    if (view && view.state.doc.toString() !== value) {
      view.dispatch({
        changes: { from: 0, to: view.state.doc.length, insert: value },
        annotations: Transaction.addToHistory.of(false),
      });
    }
  }, [value]);

  return <div className='h-full min-h-0 min-w-0 overflow-hidden' ref={hostRef} />;
}
