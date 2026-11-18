import { tags } from '@lezer/highlight';
import { EditorView } from '@codemirror/view';
import { HighlightStyle, syntaxHighlighting } from '@codemirror/language';

export const editorTheme = [
  EditorView.theme({
    '&': { height: '100%', color: 'var(--foreground)', backgroundColor: 'var(--color-surface-raised)', fontSize: 'var(--text-sm)' },
    '&.cm-focused': { outline: 'none' },
    '.cm-scroller': { overflow: 'auto', fontFamily: 'var(--font-data)', lineHeight: '1.65' },
    '.cm-content': { padding: '12px 0', caretColor: 'var(--foreground)', outline: 'none' },
    '.cm-line': { padding: '0 16px 0 8px' },
    '.cm-gutters': { color: 'var(--muted-foreground)', backgroundColor: 'var(--color-surface-raised)', border: 'none' },
    // CodeMirror draws selections behind the content; active lines must stay translucent.
    '.cm-activeLine': { backgroundColor: 'var(--color-hover)' },
    '.cm-activeLineGutter': { backgroundColor: 'var(--color-surface-raised)' },
    '.cm-cursor': { borderLeftColor: 'var(--foreground)' },
    '&.cm-focused > .cm-scroller > .cm-selectionLayer .cm-selectionBackground, .cm-selectionBackground': { backgroundColor: 'var(--color-selection)' },
    '.cm-line::selection, .cm-line ::selection': { color: 'var(--foreground)' },
    '.cm-foldPlaceholder': { color: 'var(--muted-foreground)', backgroundColor: 'var(--color-surface-raised)', borderColor: 'var(--border)' },
    '.cm-panels': { color: 'var(--foreground)', backgroundColor: 'var(--color-surface)' },
    // The search panel floats below the tool cluster instead of pushing the document down.
    '.cm-panels.cm-panels-top': {
      position: 'absolute', top: 'calc(var(--control-height-xs) + 20px)', right: '8px', left: 'auto',
      width: 'min(440px, calc(100% - 16px))', border: '0', background: 'transparent',
    },
    '.cm-textfield': { color: 'var(--foreground)', background: 'var(--color-surface-raised)', border: '1px solid var(--border)', borderRadius: '4px' },
    '.cm-button': { color: 'var(--foreground)', background: 'var(--color-surface-raised)', border: '1px solid var(--border)', borderRadius: '4px' },
    '.cm-searchMatch': { backgroundColor: 'var(--color-warning-soft)' },
    '.cm-searchMatch.cm-searchMatch-selected': { outline: '1px solid var(--color-warning)' },
  }),
  syntaxHighlighting(HighlightStyle.define([
    { tag: tags.propertyName, color: 'var(--color-syntax-property)' },
    { tag: tags.string, color: 'var(--color-syntax-string)' },
    { tag: tags.number, color: 'var(--color-syntax-number)' },
    { tag: [tags.bool, tags.null], color: 'var(--color-syntax-keyword)' },
    { tag: tags.punctuation, color: 'var(--muted-foreground)' },
    { tag: tags.invalid, color: 'var(--color-danger)' },
  ])),
];

export const chineseEditorPhrases = {
  'Find': '查找', 'Replace': '替换', 'next': '下一个', 'previous': '上一个',
  'all': '全部选中', 'match case': '区分大小写', 'by word': '全词匹配',
  'regexp': '正则表达式', 'replace': '替换', 'replace all': '全部替换',
  'close': '关闭', 'current match': '当前匹配', 'replaced $ matches': '已替换 $ 处',
  'replaced match on line $': '已替换第 $ 行匹配', 'on line': '所在行',
  'Fold line': '折叠行', 'Unfold line': '展开行', 'to': '至',
  'folded code': '已折叠代码', 'unfold': '展开',
};
