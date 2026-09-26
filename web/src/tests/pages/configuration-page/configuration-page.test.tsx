import { EditorView } from '@codemirror/view';
import userEvent from '@testing-library/user-event';
import { act, render, screen, waitFor } from '@testing-library/react';
import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';

import type { ApiClient, ConfigurationCompile, ConfigurationFile, ConfigurationSchemaContract } from '@/api/api-client';

import '@/i18n';
import { toast } from '@/components/ui/toast-manager';
import { TooltipProvider } from '@/components/ui/tooltip';
import { ApiClientProvider } from '@/api/api-client-context';
import { reviewedSchemaManifest } from '@/schemas/generated';
import { TestRouter as MemoryRouter } from '@/tests/test-router';
import { ControlPlaneProvider } from '@/stores/control-plane-provider';
import { representativeSchemaVersions } from '@/tests/schemas/schema-fixtures';
import { ConfigurationPage } from '@/pages/configuration-page/configuration-page';
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
  await act(() => vi.dynamicImportSettled());
  await screen.findByRole('region', { name: 'Configuration', busy: false });
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
  await Promise.all([
    ...structuredVersions.map(version => reviewedSchemaManifest[version].load()),
    import('@/pages/configuration-page/advanced-configuration-editor'),
  ]);
});

beforeEach(() => {
  toastAdd.mockClear();
});

describe('configurationPage', () => {
  it('submits a visual edit through the configuration save action', async () => {
    const user = userEvent.setup();
    const client = await createStructuredClient({
      getConfigurationFile: vi.fn().mockResolvedValue({ ...savedFile, content: '{"log":{"level":"info"}}' }),
    });
    await renderReadyPage(client);
    await user.click(screen.getByRole('combobox', { name: 'Log level' }));
    await user.click(screen.getByRole('option', { name: 'debug' }));
    await user.click(screen.getByRole('button', { name: 'Save configuration' }));
    await waitFor(() => expect(client.saveConfigurationFile).toHaveBeenCalledOnce());
    expect(JSON.parse(client.saveConfigurationFile.mock.calls[0][0].content)).toEqual({ log: { level: 'debug' } });
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
});
