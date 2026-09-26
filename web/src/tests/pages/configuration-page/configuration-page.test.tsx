import { EditorView } from '@codemirror/view';
import userEvent from '@testing-library/user-event';
import { Link, Route, Routes } from 'react-router-dom';
import { createPrecompiledValidator } from '@rjsf/validator-ajv8';
import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';

import type { ApiClient, ConfigurationCompile, ConfigurationFile, ConfigurationSchemaContract } from '@/api/api-client';

import { ApiRequestError } from '@/api/api-client';
import '@/i18n';
import { toast } from '@/components/ui/toast-manager';
import { TooltipProvider } from '@/components/ui/tooltip';
import { ApiClientProvider } from '@/api/api-client-context';
import { reviewedSchemaManifest } from '@/schemas/generated';
import { TestRouter as MemoryRouter } from '@/tests/test-router';
import { ControlPlaneProvider } from '@/stores/control-plane-provider';
import { representativeSchemaVersions } from '@/tests/schemas/schema-fixtures';
import { ConfigurationPage } from '@/pages/configuration-page/configuration-page';
import { visibleStructuredConfiguration } from '@/pages/configuration-page/structured-validation';
import {
  createMockApiClient,
  testArtifacts,
  testDashboardContext,
  testRevision,
  testStartupArtifact,
} from '@/tests/api/mock-api-client';

const structuredVersions = representativeSchemaVersions('native');
const toastAdd = vi.spyOn(toast, 'add');
const savedFile: ConfigurationFile = {
  revision: 1, content: testRevision.document_json, syntax_valid: true, canonical_revision_id: testRevision.id,
};

function editorView(element: HTMLElement) {
  return EditorView.findFromDOM(element)!;
}
function changeEditor(element: HTMLElement, text: string) {
  const view = editorView(element);
  act(() => view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: text } }));
}

function renderPage(client: ApiClient, initialEntry = '/configuration') {
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <ApiClientProvider client={client}>
        <TooltipProvider delay={0}>
          <ControlPlaneProvider><ConfigurationPage /></ControlPlaneProvider>
        </TooltipProvider>
      </ApiClientProvider>
    </MemoryRouter>,
  );
}

async function renderReadyPage(client: ApiClient) {
  const page = renderPage(client);
  // Schema loading can exceed the default query timeout on shared CI runners.
  await screen.findByRole('region', { name: 'Configuration', busy: false }, { timeout: 5_000 });
  return page;
}

async function createStructuredClient(overrides: Partial<ApiClient> = {}, exactVersion = '1.14.0') {
  const reviewed = await reviewedSchemaManifest[exactVersion].load();
  const artifact = {
    ...testArtifacts.items[0], id: 'core_114', exact_version: exactVersion, reported_version: exactVersion,
  };
  return createMockApiClient({
    getDashboardContext: vi.fn().mockResolvedValue({ ...testDashboardContext, view: { exactVersion } }),
    getConfigurationSchema: vi.fn().mockResolvedValue({
      exact_version: exactVersion, schema_sha256: reviewed.schemaSHA256, schema: reviewed.schema,
    }),
    listCoreArtifacts: vi.fn().mockResolvedValue({ items: [artifact] }),
    ...overrides,
  });
}

beforeAll(async () => {
  // Keep the large generated validators' first import out of interaction test timeouts.
  await Promise.all(structuredVersions.map(version => reviewedSchemaManifest[version].load()));
});

beforeEach(() => {
  toastAdd.mockClear();
});

describe('configurationPage', () => {
  it.each(structuredVersions)('saves and reloads schema-valid Hysteria2 form edits for %s', async (version) => {
    const user = userEvent.setup();
    let file = { ...savedFile, content: JSON.stringify({ inbounds: [{
      type: 'hysteria2', tag: 'hy2', listen: '127.0.0.1', listen_port: 18053,
      users: [{ name: 'test', password: 'test-user-password' }],
      tls: { enabled: true, certificate_path: '/tmp/test-cert.pem', key_path: '/tmp/test-key.pem' },
    }] }) };
    const client = await createStructuredClient({
      getConfigurationFile: vi.fn(async () => file),
      saveConfigurationFile: vi.fn(async (input) => {
        file = { ...file, content: input.content, revision: input.revision + 1 };
        return file;
      }),
    }, version);
    const page = await renderReadyPage(client);
    await user.click(screen.getByRole('tab', { name: 'Inbounds' }));
    await user.click(screen.getByRole('button', { name: 'Edit' }));
    let dialog = within(screen.getByRole('dialog'));
    await user.click(dialog.getByRole('tab', { name: 'Transport' }));
    await user.click(dialog.getByRole('button', { name: 'Generate random Password' }));
    const password = (dialog.getByRole('textbox', { name: 'Password' }) as HTMLInputElement).value;
    expect(password).not.toBe('');
    await user.click(dialog.getByRole('button', { name: 'Save changes' }));
    await user.click(screen.getByRole('button', { name: 'Save configuration' }));
    await waitFor(() => expect(client.saveConfigurationFile).toHaveBeenCalledTimes(1));
    const saved = JSON.parse(file.content);
    expect(saved.inbounds[0].obfs).toEqual({ type: 'salamander', password });
    const reviewed = await reviewedSchemaManifest[version].load();
    const validator = createPrecompiledValidator(reviewed.validateFns as never, reviewed.schema);
    expect(validator.validateFormData(saved, reviewed.schema).errors).toEqual([]);
    page.unmount();
    await renderReadyPage(client);
    await user.click(screen.getByRole('tab', { name: 'Inbounds' }));
    await user.click(screen.getByRole('button', { name: 'Edit' }));
    dialog = within(screen.getByRole('dialog'));
    await user.click(dialog.getByRole('tab', { name: 'Transport' }));
    expect(dialog.getByRole('textbox', { name: 'Password' })).toHaveValue(password);
  });

  it('explains an unsupported visual editor on hover and keyboard focus without an inline notice', async () => {
    const user = userEvent.setup();
    const unsupported = {
      ...testArtifacts.items[0], exact_version: '1.14.3', reported_version: '1.14.3',
    };
    const client = createMockApiClient({
      listCoreArtifacts: vi.fn().mockResolvedValue({ items: [unsupported] }),
    });
    renderPage(client);

    await waitFor(() => {
      const currentTab = screen.getByRole('tab', { name: 'Visual editor' });
      expect(currentTab).toHaveAttribute('aria-disabled', 'true');
      expect(currentTab.parentElement).toHaveAttribute('tabindex', '0');
    });
    const visualTab = screen.getByRole('tab', { name: 'Visual editor' });
    const trigger = visualTab.parentElement!;
    expect(screen.queryByText(/No visual editor schema for 1\.14\.3/)).not.toBeInTheDocument();

    await user.hover(trigger);
    expect(await screen.findByText(/No visual editor schema for 1\.14\.3/)).toBeVisible();
    await user.unhover(trigger);
    await waitFor(() => expect(screen.queryByText(/No visual editor schema for 1\.14\.3/)).not.toBeInTheDocument());

    trigger.focus();
    expect(await screen.findByText(/No visual editor schema for 1\.14\.3/)).toBeVisible();
  });

  it('guides users to version management without blocking Advanced JSON when no core is installed', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient({
      listCoreArtifacts: vi.fn().mockResolvedValue({ items: [] }),
      getConfigurationFile: vi.fn().mockResolvedValue({ revision: 0, content: '{}', syntax_valid: true }),
    });
    renderPage(client);
    expect(await screen.findByRole('heading', { name: 'No core versions installed' })).toBeVisible();
    expect(screen.getByRole('tab', { name: 'Visual editor' })).toHaveAttribute('aria-selected', 'true');
    expect(screen.getByRole('combobox', { name: 'Configuration version' })).toBeDisabled();
    expect(screen.getByRole('combobox', { name: 'Configuration version' })).toHaveTextContent('None');
    expect(within(screen.getByRole('combobox', { name: 'Configuration version' })).getByText('Configuration version')).toBeVisible();
    expect(screen.getByRole('link', { name: 'Go to version management' })).toHaveAttribute('href', '/cores#cores-catalog');
    expect(screen.getByRole('button', { name: 'Validate configuration' })).toBeDisabled();
    expect(client.getConfigurationSchema).not.toHaveBeenCalled();
    await user.click(screen.getByRole('tab', { name: 'Advanced JSON' }));
    // Let the real lazy editor load before starting the DOM assertion's timeout.
    await act(() => vi.dynamicImportSettled());
    const editor = await screen.findByLabelText('sing-box configuration JSON');
    changeEditor(editor, '{"log":{"level":"debug"}}');
    await user.click(screen.getByRole('button', { name: 'Save configuration' }));
    await waitFor(() => expect(client.saveConfigurationFile).toHaveBeenCalledWith({
      revision: 0, content: '{"log":{"level":"debug"}}',
    }));
  });

  it('synchronizes visual and JSON edits and saves unknown values losslessly without navigation prompts', async () => {
    const user = userEvent.setup();
    const client = await createStructuredClient({
      getConfigurationFile: vi.fn().mockResolvedValue({
        ...savedFile, content: '{"dns":{},"experimental":{"large":4.2000e+99},"log":{"level":"info"}}',
      }),
    });
    await renderReadyPage(client);

    expect(screen.getByRole('tab', { name: 'Visual editor' })).toHaveAttribute('aria-selected', 'true');
    const level = screen.getByRole('combobox', { name: 'Log level' });
    await user.click(level);
    await user.click(await screen.findByRole('option', { name: 'debug' }));
    await user.click(screen.getByRole('tab', { name: 'DNS' }));
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();

    await user.click(screen.getByRole('tab', { name: 'Advanced JSON' }));
    const editor = await screen.findByLabelText('sing-box configuration JSON');
    expect(JSON.parse(editorView(editor).state.doc.toString()).log.level).toBe('debug');
    expect(editorView(editor).state.doc.toString()).toContain('4.2000e+99');
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();

    changeEditor(editor, editorView(editor).state.doc.toString().replace('"debug"', '"warn"'));
    await user.click(screen.getByRole('tab', { name: 'Visual editor' }));
    expect(await screen.findByRole('combobox', { name: 'Log level' })).toHaveTextContent('warn');
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Save configuration' }));
    await waitFor(() => expect(client.saveConfigurationFile).toHaveBeenCalledTimes(1));
    const [input] = client.saveConfigurationFile.mock.calls[0];
    expect(input.content).toContain('4.2000e+99');
    expect(JSON.parse(input.content).log.level).toBe('warn');
    expect(input.content).not.toContain('_panel');
  });

  it('prefers the enabled version and remembers an explicit version across route changes', async () => {
    const user = userEvent.setup();
    const current = {
      ...testArtifacts.items[0], id: 'core_114', exact_version: '1.14.0', reported_version: '1.14.0',
    };
    const legacy = { ...testArtifacts.items[0], id: 'core_113' };
    const client = await createStructuredClient({
      listCoreArtifacts: vi.fn().mockResolvedValue({ items: [current, legacy] }),
      getRuntimeStatus: vi.fn().mockResolvedValue({
        desired_running: false,
        enabled_core: { core_artifact_id: legacy.id, exact_core_version: legacy.exact_version },
        observation_state: 'stopped',
        target_generation: 2,
      }),
    });
    render(
      <MemoryRouter initialEntries={['/configuration']}>
        <ApiClientProvider client={client}>
          <TooltipProvider delay={0}>
            <ControlPlaneProvider>
              <Link to='/other'>Other page</Link>
              <Link to='/configuration'>Configuration page</Link>
              <Routes>
                <Route path='/configuration' element={<ConfigurationPage />} />
                <Route path='/other' element={<p>Another page</p>} />
              </Routes>
            </ControlPlaneProvider>
          </TooltipProvider>
        </ApiClientProvider>
      </MemoryRouter>,
    );

    const version = await screen.findByRole('combobox', { name: 'Configuration version' });
    expect(version).toHaveTextContent('1.13.19');
    await user.click(version);
    await user.click(await screen.findByRole('option', { name: '1.14.0' }));
    expect(version).toHaveTextContent('1.14.0');
    await user.click(screen.getByRole('link', { name: 'Other page' }));
    await user.click(screen.getByRole('link', { name: 'Configuration page' }));
    expect(await screen.findByRole('combobox', { name: 'Configuration version' })).toHaveTextContent('1.14.0');
  });

  it.each(['file', 'schema'])(
    'keeps the workspace mounted without a JSON flash when %s loads first',
    async first => {
      const client = await createStructuredClient();
      const contract = await client.getConfigurationSchema('core_114');
      let finishFile!: (file: ConfigurationFile) => void;
      let finishSchema!: (schema: ConfigurationSchemaContract) => void;
      client.getConfigurationFile = vi.fn().mockReturnValue(new Promise<ConfigurationFile>(resolve => {
        finishFile = resolve;
      }));
      client.getConfigurationSchema = vi.fn().mockReturnValue(new Promise<ConfigurationSchemaContract>(resolve => {
        finishSchema = resolve;
      }));
      renderPage(client);
      await waitFor(() => expect(client.getConfigurationSchema).toHaveBeenCalled());
      const workspace = screen.getByRole('region', { name: 'Configuration' });
      const tabs = screen.getByRole('tablist', { name: 'Configuration sections' });
      const save = screen.getByRole('button', { name: 'Save configuration' });
      expect(workspace).toHaveAttribute('aria-busy', 'true');
      expect(save).toBeDisabled();
      expect(screen.getByRole('button', { name: 'Validate configuration' })).toBeDisabled();

      await act(async () => first === 'file' ? finishFile(savedFile) : finishSchema(contract));
      expect(screen.getByRole('region', { name: 'Configuration' })).toBe(workspace);
      expect(screen.getByRole('tab', { name: 'Visual editor' })).toHaveAttribute('aria-selected', 'true');
      expect(screen.queryByLabelText('sing-box configuration JSON')).not.toBeInTheDocument();

      await act(async () => first === 'file' ? finishSchema(contract) : finishFile(savedFile));
      await screen.findByRole('combobox', { name: 'Log level' });
      expect(screen.getByRole('region', { name: 'Configuration' })).toBe(workspace);
      expect(screen.getByRole('tablist', { name: 'Configuration sections' })).toBe(tabs);
      expect(screen.getByRole('button', { name: 'Save configuration' })).toBe(save);
      expect(workspace).toHaveAttribute('aria-busy', 'false');
      expect(screen.getByRole('tab', { name: 'Visual editor' })).toHaveAttribute('aria-selected', 'true');
      expect(screen.queryByLabelText('sing-box configuration JSON')).not.toBeInTheDocument();
    },
  );

  it('preserves an explicit JSON choice while the schema is loading', async () => {
    const user = userEvent.setup();
    const client = await createStructuredClient();
    const contract = await client.getConfigurationSchema('core_114');
    let finishSchema!: (schema: ConfigurationSchemaContract) => void;
    client.getConfigurationSchema = vi.fn().mockReturnValue(new Promise<ConfigurationSchemaContract>(resolve => {
      finishSchema = resolve;
    }));
    renderPage(client);
    await waitFor(() => expect(screen.getByRole('tab', { name: 'Advanced JSON' })).toBeEnabled());
    await user.click(screen.getByRole('tab', { name: 'Advanced JSON' }));
    const editor = await screen.findByLabelText('sing-box configuration JSON');
    await act(async () => finishSchema(contract));
    await waitFor(() => expect(screen.getByRole('tab', { name: 'Visual editor' })).toBeEnabled());
    expect(screen.getByRole('tab', { name: 'Advanced JSON' })).toHaveAttribute('aria-selected', 'true');
    expect(screen.getByLabelText('sing-box configuration JSON')).toBe(editor);
  });

  it('adds from the active section toolbar without navigating away', async () => {
    const user = userEvent.setup();
    const client = await createStructuredClient({
      getConfigurationFile: vi.fn().mockResolvedValue({ ...savedFile, content: '{"dns":{"servers":[],"rules":[]}}' }),
    });
    await renderReadyPage(client);
    await user.click(screen.getByRole('tab', { name: 'DNS' }));
    const toolbar = screen.getByRole('tablist', { name: 'dns' }).parentElement!;
    expect(within(toolbar).getByRole('button', { name: 'Add' })).toBeInTheDocument();
    await user.click(screen.getByRole('tab', { name: 'Resolution & cache' }));
    expect(within(toolbar).queryByRole('button', { name: 'Add' })).not.toBeInTheDocument();
    await user.click(screen.getByRole('tab', { name: 'DNS rules' }));
    await user.click(within(toolbar).getByRole('button', { name: 'Add' }));
    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Cancel' }));
    expect(screen.getByRole('button', { name: 'Save configuration' })).toBeDisabled();
    await user.click(screen.getByRole('tab', { name: 'Servers' }));
    await user.click(within(toolbar).getByRole('button', { name: 'Add' }));
    const dialog = screen.getByRole('dialog');
    fireEvent.change(within(dialog).getByRole('textbox', { name: 'Tag' }), { target: { value: 'dns-new' } });
    await user.click(within(dialog).getByRole('button', { name: 'Add' }));
    expect(screen.getByText('dns-new')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Save configuration' }));
    await waitFor(() => expect(client.saveConfigurationFile).toHaveBeenCalledWith({
      revision: 1, content: expect.any(String),
    }));
    const saved = vi.mocked(client.saveConfigurationFile).mock.calls[0][0];
    expect(JSON.parse(saved.content)).toEqual({ dns: { servers: [{ type: 'dhcp', tag: 'dns-new' }], rules: [] } });
  });

  it.each([
    ['Certificate providers', 'certificate_providers'],
    ['HTTP clients', 'http_clients'],
    ['Network namespaces', 'network_namespaces'],
  ])('creates %s from its standalone table and saves only confirmed changes', async (label, collection) => {
    const user = userEvent.setup();
    const client = await createStructuredClient({
      getConfigurationFile: vi.fn().mockResolvedValue({ ...savedFile, content: '{}' }),
    });
    await renderReadyPage(client);
    await user.click(screen.getByRole('tab', { name: label }));
    const table = screen.getByRole('table');
    const add = within(table).getByRole('button', { name: 'Add' });
    await user.click(add);
    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Cancel' }));
    expect(screen.getByRole('button', { name: 'Save configuration' })).toBeDisabled();
    await user.click(add);
    const dialog = screen.getByRole('dialog');
    fireEvent.change(within(dialog).getByRole('textbox', { name: 'Tag' }), { target: { value: 'new-entry' } });
    await user.click(within(dialog).getByRole('button', { name: 'Add' }));
    expect(within(table).getByText('new-entry')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Save configuration' }));
    await waitFor(() => expect(client.saveConfigurationFile).toHaveBeenCalled());
    const saved = vi.mocked(client.saveConfigurationFile).mock.calls[0][0];
    expect(JSON.parse(saved.content)[collection]).toEqual([expect.objectContaining({ tag: 'new-entry' })]);
  });

  it.each(['{"log":{"level":"debug"}}', '{"log":'])('cancels navigation with unsaved %s and saves exact text', async content => {
    const user = userEvent.setup();
    const client = createMockApiClient();
    render(
      <MemoryRouter initialEntries={['/configuration#configuration-advanced']}>
        <ApiClientProvider client={client}>
          <ControlPlaneProvider>
            <Link to='/other'>Other page</Link>
            <Link to='/configuration#configuration-advanced'>Configuration page</Link>
            <Routes>
              <Route path='/configuration' element={<ConfigurationPage />} />
              <Route path='/other' element={<p>Another page</p>} />
            </Routes>
          </ControlPlaneProvider>
        </ApiClientProvider>
      </MemoryRouter>,
    );
    changeEditor(await screen.findByLabelText('sing-box configuration JSON'), content);
    await user.click(screen.getByRole('link', { name: 'Other page' }));
    const unload = new Event('beforeunload', { cancelable: true });
    window.dispatchEvent(unload);
    expect(unload.defaultPrevented).toBe(true);
    await user.click(screen.getByRole('button', { name: 'Keep editing' }));
    expect(editorView(await screen.findByLabelText('sing-box configuration JSON')).state.doc.toString()).toBe(content);
    expect(client.getConfigurationFile).toHaveBeenCalledTimes(1);
    await user.click(screen.getByRole('button', { name: 'Save configuration' }));
    await waitFor(() => expect(client.saveConfigurationFile).toHaveBeenCalledWith({ revision: 1, content }));
    await waitFor(() => {
      const after = new Event('beforeunload', { cancelable: true });
      window.dispatchEvent(after);
      expect(after.defaultPrevented).toBe(false);
    });
    if (content.endsWith(':')) {
      expect(screen.getByRole('button', { name: 'Validate configuration' })).toBeDisabled();
      expect(screen.getByText('Saved · JSON needs correction')).toBeInTheDocument();
    }
  });

  it('allows saving the initial empty file', async () => {
    const client = createMockApiClient({
      getConfigurationFile: vi.fn().mockResolvedValue({ revision: 0, content: '{}', syntax_valid: true }),
    });
    const user = userEvent.setup();
    renderPage(client, '/configuration#configuration-advanced');
    expect(editorView(await screen.findByLabelText('sing-box configuration JSON')).state.doc.toString()).toBe('{}');
    expect(screen.getByRole('button', { name: 'Validate configuration' })).toBeDisabled();
    await user.click(screen.getByRole('button', { name: 'Save configuration' }));
    await waitFor(() => expect(client.saveConfigurationFile).toHaveBeenCalledWith({ revision: 0, content: '{}' }));
  });

  it('does not restore a discarded configuration draft on returning to the page', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient();
    render(
      <MemoryRouter initialEntries={['/configuration#configuration-advanced']}>
        <ApiClientProvider client={client}>
          <ControlPlaneProvider>
            <Link to='/other'>Other page</Link>
            <Link to='/configuration#configuration-advanced'>Configuration page</Link>
            <Routes>
              <Route path='/configuration' element={<ConfigurationPage />} />
              <Route path='/other' element={<p>Another page</p>} />
            </Routes>
          </ControlPlaneProvider>
        </ApiClientProvider>
      </MemoryRouter>,
    );
    const editor = await screen.findByLabelText('sing-box configuration JSON');
    const saved = editorView(editor).state.doc.toString();
    changeEditor(editor, '{"unsaved":');
    await user.click(screen.getByRole('link', { name: 'Other page' }));
    await user.click(screen.getByRole('button', { name: 'Discard changes' }));
    expect(screen.getByText('Another page')).toBeVisible();
    await user.click(screen.getByRole('link', { name: 'Configuration page' }));
    expect(editorView(await screen.findByLabelText('sing-box configuration JSON')).state.doc.toString()).toBe(saved);
    expect(client.saveConfigurationFile).not.toHaveBeenCalled();
  });

  it('loads previously saved incomplete JSON and permits correcting it', async () => {
    renderPage(createMockApiClient({
      getConfigurationFile: vi.fn().mockResolvedValue({ revision: 8, content: '{"log":', syntax_valid: false }),
    }));
    const editor = await screen.findByLabelText('sing-box configuration JSON');
    expect(editorView(editor).state.doc.toString()).toBe('{"log":');
    expect(screen.getByRole('tab', { name: 'Visual editor' })).toHaveAttribute('aria-disabled', 'true');
    changeEditor(editor, '{"log":{}}');
    expect(screen.getByRole('button', { name: 'Save configuration' })).toBeEnabled();
    expect(screen.getByRole('button', { name: 'Validate configuration' })).toBeDisabled();
  });

  it('locks editing during save and preserves exact numeric text and the file CAS revision', async () => {
    const user = userEvent.setup();
    let finishSave: (value: ConfigurationFile) => void = () => {};
    const pending = new Promise<ConfigurationFile>(resolve => {
      finishSave = resolve;
    });
    const client = createMockApiClient({ saveConfigurationFile: vi.fn().mockReturnValue(pending) });
    renderPage(client, '/configuration#configuration-advanced');
    const editor = await screen.findByLabelText('sing-box configuration JSON');
    const content = '{\n  "future": { "large": 900719925474099312345678901234567890, "threshold": 4.2000e+99 }\n}';
    changeEditor(editor, content);
    await user.click(screen.getByRole('button', { name: 'Save configuration' }));
    expect(editor).toHaveAttribute('contenteditable', 'false');
    expect(client.saveConfigurationFile).toHaveBeenCalledWith({ revision: 1, content });
    finishSave({ ...savedFile, revision: 2, content });
    await waitFor(() => expect(editor).toHaveAttribute('contenteditable', 'true'));
    expect(editorView(editor).state.doc.toString()).toBe(content);
  });

  it('keeps raw editing and binary checks available after Schema verification fails', async () => {
    const base = await createStructuredClient();
    const contract = await base.getConfigurationSchema('core_114');
    const client = await createStructuredClient({
      getConfigurationSchema: vi.fn().mockResolvedValue({ ...contract, schema_sha256: 'f'.repeat(64) }),
    });
    renderPage(client);
    expect(await screen.findByLabelText('sing-box configuration JSON')).toHaveAttribute('contenteditable', 'true');
    await waitFor(() => expect(client.getConfigurationSchema).toHaveBeenCalled());
    expect(screen.getByRole('tab', { name: 'Visual editor' })).toHaveAttribute('aria-disabled', 'true');
    expect(screen.getByRole('button', { name: 'Validate configuration' })).toBeEnabled();
  });

  it('retains unsaved text when a concurrent file update rejects the save', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient({
      saveConfigurationFile: vi.fn().mockRejectedValue(new ApiRequestError('Configuration changed elsewhere.', { status: 412, code: 'configuration_file_conflict' })),
    });
    renderPage(client, '/configuration#configuration-advanced');
    const editor = await screen.findByLabelText('sing-box configuration JSON');
    const content = '{"future_feature":{"enabled":true}}';
    changeEditor(editor, content);
    await user.click(screen.getByRole('button', { name: 'Save configuration' }));
    await waitFor(() => expect(toastAdd).toHaveBeenCalledWith(expect.objectContaining({ type: 'error' })));
    expect(editor).toHaveAttribute('contenteditable', 'true');
    expect(editorView(editor).state.doc.toString()).toBe(content);
    expect(screen.getByRole('button', { name: 'Save configuration' })).toBeEnabled();
  });

  it.each(['ready', 'failed'] as const)('shows a %s validation result without launching the core', async state => {
    const user = userEvent.setup();
    let finish!: (value: ConfigurationCompile) => void;
    const client = createMockApiClient({
      compileConfiguration: vi.fn(() => new Promise<ConfigurationCompile>(resolve => {
        finish = resolve;
      })),
    });
    renderPage(client, '/configuration#configuration-advanced');
    await screen.findByLabelText('sing-box configuration JSON');
    await user.click(screen.getByRole('button', { name: 'Validate configuration' }));
    expect(screen.getByLabelText('sing-box configuration JSON')).toHaveAttribute('contenteditable', 'false');
    expect(toastAdd).not.toHaveBeenCalled();
    await act(async () => finish({ support: { structured: true, exact_version: '1.13.19' }, artifact: { ...testStartupArtifact, state } }));
    await waitFor(() => expect(toastAdd).toHaveBeenCalledWith(expect.objectContaining({
      type: state === 'ready' ? 'success' : 'error',
    })));
    expect(screen.getByLabelText('sing-box configuration JSON')).toHaveAttribute('contenteditable', 'true');
    expect(client.startRuntime).not.toHaveBeenCalled();
    expect(client.restartRuntime).not.toHaveBeenCalled();
  });

  it('validates only fields declared by the version Schema', () => {
    expect(visibleStructuredConfiguration({
      type: 'object', properties: { log: { type: 'object', properties: { level: { type: 'string' } } } },
    }, { future: true, log: { future: 'retained', level: 'info' } })).toEqual({ log: { level: 'info' } });
  });
});
