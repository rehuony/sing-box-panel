import type { RJSFSchema } from '@rjsf/utils';

import { useState } from 'react';
import userEvent from '@testing-library/user-event';
import { customizeValidator } from '@rjsf/validator-ajv8';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';

import type { ReviewedSchemaResolution } from '@/schemas/resolve-reviewed-schema';
import type { CanonicalDraft } from '@/pages/configuration-page/use-canonical-configuration';

import i18n from '@/i18n';
import { TooltipProvider } from '@/components/ui/tooltip';
import { uiSchemaFromPanel } from '@/pages/configuration-page/schema-ui';
import { SchemaSectionForm } from '@/pages/configuration-page/schema-section-form';
import { encodeCanonicalDraft, parseCanonicalDraft } from '@/pages/configuration-page/use-canonical-configuration';

afterEach(() => i18n.changeLanguage('en'));

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
  it('localizes scalar, list and optional-object help without translating map keys or changing the draft', async () => {
    await i18n.changeLanguage('zh-CN');
    const user = userEvent.setup();
    const onChange = vi.fn();
    const schema: RJSFSchema = {
      type: 'object',
      properties: {
        final: { 'type': 'string', 'x-panel': { label: { en: 'Final' } } } as RJSFSchema,
        disable_cache: { type: 'boolean' },
        rules: { type: 'array', items: { type: 'string' } },
        optimistic: { type: 'object', properties: { timeout: { type: 'string' } } },
        headers: { type: 'object', additionalProperties: { type: 'string' } },
      },
    };
    const data = { final: 'dns-local', disable_cache: false, rules: [], headers: { server: 'custom-header' } };
    render(
      <TooltipProvider delay={0}>
        <SchemaSectionForm basePointer='/dns' data={data} onChange={onChange}
          resolution={{ ...resolution, schema: { type: 'object', properties: { dns: schema } } }} schema={schema} />
      </TooltipProvider>,
    );
    expect(screen.getByRole('textbox', { name: '默认 DNS 服务器' })).toHaveValue('dns-local');
    expect(screen.getByRole('switch', { name: '禁用 DNS 缓存' })).not.toBeChecked();
    expect(screen.getByRole('textbox', { name: '字段名称' })).toHaveValue('server');
    const help = screen.getByRole('button', { name: '默认 DNS 服务器的说明' });
    await user.hover(help);
    const popup = await screen.findByRole('tooltip');
    expect(popup.querySelector('code')).toBeNull();
    expect(popup).toHaveTextContent('第一个 DNS 服务器');
    await user.unhover(help);
    await waitFor(() => expect(screen.queryByText(/DNS 规则未指定服务器时使用/)).not.toBeInTheDocument());
    await user.click(screen.getByRole('textbox', { name: '默认 DNS 服务器' }));
    await user.tab({ shift: true });
    expect(help).toHaveFocus();
    await user.keyboard('{Enter}');
    expect(await screen.findByText(/DNS 规则未指定服务器时使用/)).toBeVisible();
    await user.keyboard('{Escape}');
    expect(screen.getByRole('button', { name: '规则列表的说明' })).toBeEnabled();
    expect(screen.getByRole('button', { name: '乐观 DNS 缓存的说明' })).toBeEnabled();
    expect(onChange).not.toHaveBeenCalled();
    await act(() => i18n.changeLanguage('en'));
    expect(screen.getByRole('textbox', { name: 'Final' })).toHaveValue('dns-local');
    expect(screen.queryByRole('button', { name: '默认 DNS 服务器的说明' })).not.toBeInTheDocument();
    expect(onChange).not.toHaveBeenCalled();
  });

  it('shows field descriptions beside labels on hover and keyboard activation without changing values', async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    const schema: RJSFSchema = {
      type: 'object',
      properties: {
        interval: { type: 'string', description: 'A Go duration such as 300ms or 5s.' },
        enabled: { type: 'boolean', description: 'Synchronize the system clock.' },
        server: { type: 'string' },
      },
    };
    const { container, rerender } = render(
      <TooltipProvider delay={0}>
        <SchemaSectionForm basePointer='/section' data={{ interval: '5s', enabled: false }} onChange={onChange}
          resolution={{ ...resolution, schema: { type: 'object', properties: { section: schema } } }} schema={schema} />
      </TooltipProvider>,
    );
    expect(screen.queryByText('A Go duration such as 300ms or 5s.')).not.toBeInTheDocument();
    expect(container.querySelector('[data-slot="field-description"]')).toBeNull();
    expect(screen.queryByRole('button', { name: 'Help for Server' })).not.toBeInTheDocument();

    const intervalHelp = screen.getByRole('button', { name: 'Help for Sync interval' });
    expect(intervalHelp.parentElement).toHaveTextContent('Sync interval');
    await user.hover(intervalHelp);
    expect(await screen.findByText('A Go duration such as 300ms or 5s.')).toBeVisible();
    await user.unhover(intervalHelp);
    await waitFor(() => expect(screen.queryByText('A Go duration such as 300ms or 5s.')).not.toBeInTheDocument());
    await user.click(screen.getByRole('textbox', { name: 'Sync interval' }));
    await user.tab({ shift: true });
    expect(intervalHelp).toHaveFocus();
    await user.keyboard('{Enter}');
    expect(await screen.findByText('A Go duration such as 300ms or 5s.')).toBeVisible();
    await user.keyboard('{Escape}');
    await waitFor(() => expect(screen.queryByText('A Go duration such as 300ms or 5s.')).not.toBeInTheDocument());

    await user.click(screen.getByRole('button', { name: 'Help for Enabled' }));
    expect(screen.getByRole('switch', { name: 'Enabled' })).not.toBeChecked();
    expect(screen.getByRole('textbox', { name: 'Sync interval' })).toHaveValue('5s');
    expect(onChange).not.toHaveBeenCalled();

    rerender(
      <TooltipProvider delay={0}>
        <SchemaSectionForm basePointer='/section' data={{ interval: '5s' }} disabled onChange={onChange}
          resolution={{ ...resolution, schema: { type: 'object', properties: { section: schema } } }} schema={schema} />
      </TooltipProvider>,
    );
    expect(screen.getByRole('textbox', { name: 'Sync interval' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Help for Sync interval' })).toBeEnabled();
  });

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

it('only edits through the dialog and retains unknown fields across confirmed type changes', async () => {
  const user = userEvent.setup();
  render(<ServerHarness />);
  expect(screen.queryByRole('textbox', { name: 'Server' })).not.toBeInTheDocument();
  await user.click(screen.getByText('dns-remote'));
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'dns-remote' })).not.toBeInTheDocument();
  const original = screen.getByLabelText('Server draft').textContent;
  const edit = screen.getByRole('button', { name: 'Edit' });
  await user.click(edit);
  expect(screen.getByRole('combobox', { name: 'Type' })).toHaveTextContent('https');
  expect(screen.queryByText(/Option \d/)).not.toBeInTheDocument();
  fireEvent.change(screen.getByRole('textbox', { name: 'Server' }), { target: { value: '9.9.9.9' } });
  expect(screen.getByLabelText('Server draft').textContent).toBe(original);
  await user.click(screen.getByRole('button', { name: 'Cancel' }));
  expect(screen.getByLabelText('Server draft').textContent).toBe(original);
  expect(edit).toHaveFocus();
  await user.click(edit);
  expect(screen.getByRole('textbox', { name: 'Server' })).toHaveValue('1.1.1.1');
  fireEvent.change(screen.getByRole('textbox', { name: 'Server' }), { target: { value: '9.9.9.9' } });
  await user.click(screen.getByRole('button', { name: 'Save changes' }));
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  expect(screen.getByText('9.9.9.9')).toBeInTheDocument();
  await user.click(edit);
  await user.click(screen.getByRole('combobox', { name: 'Type' }));
  await user.click(await screen.findByRole('option', { name: 'local' }));
  expect(screen.queryByRole('textbox', { name: 'Server' })).not.toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: 'Save changes' }));
  expect(JSON.parse(screen.getByLabelText('Server draft').textContent ?? '{}')).toEqual({
    servers: [{ type: 'local', tag: 'dns-remote', future: { keep: true } }],
  });
});

it('keeps the edit form and title mounted until the closing animation finishes', async () => {
  const user = userEvent.setup();
  render(<ServerHarness />);
  await user.click(screen.getByRole('button', { name: 'Edit' }));
  const dialog = screen.getByRole('dialog', { name: 'Edit entry' });
  let finish!: () => void;
  const finished = new Promise<void>((resolve) => {
    finish = resolve;
  });
  Object.defineProperty(dialog, 'getAnimations', { value: () => [{ finished }] });
  await user.click(within(dialog).getByRole('button', { name: 'Cancel' }));
  expect(dialog).toHaveAttribute('data-closed');
  expect(within(dialog).getByRole('heading', { name: 'Edit entry' })).toBeInTheDocument();
  expect(within(dialog).getByRole('textbox', { name: 'Server' })).toHaveValue('1.1.1.1');
  await act(async () => {
    finish();
  });
  await waitFor(() => expect(dialog).not.toBeInTheDocument());
});

it('stages new collection entries in a dialog and leaves the list unchanged on cancel or Escape', async () => {
  const user = userEvent.setup();
  render(<ServerHarness />);
  const original = screen.getByLabelText('Server draft').textContent;
  const add = screen.getByRole('button', { name: 'Add' });
  await user.click(add);
  let dialog = screen.getByRole('dialog', { name: 'Add entry' });
  fireEvent.change(within(dialog).getByRole('textbox', { name: 'Tag' }), { target: { value: 'cancelled' } });
  expect(screen.getByLabelText('Server draft').textContent).toBe(original);
  await user.click(within(dialog).getByRole('button', { name: 'Cancel' }));
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  expect(screen.getByLabelText('Server draft').textContent).toBe(original);
  expect(add).toHaveFocus();

  await user.click(add);
  await user.keyboard('{Escape}');
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  expect(screen.getByLabelText('Server draft').textContent).toBe(original);

  await user.click(add);
  dialog = screen.getByRole('dialog');
  fireEvent.change(within(dialog).getByRole('textbox', { name: 'Tag' }), { target: { value: 'dns-new' } });
  await user.click(within(dialog).getByRole('combobox', { name: 'Type' }));
  await user.click(await screen.findByRole('option', { name: 'https' }));
  fireEvent.change(within(dialog).getByRole('textbox', { name: 'Server' }), { target: { value: '9.9.9.9' } });
  await user.click(within(dialog).getByRole('button', { name: 'Add' }));
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Back' })).not.toBeInTheDocument();
  expect(JSON.parse(screen.getByLabelText('Server draft').textContent ?? '{}')).toEqual({
    servers: [
      { type: 'https', tag: 'dns-remote', server: '1.1.1.1', future: { keep: true } },
      { type: 'https', tag: 'dns-new', server: '9.9.9.9' },
    ],
  });
  expect(screen.getByText('dns-new')).toBeInTheDocument();
});

it('keeps nested additions inside the parent dialog until the parent is confirmed', async () => {
  const user = userEvent.setup();
  const nestedSchema: RJSFSchema = {
    type: 'array', items: { type: 'object', properties: {
      name: { type: 'string' },
      users: { type: 'array', items: { type: 'object', properties: { name: { type: 'string' } } } },
    } },
  };
  function NestedHarness() {
    const [draft, setDraft] = useState<CanonicalDraft>({ section: [] });
    return (
      <>
        <SchemaSectionForm basePointer='/section' data={draft.section} onChange={setDraft}
          schema={nestedSchema} resolution={{ ...resolution, schema: { type: 'object', properties: { section: nestedSchema } } }} />
        <output aria-label='Nested draft'>{JSON.stringify(draft)}</output>
      </>
    );
  }
  render(<NestedHarness />);
  await user.click(screen.getByRole('button', { name: 'Add' }));
  let parent = screen.getByRole('dialog');
  fireEvent.change(within(parent).getByRole('textbox', { name: 'Name' }), { target: { value: 'parent' } });
  await user.click(within(parent).getByRole('tab', { name: 'Authentication' }));
  await user.click(within(parent).getAllByRole('button', { name: 'Add' })[0]);
  const child = screen.getByRole('dialog');
  fireEvent.change(within(child).getByRole('textbox', { name: 'Name' }), { target: { value: 'child' } });
  const ids = [...document.querySelectorAll('[id]')].map((element) => element.id);
  expect(new Set(ids).size).toBe(ids.length);
  await user.click(within(child).getByRole('button', { name: 'Add' }));
  expect(screen.getByLabelText('Nested draft')).toHaveTextContent('{"section":[]}');
  parent = screen.getByRole('dialog');
  expect(within(parent).getByText('child')).toBeInTheDocument();
  await user.click(within(parent).getAllByRole('button', { name: 'Add' }).at(-1)!);
  expect(JSON.parse(screen.getByLabelText('Nested draft').textContent ?? '{}')).toEqual({
    section: [{ name: 'parent', users: [{ name: 'child' }] }],
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
