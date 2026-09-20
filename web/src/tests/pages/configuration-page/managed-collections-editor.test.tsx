import type { RJSFSchema } from '@rjsf/utils';

import { useState } from 'react';
import userEvent from '@testing-library/user-event';
import { beforeAll, describe, expect, it, vi } from 'vitest';
import { createPrecompiledValidator } from '@rjsf/validator-ajv8';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';

import type { ReviewedSchemaResolution } from '@/schemas/resolve-reviewed-schema';
import type { CanonicalDraft } from '@/pages/configuration-page/use-canonical-configuration';

import '@/i18n';
import { ApiClientProvider } from '@/api/api-client-context';
import { reviewedSchemaManifest } from '@/schemas/generated';
import { createMockApiClient } from '@/tests/api/mock-api-client';
import { ManagedCollectionsEditor } from '@/pages/configuration-page/managed-collections-editor';

const reviewedEntry = reviewedSchemaManifest['1.14.0'];
let resolution: ReviewedSchemaResolution | null = null;

beforeAll(async () => {
  const reviewed = await reviewedEntry?.load();
  if (reviewed === undefined) return;
  resolution = {
    schema: reviewed.schema,
    createValidator: (sectionSchema: RJSFSchema) =>
      createPrecompiledValidator(reviewed.validateFns as never, sectionSchema),
  };
});

function Harness({ initial, linkedTag }: { initial: CanonicalDraft; linkedTag?: string }) {
  const [draft, setDraft] = useState(initial);
  const [client] = useState(() =>
    createMockApiClient({
      newInboundDefaults: vi
        .fn()
        .mockImplementation(async (type) =>
          type === 'anytls'
            ? { type, users: [{ name: 'shared', password: 'fixture-identity' }] }
            : { type },
        ),
    }),
  );
  if (resolution === null) throw new Error('The exact reviewed schema fixture is unavailable.');
  return (
    <ApiClientProvider client={client}>
      <ManagedCollectionsEditor
        draft={draft}
        linkedTag={linkedTag}
        selectedCollection={linkedTag === undefined ? undefined : 'inbounds'}
        onChange={(change) => setDraft((current) => change(current))}
        resolution={resolution}
      />
      <output aria-label='Canonical draft'>{JSON.stringify(draft)}</output>
    </ApiClientProvider>
  );
}

describe.skipIf(reviewedEntry === undefined)('managedCollectionsEditor', () => {
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

  it('adds the prepared identity to the editable entry without saving the file', async () => {
    const user = userEvent.setup();
    render(<Harness initial={{ inbounds: [] }} />);
    await user.click(screen.getByRole('tab', { name: /Inbounds/ }));
    await user.click(screen.getByRole('button', { name: 'Add node' }));
    await user.click(screen.getByLabelText('Protocol'));
    await user.click(await screen.findByRole('option', { name: 'anytls' }));
    await user.click(screen.getByRole('button', { name: 'Continue' }));
    expect(screen.getByLabelText('Canonical draft')).not.toHaveTextContent('inbound-1');
    await user.click(await screen.findByRole('button', { name: 'Create' }));
    expect(screen.getByLabelText('Canonical draft')).toHaveTextContent('fixture-identity');
    expect(screen.getByLabelText('Canonical draft')).toHaveTextContent('"tag":"inbound-1"');
  });
  it('uses only protocol types from the exact reviewed schema', async () => {
    const user = userEvent.setup();
    render(<Harness initial={{ inbounds: [] }} />);

    await user.click(screen.getByRole('tab', { name: /Inbounds/ }));
    await user.click(screen.getByRole('button', { name: 'Add node' }));
    await user.click(screen.getByLabelText('Protocol'));

    expect(await screen.findByRole('option', { name: 'anytls' })).toBeInTheDocument();
    expect(screen.queryByRole('option', { name: 'selector' })).not.toBeInTheDocument();
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
    await user.click(screen.getByRole('button', { name: 'Actions for broken' }));
    await user.click(await screen.findByRole('menuitem', { name: 'Repair identity' }));

    const encoded = screen.getByLabelText('Canonical draft').textContent ?? '';
    expect(encoded).toContain('"type":"mixed"');
    expect(encoded).toContain('"tag":"broken"');
    expect(encoded).not.toContain('_panel');
  });

  it('reorders a collection with the keyboard drag handle', async () => {
    const user = userEvent.setup();
    render(
      <Harness
        initial={{
          inbounds: [
            { type: 'mixed', tag: 'first' },
            { type: 'socks', tag: 'second' },
          ],
        }}
      />,
    );

    await user.click(screen.getByRole('tab', { name: /Inbounds/ }));
    screen.getAllByRole('article').forEach((card, index) => {
      vi.spyOn(card, 'getBoundingClientRect').mockReturnValue({
        bottom: 40 + index * 64,
        height: 48,
        left: 0,
        right: 320,
        top: index * 64,
        width: 320,
        x: 0,
        y: index * 64,
        toJSON: () => ({}),
      });
    });
    const handle = screen.getByRole('button', { name: 'Reorder first' });
    handle.focus();
    await user.keyboard(' ');
    await user.keyboard('{ArrowDown}');
    await user.keyboard(' ');

    const encoded = screen.getByLabelText('Canonical draft').textContent ?? '';
    expect(encoded.indexOf('"tag":"second"')).toBeLessThan(encoded.indexOf('"tag":"first"'));
  });
});
