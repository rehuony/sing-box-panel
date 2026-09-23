import type { RJSFSchema } from '@rjsf/utils';

import { useState } from 'react';
import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import { fireEvent, render, screen, within } from '@testing-library/react';
import { createPrecompiledValidator, customizeValidator } from '@rjsf/validator-ajv8';

import '@/i18n';
import { TooltipProvider } from '@/components/ui/tooltip';
import { reviewedSchemaManifest } from '@/schemas/generated';
import { resolvedSchema } from '@/pages/configuration-page/schema-ui';
import { SchemaDialogLayout } from '@/pages/configuration-page/rjsf-shadcn-theme';
import { SchemaSectionForm } from '@/pages/configuration-page/schema-section-form';
import { encodeCanonicalDraft, parseCanonicalDraft } from '@/pages/configuration-page/use-canonical-configuration';

const schema: RJSFSchema = {
  type: 'object', properties: {
    tag: { type: 'string' }, server: { type: 'string' },
    connect_timeout: { type: 'string' },
    tls: { type: 'object', properties: { enabled: { type: 'boolean' }, server_name: { type: 'string' } } },
  },
};

describe('schema dialog layout', () => {
  it.each([['username', 'password'], ['Username', 'Password']])('orders %s before %s and leaves the empty collection unchanged when cancelled', async (usernameKey, passwordKey) => {
    const user = userEvent.setup();
    const authentication: RJSFSchema = {
      type: 'object', properties: {
        tag: { type: 'string' },
        users: { type: 'array', items: { type: 'object', properties: {
          [passwordKey]: { type: 'string' },
          [usernameKey]: { type: 'string' },
        } } },
      },
    };
    const onChange = vi.fn();
    render(
      <SchemaSectionForm dialogLayout schema={authentication} basePointer='/entry' data={{ users: [] }} onChange={onChange}
        resolution={{ schema: authentication, createValidator: () => customizeValidator() }} />,
    );
    await user.click(screen.getByRole('tab', { name: 'Authentication' }));
    expect(screen.getByText('No entries yet')).toBeVisible();
    await user.click(screen.getByRole('button', { name: 'Add' }));
    const dialog = within(screen.getByRole('dialog'));
    const username = dialog.getByLabelText('Username');
    const password = dialog.getByLabelText('Password');
    expect(username.compareDocumentPosition(password) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    fireEvent.change(username, { target: { value: 'pending-user' } });
    await user.click(dialog.getByRole('button', { name: 'Cancel' }));
    expect(onChange).not.toHaveBeenCalled();
    expect(screen.getByText('No entries yet')).toBeVisible();
  });

  it.each(['tls', 'transport'])('replaces Configure in the %s header and restores it after removal', async (field) => {
    const user = userEvent.setup();
    const optionalSchema: RJSFSchema = {
      type: 'object', properties: { [field]: { type: 'object', properties: { enabled: { type: 'boolean' } } } },
    };
    function Harness() {
      const [draft, setDraft] = useState(() => parseCanonicalDraft('{"entry":{}}'));
      return (
        <SchemaSectionForm schema={optionalSchema} basePointer='/entry' data={draft.entry} onChange={setDraft}
          resolution={{ schema: optionalSchema, createValidator: () => customizeValidator() }} />
      );
    }
    render(<Harness />);
    const configure = screen.getByRole('button', { name: 'Configure' });
    const header = configure.parentElement;
    await user.click(configure);
    const remove = screen.getByRole('button', { name: 'Remove settings' });
    expect(remove.parentElement).toBe(header);
    expect(remove).toHaveAttribute('data-variant', 'destructive');
    expect(remove.compareDocumentPosition(screen.getByRole('switch', { name: 'Enabled' })) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    await user.click(remove);
    expect(screen.queryByRole('switch', { name: 'Enabled' })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Configure' }).parentElement).toBe(header);
  });

  it('retains edits and optional-object state across keyboard tab changes without altering untouched values', async () => {
    const user = userEvent.setup();
    const root: RJSFSchema = { type: 'object', properties: { entry: schema } };
    function Harness() {
      const [draft, setDraft] = useState(() => parseCanonicalDraft('{"entry":{"tag":"one","server":"host","connect_timeout":"5s","future":900719925474099312345}}'));
      return (
        <>
          <SchemaSectionForm dialogLayout schema={schema} basePointer='/entry' data={draft.entry} onChange={setDraft}
            resolution={{ schema: root, createValidator: () => customizeValidator() }} />
          <output aria-label='Draft'>{encodeCanonicalDraft(draft)}</output>
        </>
      );
    }
    render(<Harness />);
    expect(screen.getByRole('tablist', { name: 'Entry settings' })).toHaveAttribute('aria-orientation', 'vertical');
    const original = screen.getByLabelText('Draft').textContent;
    expect(screen.queryByRole('textbox', { name: 'Connection timeout' })).not.toBeInTheDocument();
    await user.click(screen.getByRole('tab', { name: 'TLS' }));
    expect(screen.getByLabelText('Draft').textContent).toBe(original);
    await user.click(screen.getByRole('button', { name: 'Configure' }));
    expect(screen.getByRole('textbox', { name: 'Server name' })).toBeVisible();
    const configured = screen.getByLabelText('Draft').textContent;
    await user.click(screen.getByRole('tab', { name: 'Connection options' }));
    expect(screen.getByLabelText('Draft').textContent).toBe(configured);
    fireEvent.change(screen.getByRole('textbox', { name: 'Connection timeout' }), { target: { value: '10s' } });
    await user.click(screen.getByRole('tab', { name: 'TLS' }));
    expect(screen.getByRole('textbox', { name: 'Server name' })).toBeVisible();
    fireEvent.change(screen.getByRole('textbox', { name: 'Server name' }), { target: { value: 'tls.example' } });
    await user.click(screen.getByRole('tab', { name: 'Basic settings' }));
    await user.keyboard('{ArrowDown}{Enter}');
    expect(screen.getByRole('tab', { name: 'TLS' })).toHaveAttribute('aria-selected', 'true');
    await user.keyboard('{End}{Enter}');
    expect(screen.getByRole('tab', { name: 'Connection options' })).toHaveAttribute('aria-selected', 'true');
    expect(screen.getByRole('textbox', { name: 'Connection timeout' })).toHaveValue('10s');
    expect(screen.getByLabelText('Draft')).toHaveTextContent('"server_name":"tls.example"');
    expect(screen.getByLabelText('Draft')).toHaveTextContent('900719925474099312345');
  });

  it('returns to basic settings when the selected protocol no longer has the active group', async () => {
    const user = userEvent.setup();
    const { rerender } = render(<SchemaDialogLayout schema={schema} data={{}}><span>Fields</span></SchemaDialogLayout>);
    await user.click(screen.getByRole('tab', { name: 'TLS' }));
    rerender(<SchemaDialogLayout schema={{ type: 'object', properties: { tag: { type: 'string' } } }} data={{}}><span>Fields</span></SchemaDialogLayout>);
    expect(screen.queryByRole('tab', { name: 'TLS' })).not.toBeInTheDocument();
    expect(screen.queryByRole('tablist')).not.toBeInTheDocument();
    expect(screen.getByRole('tabpanel', { name: 'Basic settings' })).toBeVisible();
  });

  it('uses the same section navigation for added array entries and discards pending fields on cancel', async () => {
    const user = userEvent.setup();
    const array: RJSFSchema = { type: 'array', items: schema };
    const root: RJSFSchema = { type: 'object', properties: { entries: array } };
    const onChange = vi.fn();
    render(
      <SchemaSectionForm schema={array} basePointer='/entries' data={[]} onChange={onChange}
        resolution={{ schema: root, createValidator: () => customizeValidator() }} />,
    );
    await user.click(screen.getByRole('button', { name: 'Add' }));
    const dialog = within(screen.getByRole('dialog'));
    expect(dialog.getByRole('tablist')).toHaveAttribute('aria-orientation', 'vertical');
    fireEvent.change(dialog.getByRole('textbox', { name: 'Server' }), { target: { value: 'pending-host' } });
    await user.click(dialog.getByRole('tab', { name: 'Connection options' }));
    fireEvent.change(dialog.getByRole('textbox', { name: 'Connection timeout' }), { target: { value: '10s' } });
    await user.click(dialog.getByRole('tab', { name: 'Basic settings' }));
    expect(dialog.getByRole('textbox', { name: 'Server' })).toHaveValue('pending-host');
    await user.click(dialog.getByRole('button', { name: 'Cancel' }));
    expect(onChange).not.toHaveBeenCalled();
  });

  it('confirms edits from multiple groups together while preserving unknown fields and lossless numbers', async () => {
    const user = userEvent.setup();
    const array: RJSFSchema = { type: 'array', items: schema };
    const root: RJSFSchema = { type: 'object', properties: { entries: array } };
    function Harness() {
      const [draft, setDraft] = useState(() => parseCanonicalDraft('{"entries":[{"tag":"one","server":"host","connect_timeout":"5s","future":900719925474099312345}]}'));
      return (
        <>
          <SchemaSectionForm schema={array} basePointer='/entries' data={draft.entries} onChange={setDraft}
            resolution={{ schema: root, createValidator: () => customizeValidator() }} />
          <output aria-label='Draft'>{encodeCanonicalDraft(draft)}</output>
        </>
      );
    }
    render(<Harness />);
    const original = screen.getByLabelText('Draft').textContent;
    await user.click(screen.getByRole('button', { name: 'Edit' }));
    const dialog = within(screen.getByRole('dialog'));
    fireEvent.change(dialog.getByRole('textbox', { name: 'Server' }), { target: { value: 'new-host' } });
    await user.click(dialog.getByRole('tab', { name: 'Connection options' }));
    fireEvent.change(dialog.getByRole('textbox', { name: 'Connection timeout' }), { target: { value: '15s' } });
    expect(screen.getByLabelText('Draft').textContent).toBe(original);
    await user.click(dialog.getByRole('button', { name: 'Save changes' }));
    expect(screen.getByLabelText('Draft')).toHaveTextContent('"server":"new-host"');
    expect(screen.getByLabelText('Draft')).toHaveTextContent('"connect_timeout":"15s"');
    expect(screen.getByLabelText('Draft')).toHaveTextContent('900719925474099312345');
  });

  it.each(['1.13.19', '1.14.0'])('separates matching fields and actions in %s rule dialogs without changing the parent draft on cancel', async (version) => {
    const reviewed = await reviewedSchemaManifest[version].load();
    const user = userEvent.setup();
    const route = resolvedSchema(reviewed.schema.properties?.route as RJSFSchema, reviewed.schema);
    const rules = route.properties?.rules as RJSFSchema;
    const onChange = vi.fn();
    render(
      <TooltipProvider>
        <SchemaSectionForm schema={rules} basePointer='/route/rules'
          data={[{ type: 'default', action: 'route', domain: ['example.com'], domain_regex: ['.*test'], outbound: 'direct' }]}
          onChange={onChange} resolution={{ schema: reviewed.schema,
            createValidator: (schema) => createPrecompiledValidator(reviewed.validateFns as never, schema) }} />
      </TooltipProvider>,
    );
    await user.click(screen.getByRole('button', { name: 'Edit' }));
    const dialog = within(screen.getByRole('dialog'));
    expect(dialog.getByRole('tab', { name: 'Match conditions' })).toHaveAttribute('aria-selected', 'true');
    expect(dialog.getByDisplayValue('example.com')).toBeVisible();
    expect(dialog.queryByRole('textbox', { name: 'Outbound' })).not.toBeInTheDocument();
    await user.click(dialog.getByRole('tab', { name: 'More conditions' }));
    expect(dialog.getByDisplayValue('.*test')).toBeVisible();
    await user.click(dialog.getByRole('tab', { name: 'Action' }));
    expect(dialog.queryByRole('combobox', { name: 'Domain' })).not.toBeInTheDocument();
    expect(dialog.getByRole('textbox', { name: 'Outbound' })).toHaveValue('direct');
    fireEvent.change(dialog.getByRole('textbox', { name: 'Outbound' }), { target: { value: 'proxy' } });
    await user.click(dialog.getByRole('tab', { name: 'Match conditions' }));
    await user.click(dialog.getByRole('button', { name: 'Cancel' }));
    expect(onChange).not.toHaveBeenCalled();
  });
});
