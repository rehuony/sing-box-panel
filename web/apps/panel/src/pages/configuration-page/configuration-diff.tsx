import { json } from '@codemirror/lang-json';
import { EditorState } from '@codemirror/state';
import { useLayoutEffect, useRef } from 'react';
import { unifiedMergeView } from '@codemirror/merge';
import { EditorView, lineNumbers } from '@codemirror/view';

import { editorTheme } from './advanced-configuration-editor/editor-theme';

/** Removed and added lines in the state colours, so a difference reads the same in both themes. */
const diffTheme = EditorView.theme({
  '.cm-changedLine': { backgroundColor: 'var(--color-success-soft)' },
  '.cm-changedText': { backgroundColor: 'color-mix(in srgb, var(--color-success) 24%, transparent)' },
  '.cm-deletedChunk': { backgroundColor: 'var(--color-danger-soft)', paddingLeft: '8px' },
  '.cm-deletedChunk del': { backgroundColor: 'color-mix(in srgb, var(--color-danger) 24%, transparent)', textDecoration: 'none' },
  '.cm-insertedLine': { backgroundColor: 'var(--color-success-soft)' },
});

/**
 * What replacing `original` with `next` changes, as one read-only document:
 * the next text with the removed lines shown where they were.
 */
export function ConfigurationDiff({ next, original }: { next: string; original: string }) {
  const hostRef = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => {
    if (!hostRef.current) return;
    const view = new EditorView({
      parent: hostRef.current,
      state: EditorState.create({
        doc: next,
        extensions: [
          EditorView.cspNonce.of(document.querySelector<HTMLMetaElement>('meta[name="sing-box-panel-style-nonce"]')?.content ?? ''),
          EditorState.readOnly.of(true),
          EditorView.editable.of(false),
          lineNumbers(),
          json(),
          editorTheme,
          diffTheme,
          unifiedMergeView({ mergeControls: false, original }),
        ],
      }),
    });
    return () => view.destroy();
  }, [next, original]);
  return <div className='h-full min-h-0 overflow-hidden rounded-[var(--radius-control)] border' ref={hostRef} />;
}
