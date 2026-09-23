import type { RJSFSchema } from '@rjsf/utils';

import { useState } from 'react';
import userEvent from '@testing-library/user-event';
import { beforeAll, describe, expect, it } from 'vitest';
import { createPrecompiledValidator } from '@rjsf/validator-ajv8';
import { fireEvent, render, screen, within } from '@testing-library/react';

import type { ReviewedSchemaResolution } from '@/schemas/resolve-reviewed-schema';
import type { CanonicalDraft } from '@/pages/configuration-page/use-canonical-configuration';

import '@/i18n';
import { reviewedSchemaManifest } from '@/schemas/generated';
import { SchemaSectionForm } from '@/pages/configuration-page/schema-section-form';
import { schemaProperties, uiSchemaFromPanel } from '@/pages/configuration-page/schema-ui';
import { encodeCanonicalValue } from '@/pages/configuration-page/use-canonical-configuration';

const reviewedEntry = reviewedSchemaManifest['1.14.0'];
let resolution: ReviewedSchemaResolution;
let schema: RJSFSchema;

beforeAll(async () => {
  const reviewed = await reviewedEntry?.load();
  if (!reviewed) return;
  resolution = {
    schema: reviewed.schema,
    createValidator: (root) => createPrecompiledValidator(reviewed.validateFns as never, root),
  };
  const fields = schemaProperties({ $ref: '#/$defs/DNSRule' }, reviewed.schema, { type: 'default' });
  schema = {
    type: 'object',
    properties: {
      interface_address: fields.interface_address,
      network_interface_address: fields.network_interface_address,
      query_type: fields.query_type,
      fallback_for_alpn: { type: 'object', additionalProperties: { $ref: '#/$defs/ServerOptions' } },
    },
  };
});

function Harness({ initial }: { initial: Record<string, unknown> }) {
  const [draft, setDraft] = useState<CanonicalDraft>({ section: initial });
  return (
    <>
      <SchemaSectionForm basePointer='/section' data={draft.section} onChange={setDraft}
        resolution={resolution} schema={schema}
        uiSchema={uiSchemaFromPanel(schema, [], resolution.schema, draft.section)} />
      <output aria-label='Map draft'>{encodeCanonicalValue(draft.section)}</output>
    </>
  );
}

describe.skipIf(!reviewedEntry)('native map and nested union fields', () => {
  it('creates text values, renames keys and removes entries without object placeholders', async () => {
    const user = userEvent.setup();
    render(<Harness initial={{ interface_address: { eth0: '192.0.2.1' }, future: { keep: true } }} />);
    const map = within(screen.getByRole('group', { name: 'interface_address' }));
    await user.click(map.getByRole('button', { name: 'Add field' }));
    expect(screen.getByLabelText('Map draft')).toHaveTextContent('"newKey":""');
    expect(screen.queryByDisplayValue('[object Object]')).not.toBeInTheDocument();
    expect(screen.queryByText('Option 1')).not.toBeInTheDocument();
    // The collection's header owns one removal control; individual map values have none.
    expect(map.getAllByRole('button', { name: 'Remove settings' })).toHaveLength(1);
    const key = map.getAllByRole('textbox', { name: 'Field name' })[1];
    fireEvent.change(key, { target: { value: 'eth1' } });
    fireEvent.blur(key);
    fireEvent.change(map.getAllByRole('textbox', { name: 'Single value' })[1], { target: { value: '198.51.100.1' } });
    expect(JSON.parse(screen.getByLabelText('Map draft').textContent!)).toEqual({
      interface_address: { eth0: '192.0.2.1', eth1: '198.51.100.1' }, future: { keep: true },
    });
    await user.click(map.getAllByRole('button', { name: 'Remove' })[1]);
    expect(screen.getByLabelText('Map draft')).not.toHaveTextContent('eth1');
  });

  it('edits existing lists and switches new map values between text and list', async () => {
    const user = userEvent.setup();
    render(<Harness initial={{ network_interface_address: { wifi: ['192.0.2.2'] } }} />);
    const map = within(screen.getByRole('group', { name: 'network_interface_address' }));
    expect(map.getByRole('combobox', { name: 'wifi' })).toHaveTextContent('List');
    fireEvent.change(map.getByDisplayValue('192.0.2.2'), { target: { value: '192.0.2.3' } });
    await user.click(map.getByRole('button', { name: 'Add field' }));
    await user.click(map.getByRole('combobox', { name: 'newKey' }));
    await user.click(await screen.findByRole('option', { name: 'List' }));
    await user.click(map.getAllByRole('button', { name: 'Add' })[1]);
    fireEvent.change(map.getByRole('textbox', { name: 'newKey-1' }), { target: { value: '198.51.100.2' } });
    expect(JSON.parse(screen.getByLabelText('Map draft').textContent!)).toEqual({
      network_interface_address: { wifi: ['192.0.2.3'], newKey: ['198.51.100.2'] },
    });
    await user.click(map.getByRole('combobox', { name: 'newKey' }));
    await user.click(await screen.findByRole('option', { name: 'Single value' }));
    expect(map.getByRole('textbox', { name: 'Single value' })).toHaveValue('');
  });

  it('selects and labels nested scalar and list representations from saved data', () => {
    render(<Harness initial={{ query_type: ['A', 28] }} />);
    expect(screen.queryByText('Option 1')).not.toBeInTheDocument();
    expect(screen.getByRole('combobox', { name: 'query_type' })).toHaveTextContent('List');
    expect(screen.getByRole('spinbutton')).toHaveValue(28);
    expect(screen.getAllByRole('combobox').some((input) => input.textContent?.includes('A'))).toBe(true);
  });

  it('adds values to a referenced scalar union list inline', async () => {
    const user = userEvent.setup();
    render(<Harness initial={{ query_type: [] }} />);
    await user.click(screen.getByRole('button', { name: 'Add' }));
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    fireEvent.change(screen.getByRole('spinbutton'), { target: { value: '28' } });
    expect(screen.getByLabelText('Map draft')).toHaveTextContent('"query_type":[28]');
  });

  it('renders referenced object map values directly without duplicate headings or configure controls', async () => {
    const user = userEvent.setup();
    render(<Harness initial={{ fallback_for_alpn: { h2: { server: 'localhost', server_port: 443, future: true } } }} />);
    const map = within(screen.getByRole('group', { name: 'fallback_for_alpn' }));
    expect(map.queryByText('h2')).not.toBeInTheDocument();
    expect(map.queryByRole('button', { name: 'Configure' })).not.toBeInTheDocument();
    expect(map.getAllByRole('button', { name: 'Remove settings' })).toHaveLength(1);
    fireEvent.change(map.getByDisplayValue('localhost'), { target: { value: '127.0.0.1' } });
    await user.click(map.getByRole('button', { name: 'Add field' }));
    expect(map.getAllByRole('textbox', { name: 'Server' })).toHaveLength(2);
    expect(JSON.parse(screen.getByLabelText('Map draft').textContent!)).toEqual({
      fallback_for_alpn: { h2: { server: '127.0.0.1', server_port: 443, future: true }, newKey: {} },
    });
  });
});
