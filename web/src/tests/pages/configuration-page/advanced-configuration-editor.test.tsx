import { useState } from 'react';
import { undo } from '@codemirror/commands';
import { EditorView } from '@codemirror/view';
import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';

import '@/i18n';
import { AdvancedConfigurationEditor } from '@/pages/configuration-page/advanced-configuration-editor';

const original = '{"large":90071992547409931234567890,"threshold":4.2000e+99,"future":{"enabled":true}}';
function Harness() {
  const [text, setText] = useState(original);
  return <AdvancedConfigurationEditor text={text} error={null} disabled={false} onChange={setText} />;
}

describe('advanced JSON editor', () => {
  it('searches by case and whole word, navigates matches and replaces regex captures with undo', async () => {
    const user = userEvent.setup();
    render(<AdvancedConfigurationEditor text='["Info", "info", "information", "node-1", "node-2"]' error={null} disabled={false} onChange={vi.fn()} />);
    const view = EditorView.findFromDOM(screen.getByRole('textbox', { name: 'sing-box configuration JSON' }))!;
    await user.click(screen.getByRole('button', { name: 'Find / replace' }));
    const panel = within(screen.getByRole('region', { name: 'Find / replace' }));
    const input = panel.getByRole('textbox', { name: 'Find' });
    expect(input).toHaveFocus();
    expect(panel.queryByRole('textbox', { name: 'Replace' })).not.toBeInTheDocument();
    await user.type(input, 'info');
    expect(panel.getByRole('status')).toHaveTextContent('3 matches');
    await user.click(panel.getByRole('button', { name: 'Match case' }));
    expect(panel.getByRole('status')).toHaveTextContent('2 matches');
    await user.click(panel.getByRole('button', { name: 'Whole word' }));
    expect(panel.getByRole('status')).toHaveTextContent('1 match');
    await user.click(panel.getByRole('button', { name: 'Next match' }));
    expect(view.state.sliceDoc(view.state.selection.main.from, view.state.selection.main.to)).toBe('info');
    expect(panel.getByRole('status')).toHaveTextContent('1 / 1');
    await user.click(panel.getByRole('button', { name: 'Regular expression' }));
    fireEvent.change(input, { target: { value: 'node-(\\d)' } });
    await user.click(panel.getByRole('button', { name: 'Toggle replace' }));
    await user.type(panel.getByRole('textbox', { name: 'Replace' }), 'server-$1');
    await user.click(panel.getByRole('button', { name: 'Select all matches' }));
    expect(view.state.selection.ranges).toHaveLength(2);
    await user.click(panel.getByRole('button', { name: 'Replace all' }));
    expect(view.state.doc.toString()).toContain('"server-1", "server-2"');
    act(() => {
      undo(view);
    });
    expect(view.state.doc.toString()).toContain('"node-1", "node-2"');
    await user.click(input);
    await user.keyboard('{Escape}');
    await waitFor(() => expect(screen.queryByRole('region', { name: 'Find / replace' })).not.toBeInTheDocument());
    expect(view.hasFocus).toBe(true);
  });

  it('reports invalid expressions, prevents read-only replacements and reopens its custom panel', async () => {
    const user = userEvent.setup();
    render(<AdvancedConfigurationEditor text='{"test":1}' error={null} disabled onChange={vi.fn()} />);
    await user.click(screen.getByRole('button', { name: 'Find / replace' }));
    fireEvent.change(screen.getByRole('textbox', { name: 'Find' }), { target: { value: '[' } });
    await user.click(screen.getByRole('button', { name: 'Regular expression' }));
    expect(screen.getByRole('status')).toHaveTextContent('Invalid regex');
    expect(screen.getByRole('button', { name: 'Next match' })).toBeDisabled();
    fireEvent.change(screen.getByRole('textbox', { name: 'Find' }), { target: { value: 'test' } });
    await user.click(screen.getByRole('button', { name: 'Toggle replace' }));
    expect(screen.getByRole('textbox', { name: 'Replace' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Replace all' })).toBeDisabled();
    await user.click(screen.getByRole('button', { name: 'Close' }));
    await user.click(screen.getByRole('button', { name: 'Find / replace' }));
    expect(screen.getByRole('textbox', { name: 'Find' })).toHaveValue('test');
    expect(document.querySelectorAll('.editor-search')).toHaveLength(1);
    expect(document.querySelector('.cm-search')).not.toBeInTheDocument();
  });

  it('formats losslessly in one undo step, folds and opens search', async () => {
    const user = userEvent.setup();
    render(<Harness />);
    const editor = screen.getByRole('textbox', { name: 'sing-box configuration JSON' });
    const view = EditorView.findFromDOM(editor)!;
    expect(view.state.doc.toString()).toBe(original);
    await user.click(screen.getByRole('button', { name: 'Format' }));
    expect(view.state.doc.toString()).toContain('\n  "large": 90071992547409931234567890,');
    expect(view.state.doc.toString()).toContain('4.2000e+99');
    act(() => {
      undo(view);
    });
    expect(view.state.doc.toString()).toBe(original);
    await user.click(screen.getByRole('button', { name: 'Format' }));
    await user.click(screen.getByRole('button', { name: 'Fold all' }));
    expect(view.dom.querySelector('.cm-foldPlaceholder')).not.toBeNull();
    await user.click(screen.getByRole('button', { name: 'Unfold all' }));
    expect(view.dom.querySelector('.cm-foldPlaceholder')).toBeNull();
    await user.click(screen.getByRole('button', { name: 'Find / replace' }));
    expect(screen.getByRole('textbox', { name: 'Find' })).toBeVisible();
  });

  it('retains invalid text and prevents formatting or edits while locked', async () => {
    const onChange = vi.fn();
    const { rerender } = render(<AdvancedConfigurationEditor text='{"log":' error={new Error('Incomplete JSON')} disabled={false} onChange={onChange} />);
    expect(screen.getByRole('button', { name: 'Format' })).toBeDisabled();
    expect(screen.getByRole('alert')).toHaveTextContent('Incomplete JSON');
    const editor = screen.getByRole('textbox', { name: 'sing-box configuration JSON' });
    const view = EditorView.findFromDOM(editor)!;
    expect(view.state.doc.toString()).toBe('{"log":');
    rerender(<AdvancedConfigurationEditor text={original} error={null} disabled onChange={onChange} />);
    expect(view.state.doc.toString()).toBe(original);
    expect(view.state.readOnly).toBe(true);
    expect(editor).toHaveAttribute('contenteditable', 'false');
    expect(editor).not.toHaveAttribute('aria-describedby');
  });
});
