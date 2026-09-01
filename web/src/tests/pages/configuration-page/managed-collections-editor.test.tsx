import type { RJSFSchema } from '@rjsf/utils';

import { useState } from 'react';
import userEvent from '@testing-library/user-event';
import { render, screen } from '@testing-library/react';
import { beforeAll, describe, expect, it, vi } from 'vitest';
import { createPrecompiledValidator } from '@rjsf/validator-ajv8';

import type { ReviewedSchemaResolution } from '@/schemas/resolve-reviewed-schema';
import type { CanonicalDraft } from '@/pages/configuration-page/use-canonical-configuration';

import '@/i18n';
import { reviewedSchemaManifest } from '@/schemas/generated';
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

function Harness({ initial }: { initial: CanonicalDraft }) {
  const [draft, setDraft] = useState(initial);
  if (resolution === null) throw new Error('The exact reviewed schema fixture is unavailable.');
  return (
    <>
      <ManagedCollectionsEditor
        draft={draft}
        onChange={(change) => setDraft((current) => change(current))}
        resolution={resolution}
      />
      <output aria-label='Canonical draft'>{JSON.stringify(draft)}</output>
    </>
  );
}

describe.skipIf(reviewedEntry === undefined)('managedCollectionsEditor', () => {
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
    await user.click(screen.getByRole('button', { name: 'Create' }));

    const encoded = screen.getByLabelText('Canonical draft').textContent ?? '';
    expect(encoded).toContain('"tag":"inbound-1"');
    expect(encoded).toContain('"type":"mixed"');
    expect(encoded).not.toContain('_panel');
  });

  it('surfaces malformed legacy nodes as repairable instead of crashing', async () => {
    const user = userEvent.setup();
    render(
      <Harness initial={{
        inbounds: [{ tag: 'broken', listen: '::1' }],
      }} />,
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
      <Harness initial={{
        inbounds: [
          { type: 'mixed', tag: 'first' },
          { type: 'socks', tag: 'second' },
        ],
      }} />,
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
