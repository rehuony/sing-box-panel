import { tags } from '@lezer/highlight';
import { EditorView } from '@codemirror/view';
import { HighlightStyle, syntaxHighlighting } from '@codemirror/language';

export const editorTheme = [
  EditorView.theme({
    '&': { height: '100%', color: 'var(--color-text)', backgroundColor: 'var(--color-surface-raised)', fontSize: 'var(--text-sm)' },
    '&.cm-focused': { outline: 'none' },
    '.cm-scroller': { overflow: 'auto', fontFamily: 'var(--font-data)', lineHeight: '1.65' },
    '.cm-content': { padding: '12px 0', caretColor: 'var(--color-text)' },
    '.cm-line': { padding: '0 16px 0 8px' },
    '.cm-gutters': { color: 'var(--color-text-muted)', backgroundColor: 'var(--color-surface-raised)', border: 'none' },
    // CodeMirror draws selections behind the content; active lines must stay translucent.
    '.cm-activeLine': { backgroundColor: 'color-mix(in srgb, var(--color-accent) 5%, transparent)' },
    '.cm-activeLineGutter': { backgroundColor: 'var(--color-paper-2)' },
    '.cm-cursor': { borderLeftColor: 'var(--color-text)' },
    '&.cm-focused > .cm-scroller > .cm-selectionLayer .cm-selectionBackground, .cm-selectionBackground': { backgroundColor: 'color-mix(in srgb, var(--color-accent) 24%, transparent)' },
    '.cm-foldPlaceholder': { color: 'var(--color-text-muted)', backgroundColor: 'var(--color-paper-2)', borderColor: 'var(--color-rule-2)' },
    '.cm-panels': { color: 'var(--color-text)', backgroundColor: 'var(--color-surface)' },
    '.cm-panels.cm-panels-top': { borderBottomColor: 'var(--color-rule-2)' },
    '.cm-textfield': { color: 'var(--color-text)', background: 'var(--color-paper-2)', border: '1px solid var(--color-rule-2)', borderRadius: '4px' },
    '.cm-button': { color: 'var(--color-text)', background: 'var(--color-paper-2)', border: '1px solid var(--color-rule-2)', borderRadius: '4px' },
    '.cm-searchMatch': { backgroundColor: 'var(--color-warning-soft)' },
    '.cm-searchMatch.cm-searchMatch-selected': { outline: '1px solid var(--color-warning)' },
  }),
  syntaxHighlighting(HighlightStyle.define([
    { tag: tags.propertyName, color: 'var(--json-property)' },
    { tag: tags.string, color: 'var(--json-string)' },
    { tag: tags.number, color: 'var(--json-number)' },
    { tag: [tags.bool, tags.null], color: 'var(--json-keyword)' },
    { tag: tags.punctuation, color: 'var(--color-text-muted)' },
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
