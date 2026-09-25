import type { RJSFSchema } from '@rjsf/utils';

import { useState } from 'react';
import userEvent from '@testing-library/user-event';
import { beforeAll, describe, expect, it } from 'vitest';
import { createPrecompiledValidator } from '@rjsf/validator-ajv8';
import { fireEvent, render, screen } from '@testing-library/react';

import type { ReviewedSchemaResolution } from '@/schemas/resolve-reviewed-schema';

import '@/i18n';
import { reviewedSchemaManifest } from '@/schemas/generated';
import { schemaProperties } from '@/pages/configuration-page/schema-ui';
import { SchemaSectionForm } from '@/pages/configuration-page/schema-section-form';
import { encodeCanonicalDraft, parseCanonicalDraft } from '@/pages/configuration-page/use-canonical-configuration';

const reviewedEntry = reviewedSchemaManifest['1.14.0'];
const fingerprint = 'client_certificate_public_key_sha256';
let resolution: ReviewedSchemaResolution;
let tlsFields: Record<string, RJSFSchema>;

beforeAll(async () => {
  expect(reviewedEntry, 'Missing committed 1.14.0 Schema fixture').toBeDefined();
  const reviewed = await reviewedEntry.load();
  resolution = {
    schema: reviewed.schema,
    createValidator: (root) => createPrecompiledValidator(reviewed.validateFns as never, root),
  };
  tlsFields = schemaProperties({ $ref: '#/$defs/InboundTLSOptions' }, reviewed.schema);
});

function Harness({ field, initial, disabled = false }: { field: string; initial: unknown; disabled?: boolean }) {
  const [draft, setDraft] = useState(() => parseCanonicalDraft(
    `{"section":{"${field}":${JSON.stringify(initial)},"enabled":false,"future":900719925474099312345}}`,
  ));
  const schema: RJSFSchema = { type: 'object', properties: { [field]: tlsFields[field], enabled: { type: 'boolean' } } };
  return (
    <>
      <SchemaSectionForm basePointer='/section' data={draft.section} disabled={disabled} onChange={setDraft}
        resolution={resolution} schema={schema} />
      <output aria-label='Representation draft'>{encodeCanonicalDraft(draft)}</output>
    </>
  );
}

describe('schema value representations', () => {
  it.each(['alpn', 'certificate', 'client_certificate', 'client_certificate_path', 'cipher_suites'])(
    'uses a single Single value / List selector for %s', async (field) => {
      const user = userEvent.setup();
      render(<Harness field={field} initial='' />);
      const selector = screen.getByRole('combobox');
      expect(selector).toHaveTextContent('Single value');
      await user.click(selector);
      expect((await screen.findAllByRole('option')).map((option) => option.textContent)).toEqual(['Single value', 'List']);
      await user.click(screen.getByRole('option', { name: 'List' }));
      expect(screen.getByLabelText('Representation draft')).toHaveTextContent(`"${field}":[]`);
      await user.click(selector);
      await user.click(await screen.findByRole('option', { name: 'Single value' }));
      fireEvent.change(screen.getByRole('textbox'), { target: { value: 'value' } });
      expect(screen.getByLabelText('Representation draft')).toHaveTextContent(`"${field}":"value"`);
    },
  );

  it('flattens the fingerprint choices without a second value type selector', async () => {
    const user = userEvent.setup();
    render(<Harness field={fingerprint} initial='old-hash' />);
    const selector = screen.getByRole('combobox');
    expect(selector).toHaveTextContent('Single value');
    await user.click(selector);
    expect((await screen.findAllByRole('option')).map((option) => option.textContent)).toEqual(['Single value', 'List', 'Byte sequence']);
    await user.keyboard('{Escape}');
    fireEvent.change(screen.getByDisplayValue('old-hash'), { target: { value: 'new-hash' } });
    expect(screen.getByLabelText('Representation draft')).toHaveTextContent(`"${fingerprint}":"new-hash"`);
    expect(screen.getByLabelText('Representation draft')).toHaveTextContent('900719925474099312345');
  });

  it.each([
    ['byte sequence', [12, 255], 'Byte sequence'],
    ['mixed list', ['hash', [12, 255]], 'List'],
    ['empty list', [], 'List'],
  ])('retains a saved %s when another field changes', async (_name, initial, selected) => {
    const user = userEvent.setup();
    render(<Harness field={fingerprint} initial={initial} />);
    expect(screen.getByRole('combobox', { name: fingerprint })).toHaveTextContent(selected as string);
    await user.click(screen.getByRole('switch', { name: 'Enabled' }));
    expect(screen.getByLabelText('Representation draft')).toHaveTextContent(`"${fingerprint}":${JSON.stringify(initial)}`);
    expect(screen.getByLabelText('Representation draft')).toHaveTextContent('900719925474099312345');
  });

  it('edits a saved byte sequence in a mixed list without replacing its text member', () => {
    render(<Harness field={fingerprint} initial={['hash', [12, 255]]} />);
    fireEvent.change(screen.getAllByRole('spinbutton')[0], { target: { value: '42' } });
    expect(screen.getByLabelText('Representation draft')).toHaveTextContent(`"${fingerprint}":["hash",[42,255]]`);
  });

  it('switches directly from text to bytes and then to a list', async () => {
    const user = userEvent.setup();
    render(<Harness field={fingerprint} initial='hash' />);
    const selector = screen.getByRole('combobox', { name: fingerprint });
    await user.click(selector);
    await user.click(await screen.findByRole('option', { name: 'Byte sequence' }));
    expect(selector).toHaveTextContent('Byte sequence');
    await user.click(screen.getByRole('button', { name: 'Add' }));
    fireEvent.change(screen.getByRole('spinbutton'), { target: { value: '42' } });
    expect(screen.getByLabelText('Representation draft')).toHaveTextContent(`"${fingerprint}":[42]`);
    await user.click(selector);
    await user.click(await screen.findByRole('option', { name: 'List' }));
    expect(selector).toHaveTextContent('List');
    expect(screen.getByLabelText('Representation draft')).toHaveTextContent(`"${fingerprint}":[]`);
  });

  it('keeps private key representation choices separate from visible credential values', async () => {
    const user = userEvent.setup();
    const { container } = render(<Harness field='key' initial='private-key' />);
    const selector = screen.getByRole('combobox', { name: 'Private key' });
    expect(selector).toHaveTextContent('Single value');
    expect(screen.queryByRole('spinbutton')).not.toBeInTheDocument();
    expect(screen.getByDisplayValue('private-key')).toHaveAttribute('type', 'text');
    await user.click(selector);
    await user.click(await screen.findByRole('option', { name: 'List' }));
    await user.click(screen.getByRole('button', { name: 'Add' }));
    const input = container.querySelector('input[type="text"]');
    expect(input).not.toBeNull();
    fireEvent.change(input!, { target: { value: 'key-line' } });
    expect(screen.getByLabelText('Representation draft')).toHaveTextContent('"key":["key-line"]');
  });

  it('disables both the representation selector and its value', () => {
    render(<Harness disabled field={fingerprint} initial='hash' />);
    expect(screen.getByRole('combobox')).toBeDisabled();
    expect(screen.getByRole('textbox')).toBeDisabled();
  });
});
