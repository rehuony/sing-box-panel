import type { RJSFSchema } from '@rjsf/utils';

import { useState } from 'react';
import { expect, it } from 'vitest';
import userEvent from '@testing-library/user-event';
import { customizeValidator } from '@rjsf/validator-ajv8';
import { fireEvent, render, screen, within } from '@testing-library/react';

import type { CanonicalDraft } from '@/pages/configuration-page/use-canonical-configuration';

import '@/i18n';
import { ConfigurationSectionEditor } from '@/pages/configuration-page/configuration-section-editor';
import { encodeCanonicalValue, parseCanonicalDraft } from '@/pages/configuration-page/use-canonical-configuration';

const dns: RJSFSchema = {
  type: 'object',
  properties: {
    servers: { type: 'array', items: { type: 'object', properties: { tag: { type: 'string' } } } },
    rules: { type: 'array', items: { type: 'object', properties: { action: { type: 'string' } } } },
    final: { type: 'string' },
  },
};
const root: RJSFSchema = { type: 'object', properties: { dns } };

function Harness() {
  const [draft, setDraft] = useState<CanonicalDraft>({
    dns: { servers: [{ tag: 'remote', future: 42 }], rules: [{ action: 'route' }], final: 'remote', unknown: true },
  });
  return (
    <>
      <ConfigurationSectionEditor disabled={false} draft={draft} name='dns' onChange={setDraft}
        resolution={{ schema: root, createValidator: () => customizeValidator() }} schema={dns} />
      <output aria-label='Draft'>{encodeCanonicalValue(draft)}</output>
    </>
  );
}

it('separates lists from settings without erasing fields in the other tabs', async () => {
  const user = userEvent.setup();
  render(<Harness />);
  expect(screen.getByText('remote')).toBeInTheDocument();
  expect(screen.queryByRole('textbox', { name: 'Default destination' })).not.toBeInTheDocument();
  await user.click(screen.getByRole('tab', { name: 'Resolution & cache' }));
  fireEvent.change(screen.getByRole('textbox', { name: 'Default destination' }), { target: { value: 'local' } });
  await user.click(screen.getByRole('tab', { name: 'Servers' }));
  expect(screen.getByText('remote')).toBeInTheDocument();
  expect(JSON.parse(screen.getByLabelText('Draft').textContent ?? '{}')).toEqual({
    dns: { servers: [{ tag: 'remote', future: 42 }], rules: [{ action: 'route' }], final: 'local', unknown: true },
  });
});

const collectionSchema: RJSFSchema = {
  type: 'array',
  items: {
    type: 'object',
    properties: {
      tag: { type: 'string' },
      type: { type: 'string' },
      counter: { type: 'integer' },
      users: { type: 'array', items: { type: 'object', properties: { name: { type: 'string' } } } },
    },
  },
};

function CollectionHarness({ initial, disabled = false, schema = collectionSchema }: {
  initial: string;
  disabled?: boolean;
  schema?: RJSFSchema;
}) {
  const [draft, setDraft] = useState(() => parseCanonicalDraft(initial));
  return (
    <>
      <ConfigurationSectionEditor name='http_clients' draft={draft} disabled={disabled} onChange={setDraft} schema={schema}
        resolution={{ schema: { type: 'object', properties: { http_clients: schema } }, createValidator: () => customizeValidator() }} />
      <output aria-label='Draft'>{encodeCanonicalValue(draft)}</output>
    </>
  );
}

it('moves standalone records without losing unknown fields or numeric lexemes and keeps nested arrays in their existing layout', async () => {
  const user = userEvent.setup();
  render(<CollectionHarness initial='{"http_clients":[{"tag":"first","counter":900719925474099312345,"future":4.2000e+99,"users":[{"name":"nested"}]},{"tag":"second"}]}' />);
  const table = screen.getByRole('table');
  expect(screen.getByRole('button', { name: 'Move first up' })).toBeDisabled();
  expect(screen.getByRole('button', { name: 'Move second down' })).toBeDisabled();
  screen.getByRole('button', { name: 'Move first down' }).focus();
  await user.keyboard('{Enter}');
  expect(within(table).getAllByRole('row')[1]).toHaveTextContent('second');
  expect(screen.getByLabelText('Draft')).toHaveTextContent('"counter":900719925474099312345');
  expect(screen.getByLabelText('Draft')).toHaveTextContent('"future":4.2000e+99');
  await user.click(screen.getByRole('button', { name: 'Move first up' }));
  expect(within(table).getAllByRole('row')[1]).toHaveTextContent('first');
  await user.click(within(within(table).getAllByRole('row')[1]).getByRole('button', { name: 'Edit' }));
  const dialog = screen.getByRole('dialog');
  expect(within(dialog).queryByRole('table')).not.toBeInTheDocument();
  expect(within(dialog).getByText('nested')).toBeInTheDocument();
  fireEvent.change(within(dialog).getByRole('textbox', { name: 'Tag' }), { target: { value: 'renamed' } });
  await user.click(within(dialog).getByRole('button', { name: 'Save changes' }));
  expect(screen.getByLabelText('Draft')).toHaveTextContent('"tag":"renamed"');
  expect(screen.getByLabelText('Draft')).toHaveTextContent('"counter":900719925474099312345');
  expect(screen.getByLabelText('Draft')).toHaveTextContent('"future":4.2000e+99');
  await user.click(within(within(table).getAllByRole('row')[1]).getByRole('button', { name: 'Remove' }));
  expect(JSON.parse(screen.getByLabelText('Draft').textContent ?? '{}')).toEqual({ http_clients: [{ tag: 'second' }] });
  expect(screen.getByRole('button', { name: 'Move second up' })).toBeDisabled();
  expect(screen.getByRole('button', { name: 'Move second down' })).toBeDisabled();
});

it('respects locked arrays and schema limits in the standalone layout', () => {
  const { rerender } = render(<CollectionHarness disabled initial='{"http_clients":[{"tag":"first"},{"tag":"second"}]}' />);
  for (const button of within(screen.getByRole('table')).getAllByRole('button')) expect(button).toBeDisabled();
  rerender(<CollectionHarness initial='{}' schema={{ ...collectionSchema, maxItems: 2 }} />);
  expect(screen.queryByRole('button', { name: 'Add' })).not.toBeInTheDocument();
});
