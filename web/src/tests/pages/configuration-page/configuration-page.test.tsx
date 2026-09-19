import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { Link, MemoryRouter, Route, Routes } from 'react-router-dom';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';

import type { ApiClient, ConfigurationFile } from '@/api/api-client';

import '@/i18n';
import { ApiRequestError } from '@/api/api-client';
import { toast } from '@/components/ui/toast-manager';
import { ApiClientProvider } from '@/api/api-client-context';
import { reviewedSchemaManifest } from '@/schemas/generated';
import { ControlPlaneProvider } from '@/stores/control-plane-provider';
import { ConfigurationPage } from '@/pages/configuration-page/configuration-page';
import { visibleStructuredConfiguration } from '@/pages/configuration-page/structured-validation';
import {
  createMockApiClient,
  testArtifacts,
  testDashboardContext,
  testRevision,
  testStartupArtifact,
  testTask,
} from '@/tests/api/mock-api-client';

const reviewedSchema = reviewedSchemaManifest['1.14.0'];
const toastAdd = vi.spyOn(toast, 'add');
const savedFile: ConfigurationFile = {
  revision: 1, content: testRevision.document_json, syntax_valid: true, canonical_revision_id: testRevision.id,
};

function renderPage(client: ApiClient) {
  return render(
    <MemoryRouter>
      <ApiClientProvider client={client}>
        <ControlPlaneProvider><ConfigurationPage /></ControlPlaneProvider>
      </ApiClientProvider>
    </MemoryRouter>,
  );
}

async function createStructuredClient(overrides: Partial<ApiClient> = {}) {
  const reviewed = await reviewedSchema?.load();
  if (reviewed === undefined) throw new Error('The reviewed 1.14.0 Schema fixture is unavailable.');
  const artifact = {
    ...testArtifacts.items[0], id: 'core_114', exact_version: '1.14.0', reported_version: '1.14.0',
  };
  return createMockApiClient({
    getDashboardContext: vi.fn().mockResolvedValue({ ...testDashboardContext, view: { exactVersion: '1.14.0' } }),
    getConfigurationSchema: vi.fn().mockResolvedValue({
      exact_version: '1.14.0', schema_sha256: reviewed.schemaSHA256, schema: reviewed.schema,
    }),
    listCoreArtifacts: vi.fn().mockResolvedValue({ items: [artifact] }),
    ...overrides,
  });
}

beforeEach(() => {
  toastAdd.mockClear();
});

describe('configurationPage', () => {
  it.each(['{"log":{"level":"debug"}}', '{"log":'])('preserves unsaved %s across navigation and saves exact text', async content => {
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
    fireEvent.change(await screen.findByLabelText('sing-box configuration JSON'), { target: { value: content } });
    await user.click(screen.getByRole('link', { name: 'Other page' }));
    const unload = new Event('beforeunload', { cancelable: true });
    window.dispatchEvent(unload);
    expect(unload.defaultPrevented).toBe(true);
    await user.click(screen.getByRole('link', { name: 'Configuration page' }));
    expect(await screen.findByLabelText('sing-box configuration JSON')).toHaveValue(content);
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
    expect(await screen.findByLabelText('sing-box configuration JSON')).toHaveValue('{}');
    expect(screen.getByRole('button', { name: 'Validate configuration' })).toBeDisabled();
    await user.click(screen.getByRole('button', { name: 'Save configuration' }));
    await waitFor(() => expect(client.saveConfigurationFile).toHaveBeenCalledWith({ revision: 0, content: '{}' }));
    expect(client.listRevisions).not.toHaveBeenCalled();
    expect(client.getCanonical).not.toHaveBeenCalled();
    expect(screen.queryByRole('tab', { name: 'History' })).not.toBeInTheDocument();
    expect(screen.queryByRole('tab', { name: 'Deploy' })).not.toBeInTheDocument();
  });

  it('loads previously saved incomplete JSON and permits correcting it', async () => {
    renderPage(createMockApiClient({
      getConfigurationFile: vi.fn().mockResolvedValue({ revision: 8, content: '{"log":', syntax_valid: false }),
    }));
    const editor = await screen.findByLabelText('sing-box configuration JSON');
    expect(editor).toHaveValue('{"log":');
    expect(screen.getByRole('tab', { name: 'Visual editor' })).toHaveAttribute('aria-disabled', 'true');
    fireEvent.change(editor, { target: { value: '{"log":{}}' } });
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
    fireEvent.change(editor, { target: { value: content } });
    await user.click(screen.getByRole('button', { name: 'Save configuration' }));
    expect(editor).toBeDisabled();
    expect(client.saveConfigurationFile).toHaveBeenCalledWith({ revision: 1, content });
    finishSave({ ...savedFile, revision: 2, content });
    await waitFor(() => expect(editor).toBeEnabled());
    expect(editor).toHaveValue(content);
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
    expect(await screen.findByLabelText('sing-box configuration JSON')).toBeEnabled();
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
    fireEvent.change(editor, { target: { value: content } });
    await user.click(screen.getByRole('button', { name: 'Save configuration' }));
    await waitFor(() => expect(toastAdd).toHaveBeenCalledWith(expect.objectContaining({ type: 'error' })));
    expect(editor).toBeEnabled();
    expect(editor).toHaveValue(content);
    expect(screen.getByRole('button', { name: 'Save configuration' })).toBeEnabled();
  });

  it.each(['succeeded', 'failed'] as const)('waits for a %s validation result without launching the core', async status => {
    const user = userEvent.setup();
    const client = createMockApiClient({
      compileConfiguration: vi.fn().mockResolvedValue({ startup: testStartupArtifact, task: { ...testTask, status: 'queued' } }),
      getTask: vi.fn().mockResolvedValue({ ...testTask, status }),
    });
    renderPage(client);
    await screen.findByLabelText('sing-box configuration JSON');
    await user.click(screen.getByRole('button', { name: 'Validate configuration' }));
    expect(screen.getByLabelText('sing-box configuration JSON')).toBeDisabled();
    expect(toastAdd).not.toHaveBeenCalled();
    await waitFor(() => expect(toastAdd).toHaveBeenCalledWith(expect.objectContaining({
      type: status === 'succeeded' ? 'success' : 'error',
    })), { timeout: 3000 });
    expect(screen.getByLabelText('sing-box configuration JSON')).toBeEnabled();
    expect(client.startRuntime).not.toHaveBeenCalled();
    expect(client.restartRuntime).not.toHaveBeenCalled();
  });

  it('validates only fields declared by the version Schema', () => {
    expect(visibleStructuredConfiguration({
      type: 'object', properties: { log: { type: 'object', properties: { level: { type: 'string' } } } },
    }, { future: true, log: { future: 'retained', level: 'info' } })).toEqual({ log: { level: 'info' } });
  });
});
