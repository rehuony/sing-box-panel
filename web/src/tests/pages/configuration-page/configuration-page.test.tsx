import { EditorView } from '@codemirror/view';
import userEvent from '@testing-library/user-event';
import { Link, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
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
import { ConfigurationPage } from '@/pages/configuration-page/configuration-page';
import { visibleStructuredConfiguration } from '@/pages/configuration-page/structured-validation';
import {
  createMockApiClient,
  testArtifacts,
  testDashboardContext,
  testRevision,
  testStartupArtifact,
} from '@/tests/api/mock-api-client';

const reviewedSchema = reviewedSchemaManifest['1.14.0'];
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

function renderPage(client: ApiClient) {
  return render(
    <MemoryRouter>
      <ApiClientProvider client={client}>
        <TooltipProvider delay={0}>
          <ControlPlaneProvider><ConfigurationPage /></ControlPlaneProvider>
        </TooltipProvider>
      </ApiClientProvider>
    </MemoryRouter>,
  );
}

async function createStructuredClient(overrides: Partial<ApiClient> = {}, exactVersion = '1.14.0') {
  const reviewed = await reviewedSchemaManifest[exactVersion]?.load();
  if (reviewed === undefined) throw new Error(`The reviewed ${exactVersion} Schema fixture is unavailable.`);
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

beforeEach(() => {
  toastAdd.mockClear();
});

describe('configurationPage', () => {
  it('explains an unsupported visual editor on hover and keyboard focus without an inline notice', async () => {
    const user = userEvent.setup();
    const unsupported = {
      ...testArtifacts.items[0], exact_version: '1.14.2', reported_version: '1.14.2',
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
    expect(screen.queryByText(/No visual editor schema for 1\.14\.2/)).not.toBeInTheDocument();

    await user.hover(trigger);
    expect(await screen.findByText(/No visual editor schema for 1\.14\.2/)).toBeVisible();
    await user.unhover(trigger);
    await waitFor(() => expect(screen.queryByText(/No visual editor schema for 1\.14\.2/)).not.toBeInTheDocument());

    trigger.focus();
    expect(await screen.findByText(/No visual editor schema for 1\.14\.2/)).toBeVisible();
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
    expect(screen.getByText('No installed versions')).toBeVisible();
    expect(screen.getByRole('link', { name: 'Go to version management' })).toHaveAttribute('href', '/cores#cores-catalog');
    expect(screen.getByRole('button', { name: 'Validate configuration' })).toBeDisabled();
    expect(client.getConfigurationSchema).not.toHaveBeenCalled();
    await user.click(screen.getByRole('tab', { name: 'Advanced JSON' }));
    const editor = await screen.findByLabelText('sing-box configuration JSON');
    changeEditor(editor, '{"log":{"level":"debug"}}');
    await user.click(screen.getByRole('button', { name: 'Save configuration' }));
    await waitFor(() => expect(client.saveConfigurationFile).toHaveBeenCalledWith({
      revision: 0, content: '{"log":{"level":"debug"}}',
    }));
  });

  it.each(['1.14.0', '1.14.1'])('keeps %s visual modules and Advanced JSON synchronized without navigation prompts', async (exactVersion) => {
    const user = userEvent.setup();
    const client = await createStructuredClient({
      getConfigurationFile: vi.fn().mockResolvedValue({
        ...savedFile, content: '{"dns":{},"log":{"level":"info"}}',
      }),
    }, exactVersion);
    renderPage(client);

    const level = await screen.findByRole('combobox', { name: 'Log level' });
    await user.click(level);
    await user.click(await screen.findByRole('option', { name: 'debug' }));
    await user.click(screen.getByRole('tab', { name: 'DNS' }));
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();

    await user.click(screen.getByRole('tab', { name: 'Advanced JSON' }));
    const editor = await screen.findByLabelText('sing-box configuration JSON');
    expect(JSON.parse(editorView(editor).state.doc.toString()).log.level).toBe('debug');
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();

    changeEditor(editor, '{"dns":{},"log":{"level":"warn"}}');
    await user.click(screen.getByRole('tab', { name: 'Visual editor' }));
    expect(await screen.findByRole('combobox', { name: 'Log level' })).toHaveTextContent('warn');
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();
  });

  it.skipIf(reviewedSchema === undefined)('prefers the enabled version and remembers an explicit version across route changes', async () => {
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

  it.skipIf(reviewedSchema === undefined).each(['file', 'schema'])(
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

  it.skipIf(reviewedSchema === undefined)('preserves an explicit JSON choice while the schema is loading', async () => {
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

  it.skipIf(reviewedSchema === undefined)('adds from the active section toolbar without navigating away', async () => {
    const user = userEvent.setup();
    const client = await createStructuredClient({
      getConfigurationFile: vi.fn().mockResolvedValue({ ...savedFile, content: '{"dns":{"servers":[],"rules":[]}}' }),
    });
    renderPage(client);
    await user.click(await screen.findByRole('tab', { name: 'DNS' }));
    const toolbar = screen.getByRole('tablist', { name: 'dns' }).parentElement!;
    expect(within(toolbar).getByRole('button', { name: 'Add' })).toBeInTheDocument();
    expect(screen.queryByText('0 items')).not.toBeInTheDocument();
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
    expect(screen.queryByRole('button', { name: 'Back' })).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Save configuration' }));
    await waitFor(() => expect(client.saveConfigurationFile).toHaveBeenCalledWith({
      revision: 1, content: expect.any(String),
    }));
    const saved = vi.mocked(client.saveConfigurationFile).mock.calls[0][0];
    expect(JSON.parse(saved.content)).toEqual({ dns: { servers: [{ type: 'dhcp', tag: 'dns-new' }], rules: [] } });
  });

  it.each(['{"log":{"level":"debug"}}', '{"log":'])('cancels navigation with unsaved %s and saves exact text', async content => {
    const user = userEvent.setup();
    const client = createMockApiClient();
    render(
      <MemoryRouter initialEntries={['/configuration']}>
        <ApiClientProvider client={client}>
          <ControlPlaneProvider>
            <Link to='/other'>Other page</Link>
            <Link to='/configuration'>Configuration page</Link>
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

  it('allows saving the initial empty file without creating history or deploy tabs', async () => {
    const client = createMockApiClient({
      getConfigurationFile: vi.fn().mockResolvedValue({ revision: 0, content: '{}', syntax_valid: true }),
    });
    const user = userEvent.setup();
    renderPage(client);
    expect(editorView(await screen.findByLabelText('sing-box configuration JSON')).state.doc.toString()).toBe('{}');
    expect(screen.getByRole('button', { name: 'Validate configuration' })).toBeDisabled();
    await user.click(screen.getByRole('button', { name: 'Save configuration' }));
    await waitFor(() => expect(client.saveConfigurationFile).toHaveBeenCalledWith({ revision: 0, content: '{}' }));
    expect(screen.queryByRole('tab', { name: 'History' })).not.toBeInTheDocument();
    expect(screen.queryByRole('tab', { name: 'Deploy' })).not.toBeInTheDocument();
  });

  it('does not restore a discarded configuration draft on returning to the page', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient();
    render(
      <MemoryRouter initialEntries={['/configuration']}>
        <ApiClientProvider client={client}>
          <ControlPlaneProvider>
            <Link to='/other'>Other page</Link>
            <Link to='/configuration'>Configuration page</Link>
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
    renderPage(client);
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

  it.skipIf(reviewedSchema === undefined)('defaults to native visual fields and preserves unknown values', async () => {
    const user = userEvent.setup();
    const client = await createStructuredClient({
      getConfigurationFile: vi.fn().mockResolvedValue({
        ...savedFile, content: '{"experimental":{"large":4.2000e+99},"log":{"level":"info"}}',
      }),
    });
    renderPage(client);
    await user.click(await screen.findByRole('tab', { name: 'Logging' }, { timeout: 10_000 }));
    const level = await screen.findByRole('combobox', { name: 'Log level' });
    expect(screen.getByRole('tab', { name: 'Visual editor' })).toHaveAttribute('aria-selected', 'true');
    await user.click(level);
    await user.click(await screen.findByRole('option', { name: 'debug' }));
    await user.click(screen.getByRole('button', { name: 'Save configuration' }));
    await waitFor(() => expect(client.saveConfigurationFile).toHaveBeenCalledTimes(1));
    const [input] = client.saveConfigurationFile.mock.calls[0];
    expect(input.content).toContain('4.2000e+99');
    expect(JSON.parse(input.content).log.level).toBe('debug');
    expect(input.content).not.toContain('_panel');
  }, 20_000);

  it.skipIf(reviewedSchema === undefined)('keeps raw editing and binary checks available after Schema verification fails', async () => {
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
    renderPage(client);
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
    renderPage(client);
    await screen.findByLabelText('sing-box configuration JSON');
    await user.click(screen.getByRole('button', { name: 'Validate configuration' }));
    expect(screen.getByLabelText('sing-box configuration JSON')).toHaveAttribute('contenteditable', 'false');
    expect(toastAdd).not.toHaveBeenCalled();
    await act(async () => finish({ support: { structured: false, exact_version: '1.13.19' }, artifact: { ...testStartupArtifact, state } }));
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
