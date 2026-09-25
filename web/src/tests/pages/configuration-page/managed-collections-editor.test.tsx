import type { RJSFSchema } from '@rjsf/utils';

import { useState } from 'react';
import userEvent from '@testing-library/user-event';
import { beforeAll, describe, expect, it } from 'vitest';
import { createPrecompiledValidator } from '@rjsf/validator-ajv8';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';

import type { ReviewedSchemaResolution } from '@/schemas/resolve-reviewed-schema';
import type { CanonicalDraft } from '@/pages/configuration-page/use-canonical-configuration';

import '@/i18n';
import { reviewedSchemaManifest } from '@/schemas/generated';
import { ManagedCollectionsEditor } from '@/pages/configuration-page/managed-collections-editor';
import { encodeCanonicalDraft, parseCanonicalDraft } from '@/pages/configuration-page/use-canonical-configuration';

const reviewedEntry = reviewedSchemaManifest['1.14.0'];
let resolution: ReviewedSchemaResolution;

beforeAll(async () => {
  expect(reviewedEntry, 'Missing committed 1.14.0 Schema fixture').toBeDefined();
  const reviewed = await reviewedEntry.load();
  resolution = {
    schema: reviewed.schema,
    createValidator: (sectionSchema: RJSFSchema) =>
      createPrecompiledValidator(reviewed.validateFns as never, sectionSchema),
  };
});

function Harness({ initial, linkedTag, selectedCollection, disabled = false }: {
  initial: CanonicalDraft;
  linkedTag?: string;
  selectedCollection?: 'inbounds' | 'outbounds' | 'endpoints' | 'services';
  disabled?: boolean;
}) {
  const [draft, setDraft] = useState(initial);
  return (
    <>
      <ManagedCollectionsEditor
        draft={draft}
        disabled={disabled}
        linkedTag={linkedTag}
        selectedCollection={selectedCollection ?? (linkedTag === undefined ? undefined : 'inbounds')}
        onChange={(change) => setDraft((current) => change(current))}
        resolution={resolution}
      />
      <output aria-label='Canonical draft'>{encodeCanonicalDraft(draft)}</output>
    </>
  );
}

describe('managedCollectionsEditor', () => {
  it.each(['create', 'edit'])('retains the %s form until its closing animation finishes', async (mode) => {
    const user = userEvent.setup();
    render(<Harness initial={{ inbounds: [{ type: 'mixed', tag: 'existing' }] }} />);
    await user.click(screen.getByRole('tab', { name: 'Inbounds' }));
    await user.click(screen.getByRole('button', { name: mode === 'create' ? 'Add node' : 'Edit' }));
    if (mode === 'create') await user.click(screen.getByRole('button', { name: 'Continue' }));
    const dialog = screen.getByRole('dialog');
    fireEvent.change(within(dialog).getByLabelText('Listen port'), { target: { value: '2080' } });
    let finish!: () => void;
    const finished = new Promise<void>((resolve) => {
      finish = resolve;
    });
    Object.defineProperty(dialog, 'getAnimations', { value: () => [{ finished }] });
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }));
    expect(dialog).toHaveAttribute('data-closed');
    expect(within(dialog).getByLabelText('Listen port')).toHaveValue(2080);
    await act(async () => {
      finish();
    });
    await waitFor(() => expect(dialog).not.toBeInTheDocument());
  });

  it('opens existing entries only through Edit and commits dialog changes only on confirmation', async () => {
    const user = userEvent.setup();
    const initial = { inbounds: [{ type: 'mixed', tag: 'local', listen_port: 2080, future: { keep: true } }] };
    render(<Harness initial={initial} />);
    await user.click(screen.getByRole('tab', { name: 'Inbounds' }));
    await user.click(screen.getByText('local'));
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Edit' }));
    let dialog = screen.getByRole('dialog', { name: 'Edit entry' });
    fireEvent.change(within(dialog).getByLabelText('Listen port'), { target: { value: '3080' } });
    expect(JSON.parse(screen.getByLabelText('Canonical draft').textContent ?? '{}')).toEqual(initial);
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }));
    expect(JSON.parse(screen.getByLabelText('Canonical draft').textContent ?? '{}')).toEqual(initial);
    await user.click(screen.getByRole('button', { name: 'Edit' }));
    dialog = screen.getByRole('dialog');
    expect(within(dialog).getByLabelText('Listen port')).toHaveValue(2080);
    fireEvent.change(within(dialog).getByLabelText('Listen port'), { target: { value: '4080' } });
    await user.click(within(dialog).getByRole('button', { name: 'Save changes' }));
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(JSON.parse(screen.getByLabelText('Canonical draft').textContent ?? '{}')).toEqual({
      inbounds: [{ ...initial.inbounds[0], listen_port: 4080 }],
    });
  });

  it('opens a linked entry in the edit dialog and discards changes on Escape', async () => {
    const user = userEvent.setup();
    const initial = { inbounds: [{ type: 'mixed', tag: 'linked', listen_port: 2080 }] };
    render(<Harness initial={initial} linkedTag='linked' />);
    const dialog = screen.getByRole('dialog', { name: 'Edit entry' });
    fireEvent.change(within(dialog).getByLabelText('Listen port'), { target: { value: '3080' } });
    await user.keyboard('{Escape}');
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(JSON.parse(screen.getByLabelText('Canonical draft').textContent ?? '{}')).toEqual(initial);
  });

  it('edits prepared fields inside the dialog and discards them when cancelled', async () => {
    const user = userEvent.setup();
    render(<Harness initial={{ inbounds: [] }} />);
    await user.click(screen.getByRole('tab', { name: /Inbounds/ }));
    await user.click(screen.getByRole('button', { name: 'Add node' }));
    await user.click(screen.getByRole('button', { name: 'Continue' }));
    const dialog = screen.getByRole('dialog');
    fireEvent.change(within(dialog).getByLabelText('Listen port'), { target: { value: '2080' } });
    expect(screen.getByLabelText('Canonical draft')).toHaveTextContent('{"inbounds":[]}');
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }));
    expect(screen.getByLabelText('Canonical draft')).toHaveTextContent('{"inbounds":[]}');
    await user.click(screen.getByRole('button', { name: 'Add node' }));
    await user.click(screen.getByRole('button', { name: 'Continue' }));
    const reopened = screen.getByRole('dialog');
    expect(within(reopened).getByLabelText('Listen port')).not.toHaveValue(2080);
    fireEvent.change(within(reopened).getByLabelText('Listen port'), { target: { value: '2081' } });
    await user.click(within(reopened).getByRole('button', { name: 'Create' }));
    expect(screen.getByLabelText('Canonical draft')).toHaveTextContent('"listen_port":2081');
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Back' })).not.toBeInTheDocument();
  });

  it('creates an inbound locally without adding credentials or changing the draft before confirmation', async () => {
    const user = userEvent.setup();
    render(<Harness initial={{ inbounds: [] }} />);
    await user.click(screen.getByRole('tab', { name: /Inbounds/ }));
    await user.click(screen.getByRole('button', { name: 'Add node' }));
    await user.click(screen.getByRole('button', { name: 'Choose protocol' }));
    await user.click(await screen.findByRole('option', { name: 'anytls' }));
    await user.click(screen.getByRole('button', { name: 'Continue' }));
    expect(screen.getByLabelText('Canonical draft')).not.toHaveTextContent('inbound-1');
    await user.click(await screen.findByRole('button', { name: 'Create' }));
    expect(JSON.parse(screen.getByLabelText('Canonical draft').textContent ?? '{}')).toEqual({
      inbounds: [{ type: 'anytls', tag: 'inbound-1' }],
    });
    expect(screen.getByLabelText('Canonical draft')).toHaveTextContent('"tag":"inbound-1"');
  });
  it('uses only protocol types from the exact reviewed schema', async () => {
    const user = userEvent.setup();
    render(<Harness initial={{ inbounds: [] }} />);

    await user.click(screen.getByRole('tab', { name: /Inbounds/ }));
    await user.click(screen.getByRole('button', { name: 'Add node' }));
    await user.click(screen.getByRole('button', { name: 'Choose protocol' }));

    expect(await screen.findByRole('option', { name: 'anytls' })).toBeInTheDocument();
    expect(screen.queryByRole('option', { name: 'selector' })).not.toBeInTheDocument();
  });

  it('filters protocols and completes the highlighted choice with the keyboard', async () => {
    const user = userEvent.setup();
    render(<Harness initial={{ inbounds: [] }} selectedCollection='inbounds' />);
    await user.click(screen.getByRole('button', { name: 'Add node' }));
    const input = screen.getByRole('combobox', { name: 'Protocol' });
    const continueButton = screen.getByRole('button', { name: 'Continue' });
    await user.click(screen.getByRole('textbox', { name: 'Tag' }));
    await user.tab();
    expect(input).toHaveFocus();
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
    await user.click(input);
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
    await user.clear(input);
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
    await user.type(input, 'ANY');
    expect(await screen.findByRole('option', { name: 'anytls' })).toBeInTheDocument();
    expect(screen.queryByRole('option', { name: 'mixed' })).not.toBeInTheDocument();
    expect(continueButton).toBeDisabled();
    await user.keyboard('{Enter}');
    expect(input).toHaveValue('anytls');
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
    await user.click(input);
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Continue' }));
    await user.click(await screen.findByRole('button', { name: 'Create' }));
    expect(screen.getByLabelText('Canonical draft')).toHaveTextContent('"type":"anytls"');
    expect(JSON.parse(screen.getByLabelText('Canonical draft').textContent ?? '{}')).toEqual({
      inbounds: [{ type: 'anytls', tag: 'inbound-1' }],
    });
  });

  it('rejects an unknown protocol and opens the full list from the dropdown button', async () => {
    const user = userEvent.setup();
    render(<Harness initial={{ outbounds: [] }} selectedCollection='outbounds' />);
    await user.click(screen.getByRole('button', { name: 'Add node' }));
    const input = screen.getByRole('combobox', { name: 'Protocol' });
    const continueButton = screen.getByRole('button', { name: 'Continue' });
    await user.clear(input);
    await user.type(input, 'unknown-protocol');
    expect(await screen.findByText('No matching protocol')).toBeVisible();
    expect(screen.queryByRole('option')).not.toBeInTheDocument();
    expect(continueButton).toBeDisabled();
    await user.clear(input);
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
    await user.type(input, 'unknown-protocol');
    await user.keyboard('{Escape}');
    expect(screen.getByRole('dialog', { name: 'Create node' })).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Choose protocol' }));
    await user.click(await screen.findByRole('option', { name: 'direct' }));
    expect(input).toHaveValue('direct');
    expect(screen.getByRole('button', { name: 'Continue' })).toBeEnabled();
    await user.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(screen.getByLabelText('Canonical draft')).toHaveTextContent('{"outbounds":[]}');
  });

  it('creates a raw sing-box entity without panel metadata', async () => {
    const user = userEvent.setup();
    render(<Harness initial={{ inbounds: [] }} />);

    await user.click(screen.getByRole('tab', { name: /Inbounds/ }));
    await user.click(screen.getByRole('button', { name: 'Add node' }));
    await user.click(screen.getByRole('button', { name: 'Continue' }));
    expect(screen.getByLabelText('Canonical draft')).not.toHaveTextContent('inbound-1');
    await user.click(await screen.findByRole('button', { name: 'Create' }));

    const encoded = screen.getByLabelText('Canonical draft').textContent ?? '';
    expect(encoded).toContain('"tag":"inbound-1"');
    expect(encoded).toContain('"type":"mixed"');
    expect(encoded).not.toContain('_panel');
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Back' })).not.toBeInTheDocument();
  });

  it('surfaces malformed legacy nodes as repairable instead of crashing', async () => {
    const user = userEvent.setup();
    render(
      <Harness
        initial={{
          inbounds: [{ tag: 'broken', listen: '::1' }],
        }}
      />,
    );

    await user.click(screen.getByRole('tab', { name: /Inbounds/ }));
    expect(screen.getByText('Needs repair')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Repair identity' }));

    const encoded = screen.getByLabelText('Canonical draft').textContent ?? '';
    expect(encoded).toContain('"type":"mixed"');
    expect(encoded).toContain('"tag":"broken"');
    expect(encoded).not.toContain('_panel');
  });

  it.each(['inbounds', 'outbounds', 'endpoints', 'services'] as const)('adds %s from an empty table and stages the dialog until confirmation', async (collection) => {
    const user = userEvent.setup();
    render(<Harness initial={{ [collection]: [] }} selectedCollection={collection} />);
    const table = screen.getByRole('table');
    const addressHeader = collection === 'inbounds' ? 'Listen address' : collection === 'outbounds' ? 'Server address' : 'Details';
    expect(within(table).getAllByRole('columnheader').map(header => header.textContent)).toEqual(['Tag', 'Type', addressHeader, 'Actions']);
    expect(within(table).getAllByRole('row')).toHaveLength(2);
    expect(screen.queryByRole('tablist')).not.toBeInTheDocument();
    const add = within(table).getByRole('button', { name: 'Add node' });
    add.focus();
    await user.keyboard('{Enter}');
    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Cancel' }));
    expect(screen.getByLabelText('Canonical draft')).toHaveTextContent(`"${collection}":[]`);
    await user.click(add);
    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Continue' }));
    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Create' }));
    expect(within(table).getAllByRole('row')).toHaveLength(3);
    const entryRow = within(table).getAllByRole('row')[1];
    expect(within(entryRow).getAllByRole('cell')).toHaveLength(4);
    expect(within(entryRow).getAllByRole('button').map(button => button.getAttribute('aria-label') ?? button.textContent)).toEqual([
      'Edit', expect.stringMatching(/^Move .+ up$/), expect.stringMatching(/^Move .+ down$/), 'Delete',
    ]);
    expect(within(table).getByRole('button', { name: /Move .+ up/ })).toBeDisabled();
    expect(within(table).getByRole('button', { name: /Move .+ down/ })).toBeDisabled();
  });

  it('moves whole records by keyboard and edits and deletes the reordered entry', async () => {
    const user = userEvent.setup();
    render(
      <Harness selectedCollection='inbounds' initial={parseCanonicalDraft(
        '{"inbounds":[{"type":"mixed","tag":"first","listen_port":2080,"future":900719925474099312345},{"type":"socks","tag":"second","listen_port":3080}]}',
      )} />,
    );
    expect(screen.getByRole('button', { name: 'Move first up' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Move second down' })).toBeDisabled();
    screen.getByRole('button', { name: 'Move first down' }).focus();
    await user.keyboard(' ');
    const table = screen.getByRole('table');
    expect(within(table).getAllByRole('row')[1]).toHaveTextContent('second');
    expect(screen.getByRole('button', { name: 'Move first down' })).toBeDisabled();
    expect(screen.getByLabelText('Canonical draft')).toHaveTextContent('"future":900719925474099312345');
    await user.click(screen.getByRole('button', { name: 'Move first up' }));
    expect(within(table).getAllByRole('row')[1]).toHaveTextContent('first');
    await user.click(screen.getByRole('button', { name: 'Move first down' }));
    const movedRow = within(table).getAllByRole('row')[2];
    await user.click(within(movedRow).getByRole('button', { name: 'Edit' }));
    const dialog = screen.getByRole('dialog');
    expect(within(dialog).getByLabelText('Listen port')).toHaveValue(2080);
    fireEvent.change(within(dialog).getByLabelText('Listen port'), { target: { value: '4080' } });
    await user.click(within(dialog).getByRole('button', { name: 'Save changes' }));
    expect(screen.getByLabelText('Canonical draft')).toHaveTextContent('"listen_port":4080');
    expect(screen.getByLabelText('Canonical draft')).toHaveTextContent('"future":900719925474099312345');
    await user.click(within(within(table).getAllByRole('row')[2]).getByRole('button', { name: 'Delete' }));
    await user.click(within(screen.getByRole('alertdialog')).getByRole('button', { name: 'Delete' }));
    expect(JSON.parse(screen.getByLabelText('Canonical draft').textContent ?? '{}')).toEqual({
      inbounds: [{ type: 'socks', tag: 'second', listen_port: 3080 }],
    });
  });

  it('disables adding and moving while the configuration is locked', () => {
    render(
      <Harness selectedCollection='inbounds' disabled initial={{ inbounds: [
        { type: 'mixed', tag: 'first' }, { type: 'mixed', tag: 'second' },
      ] }} />,
    );
    for (const button of within(screen.getByRole('table')).getAllByRole('button')) expect(button).toBeDisabled();
  });

  it.each([
    { collection: 'inbounds', value: { type: 'mixed', listen: '0.0.0.0', listen_port: 2080, server: 'wrong.example', server_port: 443 }, expected: '0.0.0.0:2080' },
    { collection: 'inbounds', value: { type: 'mixed', listen: '::', listen_port: 2080 }, expected: '[::]:2080' },
    { collection: 'inbounds', value: { type: 'mixed', listen_port: 2080 }, expected: '—:2080' },
    { collection: 'inbounds', value: { type: 'tun' }, expected: '—' },
    { collection: 'outbounds', value: { type: 'trojan', server: 'proxy.example', server_port: 443, listen: 'wrong.example', listen_port: 2080, tls: { server_name: 'sni.example' }, transport: { type: 'ws' } }, expected: 'proxy.example:443' },
    { collection: 'outbounds', value: { type: 'trojan', server: '2001:db8::1', server_port: 443 }, expected: '[2001:db8::1]:443' },
    { collection: 'outbounds', value: { type: 'trojan', server: '[2001:db8::1]', server_port: '443' }, expected: '[2001:db8::1]:443' },
    { collection: 'outbounds', value: { type: 'hysteria2', server: 'hop.example', server_port: 443, server_ports: ['2000:3000', '4000'] }, expected: 'hop.example:2000:3000, 4000' },
    { collection: 'outbounds', value: { type: 'direct' }, expected: '—' },
    { collection: 'outbounds', value: { type: 'selector', outbounds: ['proxy'] }, expected: '—' },
    { collection: 'outbounds', value: { type: 'socks', server: 'proxy.example' }, expected: 'proxy.example:—' },
  ] as const)('shows only the $collection address as $expected', ({ collection, value, expected }) => {
    render(<Harness selectedCollection={collection} initial={parseCanonicalDraft(JSON.stringify({ [collection]: [{ tag: 'node', ...value }] }))} />);
    const row = within(screen.getByRole('table')).getAllByRole('row')[1];
    expect(within(row).getAllByRole('cell')[2].textContent).toBe(expected);
  });

  it('copies the current unsaved entry JSON without changing large numeric values', async () => {
    const user = userEvent.setup();
    render(<Harness selectedCollection='inbounds' initial={parseCanonicalDraft('{"inbounds":[{"type":"mixed","tag":"copy","listen":"::","listen_port":2080,"future":900719925474099312345}]}')} />);
    await user.click(screen.getByRole('button', { name: 'Edit' }));
    const dialog = screen.getByRole('dialog');
    fireEvent.change(within(dialog).getByLabelText('Listen port'), { target: { value: '3080' } });
    await user.click(within(dialog).getByRole('tab', { name: 'JSON' }));
    const preview = within(dialog).getByLabelText('JSON', { selector: 'pre' });
    expect(preview.textContent).toContain('"listen_port": 3080');
    expect(preview.textContent).toContain('900719925474099312345');
    await user.click(within(dialog).getByRole('button', { name: 'Copy JSON' }));
    expect(await navigator.clipboard.readText()).toBe(preview.textContent);
    expect(screen.getByLabelText('Canonical draft')).toHaveTextContent('"listen_port":2080');
  });
});
