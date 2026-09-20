import type { EditorState } from '@codemirror/state';
import type { EditorView, Panel, ViewUpdate } from '@codemirror/view';

/** Bridges CodeMirror's panel lifecycle to the editor's React tree. */
export class SearchPanelHost implements Panel {
  readonly dom = document.createElement('div');
  readonly top = true;
  private state: EditorState;
  private listeners = new Set<() => void>();

  constructor(
    readonly view: EditorView,
    private onMount: (panel: SearchPanelHost) => void,
    private onDestroy: () => void,
  ) {
    this.state = view.state;
    this.dom.className = 'editor-search-host';
  }

  getSnapshot = () => this.state;
  subscribe = (listener: () => void) => {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  };

  mount() {
    this.onMount(this);
  }

  destroy() {
    this.onDestroy();
  }

  update(update: ViewUpdate) {
    this.state = update.state;
    this.listeners.forEach(listener => listener());
  }
}
