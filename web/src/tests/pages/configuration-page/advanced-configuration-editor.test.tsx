import { useState } from 'react';
import { parse } from 'lossless-json';
import { undo } from '@codemirror/commands';
import { EditorView } from '@codemirror/view';
import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';

import '@/i18n';
import { toast } from '@/components/ui/toast-manager';
import { AdvancedConfigurationEditor } from '@/pages/configuration-page/advanced-configuration-editor';

const original = `{"large":90071992547409931234567890,"threshold":4.2000e+99,"future":{"enabled":true},"inbounds":[
  {"type":"hysteria2","tag":"hy2-in","listen":"0.0.0.0","listen_port":18053,"tls":{"enabled":true,"alpn":"h3","certificate_path":"cert.pem","key_path":"key.pem","min_version":"1.2","server_name":"example.com"},"ignore_client_bandwidth":true,"obfs":{"password":"test-obfs","type":"salamander"},"tcp_fast_open":false,"users":[{"password":"test-b","name":"b"},{"name":"a","password":"test-a"}]},
  {"users":[{"password":"test-anytls","name":"a"}],"tcp_fast_open":true,"reuse_addr":true,"tls":{"key_path":"key.pem","server_name":"example.com","alpn":["h2","http/1.1"],"enabled":true,"min_version":"1.2","certificate_path":"cert.pem"},"listen_port":18443,"listen":"0.0.0.0","tag":"anytls-in","type":"anytls"}
]}`;
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

  it('orders protocol and TLS fields losslessly via the button and shortcut in one undo step', async () => {
    const user = userEvent.setup();
    render(<Harness />);
    const editor = screen.getByRole('textbox', { name: 'sing-box configuration JSON' });
    const view = EditorView.findFromDOM(editor)!;
    expect(view.state.doc.toString()).toBe(original);
    await user.click(screen.getByRole('button', { name: 'Format and order fields' }));
    const formatted = view.state.doc.toString();
    expect(formatted).toContain('\n  "large": 90071992547409931234567890,');
    expect(formatted).toContain('4.2000e+99');
    expect(parse(formatted)).toEqual(parse(original));
    const { inbounds } = JSON.parse(formatted);
    expect(Object.keys(inbounds[0])).toEqual(['type', 'tag', 'listen', 'listen_port', 'tcp_fast_open', 'users', 'ignore_client_bandwidth', 'obfs', 'tls']);
    expect(Object.keys(inbounds[1])).toEqual(['type', 'tag', 'listen', 'listen_port', 'reuse_addr', 'tcp_fast_open', 'users', 'tls']);
    for (const inbound of inbounds) {
      expect(Object.keys(inbound.tls)).toEqual(['enabled', 'server_name', 'alpn', 'min_version', 'certificate_path', 'key_path']);
      expect(Object.keys(inbound.users[0])).toEqual(['name', 'password']);
    }
    expect(Object.keys(inbounds[0].obfs)).toEqual(['type', 'password']);
    act(() => {
      undo(view);
    });
    expect(view.state.doc.toString()).toBe(original);
    fireEvent.keyDown(editor, { key: 'F', code: 'KeyF', keyCode: 70, ctrlKey: true, shiftKey: true });
    expect(view.state.doc.toString()).toBe(formatted);
    await user.click(screen.getByRole('button', { name: 'Format and order fields' }));
    expect(view.state.doc.toString()).toBe(formatted);
    await user.click(screen.getByRole('button', { name: 'Fold all' }));
    expect(view.dom.querySelector('.cm-foldPlaceholder')).not.toBeNull();
    await user.click(screen.getByRole('button', { name: 'Unfold all' }));
    expect(view.dom.querySelector('.cm-foldPlaceholder')).toBeNull();
    await user.click(screen.getByRole('button', { name: 'Find / replace' }));
    expect(screen.getByRole('textbox', { name: 'Find' })).toBeVisible();
  });

  it('retains invalid text and prevents formatting or edits while locked', async () => {
    const addToast = vi.spyOn(toast, 'add');
    const onChange = vi.fn();
    const { rerender } = render(<AdvancedConfigurationEditor text='{"log":' error={new Error('Incomplete JSON')} disabled={false} onChange={onChange} />);
    expect(screen.getByRole('button', { name: 'Format and order fields' })).toBeDisabled();
    await waitFor(() => expect(addToast).toHaveBeenCalledWith(expect.objectContaining({ type: 'error', title: 'Incomplete JSON' })));
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    addToast.mockRestore();
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
