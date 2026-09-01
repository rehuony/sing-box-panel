import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';

import type { ApiClient, CanonicalRevisionListFilter } from '@/api/api-client';

import '@/i18n';
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
} from '@/tests/api/mock-api-client';

const reviewedSchema = reviewedSchemaManifest['1.14.0'];

function renderPage(client: ApiClient) {
  return render(
    <MemoryRouter>
      <ApiClientProvider client={client}>
        <ControlPlaneProvider><ConfigurationPage /></ControlPlaneProvider>
      </ApiClientProvider>
    </MemoryRouter>,
  );
}

async function selectCombobox(
  user: ReturnType<typeof userEvent.setup>,
  name: string,
  option: string,
) {
  const trigger = await screen.findByRole('combobox', { name }, { timeout: 10_000 });
  await user.click(trigger);
  await user.click(await screen.findByRole('option', { name: option }));
  return trigger;
}

async function createStructuredClient(overrides: Partial<ApiClient> = {}) {
  const reviewed = await reviewedSchema?.load();
  if (reviewed === undefined) throw new Error('The reviewed 1.14.0 Schema fixture is unavailable.');
  const artifact = {
    ...testArtifacts.items[0],
    id: 'core_114',
    exact_version: '1.14.0',
    reported_version: '1.14.0',
  };
  return createMockApiClient({
    getDashboardContext: vi.fn().mockResolvedValue({
      ...testDashboardContext,
      view: { exactVersion: '1.14.0' },
    }),
    getConfigurationSchema: vi.fn().mockResolvedValue({
      exact_version: '1.14.0',
      schema_sha256: reviewed.schemaSHA256,
      schema: reviewed.schema,
    }),
    listCoreArtifacts: vi.fn().mockResolvedValue({ items: [artifact] }),
    ...overrides,
  });
}

describe('configurationPage', () => {
  it('validates only fields declared by the version Schema', () => {
    const result = visibleStructuredConfiguration({
      type: 'object',
      properties: {
        inbounds: {
          type: 'array',
          items: {
            type: 'object',
            properties: { type: { type: 'string' } },
          },
        },
        log: {
          type: 'object',
          properties: { level: { type: 'string' } },
        },
      },
    }, {
      future_top_level: { retained: true },
      inbounds: [{ retained: 'losslessly', type: 'mixed' }],
      log: { future_nested: { retained: true }, level: 'info' },
    });

    expect(result).toEqual({
      inbounds: [{ type: 'mixed' }],
      log: { level: 'info' },
    });
  });

  it('uses Advanced JSON for a pre-1.14 version without blocking deployment', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient();
    renderPage(client);

    expect(await screen.findByText(/has no native configuration Schema/)).toBeInTheDocument();
    expect(screen.queryByRole('tab', { name: 'General' })).not.toBeInTheDocument();
    expect(screen.queryByRole('tab', { name: 'Managed' })).not.toBeInTheDocument();
    expect(screen.getByLabelText('sing-box configuration JSON')).toBeEnabled();
    expect(client.getConfigurationSchema).not.toHaveBeenCalled();

    await user.click(screen.getByRole('tab', { name: 'Deploy' }));
    expect((await screen.findAllByText('JSON only')).length).toBeGreaterThan(0);
    expect(await screen.findByRole('button', { name: 'Compile and queue check' })).toBeEnabled();
  });

  it('retains the revision cursor and appends older history on request', async () => {
    const user = userEvent.setup();
    const olderRevision = {
      ...testRevision,
      id: 'revision_40',
      parent_id: 'revision_39',
      sequence: 40,
    };
    const listRevisions = vi.fn(async (filter: CanonicalRevisionListFilter = {}) =>
      filter.beforeSequence === 41
        ? { items: [olderRevision] }
        : { items: [testRevision], next_before_sequence: 41 });
    const client = createMockApiClient({ listRevisions });
    renderPage(client);

    await user.click(await screen.findByRole('tab', { name: 'History' }));
    await user.click(await screen.findByRole('button', { name: 'Load older' }));

    expect(await screen.findByText('revision_40')).toBeInTheDocument();
    expect(listRevisions).toHaveBeenCalledWith({ beforeSequence: 41, limit: 8 });
    expect(screen.queryByRole('button', { name: 'Load older' })).not.toBeInTheDocument();
  });

  it('locks the raw editor while a revision save is in flight', async () => {
    const user = userEvent.setup();
    let finishSave: ((value: { revision: typeof testRevision; no_change: boolean }) => void) | undefined;
    const pendingSave = new Promise<{ revision: typeof testRevision; no_change: boolean }>((resolve) => {
      finishSave = resolve;
    });
    const client = createMockApiClient({
      replaceCanonical: vi.fn().mockReturnValue(pendingSave),
    });
    renderPage(client);

    const editor = await screen.findByLabelText('sing-box configuration JSON');
    fireEvent.change(editor, { target: { value: '{"log":{"level":"debug"}}' } });
    await user.click(screen.getByRole('button', { name: 'Save revision' }));

    expect(editor).toBeDisabled();
    finishSave?.({ revision: testRevision, no_change: false });
    await waitFor(() => expect(editor).toBeEnabled());
  });

  it('saves the complete raw configuration without changing numeric lexemes or the CAS base', async () => {
    const user = userEvent.setup();
    const replaceCanonical = vi.fn().mockResolvedValue({ revision: testRevision, no_change: false });
    const client = createMockApiClient({ replaceCanonical });
    renderPage(client);

    const editor = await screen.findByLabelText('sing-box configuration JSON');
    fireEvent.change(editor, {
      target: {
        value: `{
          "log": { "level": "trace" },
          "future_feature": {
            "opaque_counter": 900719925474099312345678901234567890,
            "threshold": 4.2000e+99
          }
        }`,
      },
    });
    await user.click(screen.getByRole('button', { name: 'Save revision' }));

    await waitFor(() => expect(replaceCanonical).toHaveBeenCalledTimes(1));
    const [documentJSON, baseRevision] = replaceCanonical.mock.calls[0] as [string, string];
    expect(baseRevision).toBe(testRevision.id);
    expect(documentJSON).toContain('"opaque_counter":900719925474099312345678901234567890');
    expect(documentJSON).toContain('"threshold":4.2000e+99');
    expect(documentJSON).not.toContain('schema_version');
    expect(documentJSON).not.toContain('_panel');
  });

  it.skipIf(reviewedSchema === undefined)('edits raw configuration through the 1.14 structured form', async () => {
    const user = userEvent.setup();
    const snapshot = {
      ...testRevision,
      document_json: '{"experimental":{"large":4.2000e+99},"inbounds":[],"log":{"level":"info"},"outbounds":[]}',
    };
    const replaceCanonical = vi.fn().mockResolvedValue({ revision: snapshot, no_change: false });
    const client = await createStructuredClient({
      getCanonical: vi.fn().mockResolvedValue(snapshot),
      listRevisions: vi.fn().mockResolvedValue({ items: [snapshot] }),
      replaceCanonical,
    });
    renderPage(client);

    await selectCombobox(user, 'Log level', 'debug');
    await user.click(screen.getByRole('button', { name: 'Save revision' }));

    await waitFor(() => expect(replaceCanonical).toHaveBeenCalledTimes(1));
    const [documentJSON] = replaceCanonical.mock.calls[0] as [string];
    expect(documentJSON).toContain('"large":4.2000e+99');
    expect(documentJSON).toContain('"level":"debug"');
    expect(documentJSON).not.toContain('configuration');
    expect(documentJSON).not.toContain('_panel');
  }, 20_000);

  it('keeps invalid Advanced JSON local until it is discarded', async () => {
    const user = userEvent.setup();
    const replaceCanonical = vi.fn();
    const client = createMockApiClient({ replaceCanonical });
    renderPage(client);

    const editor = await screen.findByLabelText('sing-box configuration JSON');
    fireEvent.change(editor, { target: { value: '{"log":' } });

    expect(await screen.findByRole('alert')).toHaveTextContent('Object value expected');
    expect(screen.getByRole('button', { name: 'Save revision' })).toBeDisabled();
    expect(replaceCanonical).not.toHaveBeenCalled();

    await user.click(screen.getByRole('button', { name: 'Discard' }));
    await waitFor(() => expect((editor as HTMLTextAreaElement).value).toContain('"log"'));
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Save revision' })).toBeEnabled();
  });

  it.skipIf(reviewedSchema === undefined)('falls back to Advanced without blocking deploy when Schema verification fails', async () => {
    const base = await createStructuredClient();
    const contract = await base.getConfigurationSchema('core_114');
    const client = await createStructuredClient({
      getConfigurationSchema: vi.fn().mockResolvedValue({
        ...contract,
        schema_sha256: 'f'.repeat(64),
      }),
    });
    const user = userEvent.setup();
    renderPage(client);

    expect(await screen.findByText('Structured editor is unavailable')).toBeInTheDocument();
    expect(screen.queryByRole('tab', { name: 'General' })).not.toBeInTheDocument();
    expect(screen.getByLabelText('sing-box configuration JSON')).toBeEnabled();

    await user.click(screen.getByRole('tab', { name: 'Deploy' }));
    expect(await screen.findByRole('button', { name: 'Compile and queue check' })).toBeEnabled();
  });

  it('retains a valid raw draft when server-side validation rejects the save', async () => {
    const user = userEvent.setup();
    const replaceCanonical = vi.fn().mockRejectedValue(new Error('The future feature is not valid.'));
    const client = createMockApiClient({ replaceCanonical });
    renderPage(client);

    const editor = await screen.findByLabelText('sing-box configuration JSON');
    const rejectedDocument = '{"future_feature":{"enabled":true}}';
    fireEvent.change(editor, { target: { value: rejectedDocument } });
    await user.click(screen.getByRole('button', { name: 'Save revision' }));

    expect(await screen.findByRole('alert')).toHaveTextContent('The future feature is not valid.');
    expect(editor).toBeEnabled();
    expect(editor).toHaveValue(rejectedDocument);
  });

  it('loads exact revision details and compares raw configuration paths', async () => {
    const user = userEvent.setup();
    const currentRevision = {
      ...testRevision,
      document_json: '{"experimental":{"large":900719925474099312345678901234567899}}',
    };
    const olderRevision = {
      ...testRevision,
      id: 'revision_41',
      parent_id: 'revision_40',
      sequence: 41,
      document_json: '{"experimental":{"large":900719925474099312345678901234567891}}',
    };
    const getRevision = vi.fn().mockResolvedValue(olderRevision);
    const diffRevisions = vi.fn().mockResolvedValue({
      from: olderRevision,
      to: currentRevision,
      changes: [{
        path: '/experimental/large',
        from: { present: true, value: 0 },
        to: { present: true, value: 0 },
      }],
    });
    const client = createMockApiClient({
      diffRevisions,
      getCanonical: vi.fn().mockResolvedValue(currentRevision),
      getRevision,
      listRevisions: vi.fn().mockResolvedValue({ items: [currentRevision, olderRevision] }),
    });
    renderPage(client);

    await user.click(await screen.findByRole('tab', { name: 'History' }));
    await user.click(await screen.findByRole('button', { name: 'View revision #41 details' }));
    await waitFor(() => expect(getRevision).toHaveBeenCalledWith(olderRevision.id, expect.any(AbortSignal)));
    expect(await screen.findByLabelText('Revision #41 canonical JSON'))
      .toHaveTextContent('900719925474099312345678901234567891');
    await user.keyboard('{Escape}');

    await user.selectOptions(screen.getByLabelText('From'), olderRevision.id);
    await user.selectOptions(screen.getByLabelText('To'), currentRevision.id);
    await user.click(screen.getByRole('button', { name: 'Compare' }));

    const diffPanel = screen.getByRole('heading', { name: 'Revision comparison' }).closest('[role="dialog"]');
    expect(diffPanel).not.toBeNull();
    const diff = within(diffPanel as HTMLElement);
    expect(await diff.findByText('/experimental/large')).toBeInTheDocument();
    expect(diff.getByText('900719925474099312345678901234567891')).toBeInTheDocument();
    expect(diff.getByText('900719925474099312345678901234567899')).toBeInTheDocument();
  });

  it('binds restore confirmation to the immutable revision and current base', async () => {
    const user = userEvent.setup();
    const olderRevision = {
      ...testRevision,
      id: 'revision_41',
      parent_id: 'revision_40',
      sequence: 41,
    };
    const restoreRevision = vi.fn().mockResolvedValue({ revision: testRevision, no_change: false });
    const client = createMockApiClient({
      listRevisions: vi.fn().mockResolvedValue({ items: [testRevision, olderRevision] }),
      restoreRevision,
    });
    renderPage(client);

    await user.click(await screen.findByRole('tab', { name: 'History' }));
    await user.click(await screen.findByRole('button', { name: 'Restore' }));

    const confirmation = await screen.findByRole('alertdialog');
    expect(within(confirmation).getByText(
      `Create a new revision from #41 (${olderRevision.id}) only if the current base is still ${testRevision.id}.`,
    )).toBeInTheDocument();
    await user.click(within(confirmation).getByRole('button', { name: 'Restore immutable revision' }));
    await waitFor(() => expect(restoreRevision).toHaveBeenCalledWith(olderRevision.id, testRevision.id));
  });
});
