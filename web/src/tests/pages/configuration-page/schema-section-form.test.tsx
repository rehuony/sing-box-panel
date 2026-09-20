import type { RJSFSchema } from '@rjsf/utils';

import { useState } from 'react';
import { describe, expect, it } from 'vitest';
import userEvent from '@testing-library/user-event';
import { customizeValidator } from '@rjsf/validator-ajv8';
import { fireEvent, render, screen } from '@testing-library/react';

import type { ReviewedSchemaResolution } from '@/schemas/resolve-reviewed-schema';
import type { CanonicalDraft } from '@/pages/configuration-page/use-canonical-configuration';

import '@/i18n';
import { uiSchemaFromPanel } from '@/pages/configuration-page/schema-ui';
import { SchemaSectionForm } from '@/pages/configuration-page/schema-section-form';
import { encodeCanonicalDraft, parseCanonicalDraft } from '@/pages/configuration-page/use-canonical-configuration';

const sectionSchema: RJSFSchema = {
  type: 'object',
  additionalProperties: false,
  properties: {
    users: {
      type: 'array',
      items: {
        type: 'object',
        additionalProperties: false,
        properties: {
          name: { type: 'string' },
          profile: {
            type: 'object',
            additionalProperties: false,
            properties: { known: { type: 'string' } },
          },
        },
      },
    },
  },
};

const rootSchema: RJSFSchema = {
  type: 'object',
  additionalProperties: false,
  properties: { section: sectionSchema },
};

const resolution: ReviewedSchemaResolution = {
  schema: rootSchema,
  createValidator: () => customizeValidator(),
};

function Harness() {
  const [draft, setDraft] = useState<CanonicalDraft>({
    section: {
      users: [
        {
          name: 'A',
          profile: { future: { owner: 'A' }, known: 'alpha' },
        },
        {
          name: 'B',
          profile: { future: { owner: 'B' }, known: 'beta' },
        },
      ],
    },
  });
  return (
    <>
      <SchemaSectionForm
        basePointer='/section'
        data={draft.section}
        onChange={(change) => setDraft((current) => change(current))}
        resolution={resolution}
        schema={sectionSchema}
      />
      <output aria-label='Canonical draft'>{JSON.stringify(draft)}</output>
    </>
  );
}

const numericSchema: RJSFSchema = {
  type: 'object',
  properties: {
    enabled: { type: 'boolean' },
    listen_port: { type: 'integer' },
    counter: { type: 'integer' },
    timeout: { anyOf: [{ type: 'integer' }, { type: 'string' }] },
    tuning: { type: 'object', properties: { value: { type: 'string' } } },
  },
};

function NumericHarness() {
  const [draft, setDraft] = useState(() => parseCanonicalDraft(
    '{"section":{"enabled":true,"listen_port":2080,"counter":900719925474099312345,"future":4.2000e+99}}',
  ));
  return (
    <>
      <SchemaSectionForm
        basePointer='/section'
        data={draft.section}
        onChange={(change) => setDraft(change)}
        resolution={{ ...resolution, schema: { type: 'object', properties: { section: numericSchema } } }}
        schema={numericSchema}
        uiSchema={uiSchemaFromPanel(numericSchema)}
      />
      <output aria-label='Canonical draft'>{encodeCanonicalDraft(draft)}</output>
    </>
  );
}

describe('schemaSectionForm', () => {
  it('edits array and object union representations without erasing hidden extensions', () => {
    const properties: RJSFSchema = {
      type: 'object',
      properties: {
        ranges: { anyOf: [{ type: 'string' }, { type: 'array', items: { type: 'string' } }] },
        resolver: { anyOf: [{ type: 'string' }, {
          type: 'object', properties: { server: { type: 'string' } },
        }] },
      },
    };
    function UnionHarness() {
      const [data, setData] = useState<CanonicalDraft>({
        section: { ranges: ['443:8443'], resolver: { server: 'dns-one', future: { preserve: true } } },
      });
      return (
        <>
          <SchemaSectionForm basePointer='/section' data={data.section} onChange={setData}
            resolution={{ ...resolution, schema: { type: 'object', properties: { section: properties } } }}
            schema={properties} uiSchema={uiSchemaFromPanel(properties, [], properties, data.section)} />
          <output aria-label='Saved union'>{JSON.stringify(data)}</output>
        </>
      );
    }
    render(<UnionHarness />);
    fireEvent.change(screen.getByDisplayValue('443:8443'), { target: { value: '2053:2096' } });
    fireEvent.change(screen.getByDisplayValue('dns-one'), { target: { value: 'dns-two' } });
    expect(JSON.parse(screen.getByLabelText('Saved union').textContent ?? '{}')).toEqual({
      section: { ranges: ['2053:2096'], resolver: { server: 'dns-two', future: { preserve: true } } },
    });
  });

  it('displays numeric fields and preserves untouched numeric lexemes while editing', async () => {
    const user = userEvent.setup();
    render(<NumericHarness />);
    expect(screen.getByLabelText('Listen port')).toHaveValue(2080);
    expect(screen.getByText('Enabled')).toBeVisible();
    await user.click(screen.getByRole('switch', { name: 'Enabled' }));
    const encoded = screen.getByLabelText('Canonical draft').textContent;
    expect(encoded).toContain('"enabled":false');
    expect(encoded).toContain('"counter":900719925474099312345');
    expect(encoded).toContain('"future":4.2000e+99');
    fireEvent.change(screen.getByLabelText('Listen port'), { target: { value: '2081' } });
    expect(screen.getByLabelText('Canonical draft')).toHaveTextContent('"listen_port":2081');
  });

  it('keeps optional object fields compact until explicitly configured', async () => {
    const user = userEvent.setup();
    render(<NumericHarness />);
    expect(screen.queryByLabelText('value')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Configure' }));
    expect(screen.getByLabelText('value')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Remove settings' }));
    expect(screen.queryByLabelText('value')).not.toBeInTheDocument();
    expect(screen.getByLabelText('Canonical draft')).not.toHaveTextContent('tuning');
  });

  it('names alternative input formats and writes the selected representation', async () => {
    const user = userEvent.setup();
    render(<NumericHarness />);
    await user.click(screen.getByRole('combobox', { name: 'Timeout' }));
    await user.click(await screen.findByRole('option', { name: 'Text' }));
    fireEvent.change(screen.getByRole('textbox', { name: 'Text' }), { target: { value: '5s' } });
    expect(screen.getByLabelText('Canonical draft')).toHaveTextContent('"timeout":"5s"');
  });

  it('assigns unique field IDs when forms are mounted together', () => {
    const { container } = render(
      <>
        <NumericHarness />
        <NumericHarness />
      </>,
    );
    const ids = [...container.querySelectorAll('[id]')].map((element) => element.id);
    expect(new Set(ids).size).toBe(ids.length);
  });

  it('keeps nested unknown fields attached when same-length array members are reordered', async () => {
    const user = userEvent.setup();
    render(<Harness />);

    await user.click(screen.getAllByRole('button', { name: 'Move down' })[0]);

    const encoded = screen.getByLabelText('Canonical draft').textContent ?? '';
    const draft = JSON.parse(encoded) as CanonicalDraft;
    expect(draft.section).toEqual({
      users: [
        {
          name: 'B',
          profile: { future: { owner: 'B' }, known: 'beta' },
        },
        {
          name: 'A',
          profile: { future: { owner: 'A' }, known: 'alpha' },
        },
      ],
    });
  });
});

const serverSchema: RJSFSchema = {
  type: 'array',
  items: {
    oneOf: ['h3', 'https', 'local'].map((type) => ({
      type: 'object',
      required: ['type'],
      properties: {
        type: { type: 'string', enum: [type] },
        tag: { type: 'string' },
        ...(type === 'local' ? {} : { server: { type: 'string' } }),
      },
    })),
  },
};

function ServerHarness() {
  const [draft, setDraft] = useState<CanonicalDraft>({
    servers: [{ type: 'https', tag: 'dns-remote', server: '1.1.1.1', future: { keep: true } }],
  });
  return (
    <>
      <SchemaSectionForm basePointer='/servers' data={draft.servers} onChange={setDraft}
        resolution={{ ...resolution, schema: { type: 'object', properties: { servers: serverSchema } } }} schema={serverSchema} />
      <output aria-label='Server draft'>{JSON.stringify(draft)}</output>
    </>
  );
}

it('edits the matching protocol inline and retains unknown fields across navigation and type changes', async () => {
  const user = userEvent.setup();
  render(<ServerHarness />);
  expect(screen.queryByRole('textbox', { name: 'Server' })).not.toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: 'dns-remote' }));
  expect(screen.getByRole('combobox', { name: 'Type' })).toHaveTextContent('https');
  expect(screen.queryByText(/Option \d/)).not.toBeInTheDocument();
  fireEvent.change(screen.getByRole('textbox', { name: 'Server' }), { target: { value: '9.9.9.9' } });
  await user.click(screen.getByRole('button', { name: 'Done editing' }));
  expect(screen.getByText('9.9.9.9')).toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: 'dns-remote' }));
  await user.click(screen.getByRole('combobox', { name: 'Type' }));
  await user.click(await screen.findByRole('option', { name: 'local' }));
  expect(screen.queryByRole('textbox', { name: 'Server' })).not.toBeInTheDocument();
  expect(JSON.parse(screen.getByLabelText('Server draft').textContent ?? '{}')).toEqual({
    servers: [{ type: 'local', tag: 'dns-remote', future: { keep: true } }],
  });
});

it('adds, renames and removes custom map entries through the same field layout', async () => {
  const user = userEvent.setup();
  const mapSchema: RJSFSchema = { type: 'object', additionalProperties: { type: 'string' } };
  function MapHarness() {
    const [draft, setDraft] = useState<CanonicalDraft>({ section: { 'X-Existing': 'keep' } });
    return (
      <>
        <SchemaSectionForm basePointer='/section' data={draft.section} onChange={setDraft}
          resolution={{ ...resolution, schema: { type: 'object', properties: { section: mapSchema } } }} schema={mapSchema} />
        <output aria-label='Map draft'>{JSON.stringify(draft)}</output>
      </>
    );
  }
  render(<MapHarness />);
  await user.click(screen.getByRole('button', { name: 'Add field' }));
  const key = screen.getAllByRole('textbox', { name: 'Field name' })[1];
  fireEvent.change(key, { target: { value: 'X-New' } });
  fireEvent.blur(key);
  fireEvent.change(screen.getByRole('textbox', { name: 'X-New' }), { target: { value: 'value' } });
  expect(screen.getByLabelText('Map draft')).toHaveTextContent('"X-New":"value"');
  await user.click(screen.getAllByRole('button', { name: 'Remove' })[1]);
  expect(screen.getByLabelText('Map draft')).toHaveTextContent('{"section":{"X-Existing":"keep"}}');
});
