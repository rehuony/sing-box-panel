import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';

import { ApiRequestError } from '@/api/api-client';
import { ApiClientProvider } from '@/api/api-client-context';
import { AppRoutes } from '@/routes';
import { AuthSessionProvider } from '@/stores/auth-session.store';
import {
  createMockApiClient,
  testCatalog,
  testDashboardContext,
  testRevision,
  testSession,
} from '@/tests/api/mock-api-client';

function renderRoutes(
  client = createMockApiClient(),
  initialEntry = '/',
) {
  return render(
    <ApiClientProvider client={client}>
      <AuthSessionProvider>
        <MemoryRouter initialEntries={[initialEntry]}>
          <AppRoutes />
        </MemoryRouter>
      </AuthSessionProvider>
    </ApiClientProvider>,
  );
}

describe('application routes', () => {
  it('redirects an anonymous visitor to login', async () => {
    const client = createMockApiClient({
      getSession: vi.fn().mockResolvedValue(null),
    });

    renderRoutes(client);

    expect(
      await screen.findByRole('heading', { name: 'Unlock this panel' }),
    ).toBeInTheDocument();
    expect(screen.getByLabelText('Management token')).toHaveAttribute(
      'autocomplete',
      'current-password',
    );
  });

  it('logs in with a management token and opens the dashboard', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient({
      getSession: vi.fn().mockResolvedValue(null),
      login: vi.fn().mockResolvedValue(testSession),
      getDashboardContext: vi.fn().mockResolvedValue(testDashboardContext),
    });

    renderRoutes(client, '/login');

    const tokenInput = await screen.findByLabelText('Management token');
    await user.type(tokenInput, 'local-management-token');
    await user.click(screen.getByRole('button', { name: 'Open console' }));

    expect(client.login).toHaveBeenCalledWith(
      'local-management-token',
      expect.any(AbortSignal),
    );
    expect(
      await screen.findByRole('heading', {
        name: 'One machine. One exact state.',
      }),
    ).toBeInTheDocument();
  });

  it('keeps capability warnings visible in the global context rail', async () => {
    renderRoutes();

    expect(
      await screen.findByText('Capability attention required'),
    ).toBeInTheDocument();
    const contextRail = screen
      .getByRole('heading', { name: 'Exact-version rail' })
      .closest('section');
    expect(contextRail).not.toBeNull();
    expect(within(contextRail!).getByText('Revision #42')).toBeInTheDocument();
    expect(within(contextRail!).getAllByText(/1.13.19/)).not.toHaveLength(0);
    expect(
      within(contextRail!).getByText('bundle_18 · Revision #41'),
    ).toBeInTheDocument();
  });

  it('shows an actionable inline message when login is rejected', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient({
      getSession: vi.fn().mockResolvedValue(null),
      login: vi.fn().mockRejectedValue(
        new ApiRequestError('unauthorized', {
          status: 401,
          code: 'unauthorized',
        }),
      ),
    });

    renderRoutes(client, '/login');

    await user.type(await screen.findByLabelText('Management token'), 'wrong');
    await user.click(screen.getByRole('button', { name: 'Open console' }));

    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('That management token was not accepted');
    expect(alert).toHaveFocus();
  });

  it('preserves a JSON draft when If-Match detects a revision conflict', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient({
      saveEntity: vi.fn().mockRejectedValue(
        new ApiRequestError('revision changed', {
          status: 412,
          code: 'canonical_revision_conflict',
        }),
      ),
    });

    renderRoutes(client, '/configuration');

    expect(
      await screen.findByRole('heading', { name: 'Save is not apply.' }),
    ).toBeInTheDocument();
    await user.click(await screen.findByRole('button', { name: 'Edit JSON' }));
    const editor = screen.getByLabelText('Entity JSON');
    const draft = '{"id":"edge-socks","kind":"socks","enabled":false}';
    fireEvent.change(editor, { target: { value: draft } });
    await user.click(screen.getByRole('button', { name: 'Save revision' }));

    expect(await screen.findByText('Revision conflict')).toBeInTheDocument();
    expect(editor).toHaveValue(draft);
    expect(screen.getByRole('button', { name: 'Reload current revision' })).toBeInTheDocument();
  });

  it('saves manual JSON as exact request-body bytes without applying it', async () => {
    const user = userEvent.setup();
    const manualContext = {
      ...testDashboardContext,
      capability: {
        level: 'manual_json' as const,
        label: 'Manual JSON',
        warning: 'No structured manifest is pinned for this exact version.',
      },
    };
    const client = createMockApiClient({
      getDashboardContext: vi.fn().mockResolvedValue(manualContext),
      getCoreCapability: vi.fn().mockResolvedValue({
        resolution: { exact_version: '1.13.19', source: 'explicit' },
        support_level: 'manual_json',
        pinned: false,
        quarantined: false,
      }),
    });

    renderRoutes(client, '/configuration');

    expect(
      await screen.findByRole('heading', {
        name: 'Structured support is unavailable for 1.13.19',
      }),
    ).toBeInTheDocument();
    const raw = '{\n  // keep this byte layout\n  "log": {"level": "warn"}\n}\n';
    fireEvent.change(await screen.findByLabelText('Startup bytes'), {
      target: { value: raw },
    });
    await user.click(screen.getByRole('button', { name: 'Preview reverse mapping' }));
    await waitFor(() => {
      expect(client.previewManualReplacement).toHaveBeenCalledWith({
        baseRevision: 'revision_42',
        coreVersion: '1.13.19',
        coreArtifactID: 'core_1',
        raw,
        allowCompatible: false,
      });
    });
    expect(
      await screen.findByRole('heading', { name: 'Manual reverse preview' }),
    ).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Save exact bytes' }));

    await waitFor(() => {
      expect(client.replaceManualArtifact).toHaveBeenCalledWith({
        baseRevision: 'revision_42',
        coreVersion: '1.13.19',
        coreArtifactID: 'core_1',
        raw,
        allowCompatible: false,
      });
    });
    expect(client.activateStartupArtifact).not.toHaveBeenCalled();
    expect(await screen.findByText(/check task task_manual_check is queued/)).toBeInTheDocument();
  });

  it('renders a compatible structured candidate only after explicit acceptance', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient();

    renderRoutes(client, '/configuration');

    const renderButton = await screen.findByRole('button', {
      name: 'Render structured candidate',
    });
    expect(renderButton).toBeDisabled();
    await user.click(
      await screen.findByRole('checkbox', { name: /Accept compatible projection/ }),
    );
    await user.click(renderButton);

    await waitFor(() => {
      expect(client.renderStructured).toHaveBeenCalledWith({
        coreVersion: '1.13.19',
        coreArtifactID: 'core_1',
        allowCompatible: true,
      });
    });
    expect(client.activateStartupArtifact).not.toHaveBeenCalled();
    expect(await screen.findByText(/Apply remains separate/)).toBeInTheDocument();
  });

  it('edits exact-version manifest controls only after compatible acceptance', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient();

    renderRoutes(client, '/configuration');

    const mode = await screen.findByLabelText('Routing mode');
    expect(mode).toBeDisabled();
    expect(screen.getByText('Behavior changed in this version')).toBeInTheDocument();
    expect(screen.queryByLabelText('Operator note')).not.toBeInTheDocument();

    await user.click(screen.getByRole('checkbox', {
      name: /Accept compatible controls for sing-box 1\.13\.19/,
    }));
    await user.selectOptions(mode, 'direct');
    const note = await screen.findByLabelText('Operator note');
    await user.type(note, 'Keep exact-version behavior');
    await user.click(screen.getByRole('button', { name: 'Save canonical revision' }));

    await waitFor(() => {
      expect(client.patchCanonical).toHaveBeenCalledWith(
        [
          { op: 'set', path: '/global/mode', value_json: '"direct"' },
          { op: 'set', path: '/global/note', value_json: '"Keep exact-version behavior"' },
        ],
        'revision_42',
      );
    });
    expect(client.replaceCanonical).not.toHaveBeenCalled();
    expect(client.renderStructured).not.toHaveBeenCalled();
  });

  it('renders every built-in control in stable manifest order and focuses its error summary', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient({
      getCoreCapability: vi.fn().mockResolvedValue({
        resolution: { exact_version: '1.13.19', source: 'explicit' },
        support_level: 'native_structured',
        pinned: true,
        quarantined: false,
        presentation: {
          semantic_facts: [
            { id: 'text', canonical_path: '/global/text', classification: 'supported' },
            { id: 'number', canonical_path: '/global/number', classification: 'supported' },
            { id: 'boolean', canonical_path: '/global/boolean', classification: 'supported' },
            { id: 'select', canonical_path: '/global/select', classification: 'supported' },
            { id: 'json', canonical_path: '/global/json', classification: 'intentionally_unsupported' },
            { id: 'collision.dot', canonical_path: '/global/collision_dot', classification: 'supported' },
            { id: 'collision-dot', canonical_path: '/global/collision_dash', classification: 'supported' },
          ],
          ui: [
            { id: 'json', fact_id: 'json', kind: 'json', label: 'JSON value', order: 60 },
            { id: 'group', fact_id: '', kind: 'group', label: 'Exact controls', order: 10 },
            { id: 'boolean', fact_id: 'boolean', kind: 'boolean', label: 'Boolean value', order: 40 },
            { id: 'text', fact_id: 'text', kind: 'text', label: 'Text value', order: 20 },
            { id: 'select', fact_id: 'select', kind: 'select', label: 'Select value', order: 50, options: [{ value: 'one', label: 'One' }] },
            { id: 'number', fact_id: 'number', kind: 'number', label: 'Number value', order: 30 },
            { id: 'collision.dot', fact_id: 'collision.dot', kind: 'text', label: 'Dot identity', order: 70 },
            { id: 'collision-dot', fact_id: 'collision-dot', kind: 'text', label: 'Dash identity', order: 80 },
          ],
        },
      }),
    });

    renderRoutes(client, '/configuration');

    const form = (await screen.findByRole('heading', { name: 'Versioned canonical controls' })).closest('section')!;
    await within(form).findByLabelText('Text value');
    const labels = within(form).getAllByText(/^(Text value|Number value|Boolean value|Select value|JSON value)$/);
    expect(labels.map((label) => label.textContent)).toEqual([
      'Text value', 'Number value', 'Boolean value', 'Select value', 'JSON value',
    ]);
    expect(within(form).getByLabelText('Text value')).toBeEnabled();
    expect(within(form).getByLabelText('Number value')).toHaveAttribute('type', 'text');
    expect(within(form).getByLabelText('Number value')).toHaveAttribute('inputmode', 'decimal');
    expect(within(form).getByLabelText('Boolean value')).toHaveAttribute('type', 'checkbox');
    expect(within(form).getByLabelText('Select value').tagName).toBe('SELECT');
    expect(within(form).getByLabelText('JSON value').tagName).toBe('TEXTAREA');
    expect(within(form).getByLabelText('Dot identity').id).not.toBe(
      within(form).getByLabelText('Dash identity').id,
    );
    expect(within(form).getByText('Intentionally unsupported')).toBeInTheDocument();

    await user.type(within(form).getByLabelText('Text value'), 'only edited field');
    await user.click(within(form).getByRole('button', { name: 'Save canonical revision' }));
    await waitFor(() => expect(client.patchCanonical).toHaveBeenCalledTimes(1));
    expect(client.patchCanonical).toHaveBeenCalledWith(
      [{ op: 'set', path: '/global/text', value_json: '"only edited field"' }],
      'revision_42',
    );

    fireEvent.change(within(form).getByLabelText('JSON value'), { target: { value: '{broken' } });
    await user.click(within(form).getByRole('button', { name: 'Save canonical revision' }));
    const summary = await within(form).findByRole('alert');
    expect(summary).toHaveTextContent('JSON value: Enter one valid JSON value.');
    expect(summary).toHaveFocus();
    expect(within(form).getByLabelText('JSON value')).toHaveAttribute('aria-invalid', 'true');
    expect(client.patchCanonical).toHaveBeenCalledTimes(1);
  });

  it('preserves exact numeric lexemes and sends only edited canonical pointers', async () => {
    const user = userEvent.setup();
    const documentJSON = '{"schema_version":1,"global":{"large":9007199254740993,"untouched":1e999,"payload":{"decimal":1.0}},"nodes":[],"rules":[],"subscription":{}}';
    const exactRevision = { ...testRevision, document_json: documentJSON };
    const client = createMockApiClient({
      getCanonical: vi.fn().mockResolvedValue(exactRevision),
      patchCanonical: vi.fn().mockResolvedValue({ revision: exactRevision, no_change: false }),
      getCoreCapability: vi.fn().mockResolvedValue({
        resolution: { exact_version: '1.13.19', source: 'explicit' },
        support_level: 'native_structured',
        pinned: true,
        quarantined: false,
        presentation: {
          semantic_facts: [
            { id: 'large', canonical_path: '/global/large', classification: 'supported' },
            { id: 'payload', canonical_path: '/global/payload', classification: 'supported' },
            { id: 'conditional', canonical_path: '/global/conditional', classification: 'supported' },
          ],
          ui: [
            { id: 'large', fact_id: 'large', kind: 'number', label: 'Exact number', order: 10 },
            { id: 'payload', fact_id: 'payload', kind: 'json', label: 'Exact JSON', order: 20 },
            {
              id: 'conditional',
              fact_id: 'conditional',
              kind: 'text',
              label: 'Large-number condition',
              order: 30,
              visible_when: { canonical_path: '/global/large', equals_json: '9007199254740993' },
            },
          ],
        },
      }),
    });

    renderRoutes(client, '/configuration');

    const number = await screen.findByLabelText('Exact number');
    expect(number).toHaveValue('9007199254740993');
    expect(screen.getByLabelText('Large-number condition')).toBeInTheDocument();
    await user.clear(number);
    await user.type(number, '9007199254740995');
    fireEvent.change(screen.getByLabelText('Exact JSON'), {
      target: { value: '{"long":9007199254740997,"huge":1e999,"decimal":1.0}' },
    });
    await user.click(screen.getByRole('button', { name: 'Save canonical revision' }));

    await waitFor(() => expect(client.patchCanonical).toHaveBeenCalledTimes(1));
    expect(client.patchCanonical).toHaveBeenCalledWith(
      [
        { op: 'set', path: '/global/large', value_json: '9007199254740995' },
        {
          op: 'set',
          path: '/global/payload',
          value_json: '{"long":9007199254740997,"huge":1e999,"decimal":1.0}',
        },
      ],
      'revision_42',
    );
    expect(JSON.stringify(client.patchCanonical.mock.calls[0][0])).not.toContain('untouched');
  });

  it('rejects scalar crossings and oversized array indexes before sending a patch', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient({
      getCanonical: vi.fn().mockResolvedValue({
        ...testRevision,
        document_json: '{"schema_version":1,"global":{"scalar":"leaf"},"nodes":[{"id":"edge","kind":"socks","enabled":true}],"rules":[],"subscription":{}}',
      }),
      getCoreCapability: vi.fn().mockResolvedValue({
        resolution: { exact_version: '1.13.19', source: 'explicit' },
        support_level: 'native_structured',
        pinned: true,
        quarantined: false,
        presentation: {
          semantic_facts: [
            { id: 'scalar', canonical_path: '/global/scalar/value', classification: 'supported' },
            { id: 'sparse', canonical_path: '/nodes/100001/name', classification: 'supported' },
          ],
          ui: [
            { id: 'scalar', fact_id: 'scalar', kind: 'text', label: 'Scalar crossing' },
            { id: 'sparse', fact_id: 'sparse', kind: 'text', label: 'Sparse array write' },
          ],
        },
      }),
    });

    renderRoutes(client, '/configuration');
    await user.type(await screen.findByLabelText('Scalar crossing'), 'blocked');
    await user.type(screen.getByLabelText('Sparse array write'), 'blocked');
    const summary = await screen.findByRole('alert');
    expect(summary).toHaveTextContent('crosses a non-container value');
    expect(summary).toHaveTextContent('exceeds the safe array-index limit');
    await user.click(screen.getByRole('button', { name: 'Save canonical revision' }));
    expect(client.patchCanonical).not.toHaveBeenCalled();
  });

  it('does not render empty or cross-version manifest presentations', async () => {
    const emptyClient = createMockApiClient({
      getCoreCapability: vi.fn().mockResolvedValue({
        resolution: { exact_version: '1.13.19', source: 'explicit' },
        support_level: 'native_structured',
        pinned: true,
        quarantined: false,
        presentation: { semantic_facts: [], ui: [] },
      }),
    });
    const first = renderRoutes(emptyClient, '/configuration');
    await screen.findByRole('heading', { name: 'Save is not apply.' });
    await waitFor(() => expect(emptyClient.getCoreCapability).toHaveBeenCalled());
    expect(screen.queryByRole('heading', { name: 'Versioned canonical controls' })).not.toBeInTheDocument();
    first.unmount();

    const wrongVersionClient = createMockApiClient({
      getCoreCapability: vi.fn().mockResolvedValue({
        resolution: { exact_version: '1.13.18', source: 'explicit' },
        support_level: 'native_structured',
        pinned: true,
        quarantined: false,
        presentation: {
          semantic_facts: [{ id: 'old', canonical_path: '/global/old', classification: 'supported' }],
          ui: [{ id: 'old', fact_id: 'old', kind: 'text', label: 'Old-version field' }],
        },
      }),
    });
    renderRoutes(wrongVersionClient, '/configuration');
    await screen.findByRole('heading', { name: 'Save is not apply.' });
    await waitFor(() => expect(wrongVersionClient.getCoreCapability).toHaveBeenCalledWith('1.13.19', expect.any(AbortSignal)));
    expect(screen.queryByLabelText('Old-version field')).not.toBeInTheDocument();
  });

  it('blocks a descriptor whose canonical path uses a non-numeric array token', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient({
      getCoreCapability: vi.fn().mockResolvedValue({
        resolution: { exact_version: '1.13.19', source: 'explicit' },
        support_level: 'native_structured',
        pinned: true,
        quarantined: false,
        presentation: {
          semantic_facts: [{ id: 'bad.array', canonical_path: '/nodes/not-an-index/name', classification: 'supported' }],
          ui: [{ id: 'bad.array', fact_id: 'bad.array', kind: 'text', label: 'Unsafe array field' }],
        },
      }),
    });

    renderRoutes(client, '/configuration');
    const field = await screen.findByLabelText('Unsafe array field');
    await user.type(field, 'blocked');
    const summary = await screen.findByRole('alert');
    expect(summary).toHaveTextContent('uses a non-numeric array index');
    expect(field).toHaveAttribute('aria-invalid', 'true');
    await user.click(screen.getByRole('button', { name: 'Save canonical revision' }));
    expect(client.patchCanonical).not.toHaveBeenCalled();
  });

  it('preserves a versioned form draft when its pointer-patch CAS conflicts', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient({
      patchCanonical: vi.fn().mockRejectedValue(new ApiRequestError('revision changed', {
        status: 412,
        code: 'canonical_revision_conflict',
      })),
    });

    renderRoutes(client, '/configuration');
    await user.click(await screen.findByRole('checkbox', {
      name: /Accept compatible controls for sing-box 1\.13\.19/,
    }));
    const mode = screen.getByLabelText('Routing mode');
    await user.selectOptions(mode, 'direct');
    await user.type(await screen.findByLabelText('Operator note'), 'Unsaved exact draft');
    await user.click(screen.getByRole('button', { name: 'Save canonical revision' }));

    expect(await screen.findByText('Revision conflict')).toBeInTheDocument();
    expect(mode).toHaveValue('direct');
    expect(screen.getByLabelText('Operator note')).toHaveValue('Unsaved exact draft');
    expect(screen.getByRole('button', { name: 'Reload current revision' })).toBeInTheDocument();
  });

  it('manages subscription tokens and removes one-time plaintext on acknowledgement', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient();

    renderRoutes(client, '/subscriptions');

    expect(
      await screen.findByRole('heading', { name: 'Publish only frozen state.' }),
    ).toBeInTheDocument();
    expect(await screen.findByRole('option', { name: /sing-box/ })).toBeInTheDocument();
    await user.selectOptions(screen.getByLabelText('Channel'), 'channel_sing_box');
    await user.click(screen.getByRole('button', { name: 'Issue token' }));

    await waitFor(() => {
      expect(client.createSubscriptionToken).toHaveBeenCalledWith({
        channelID: 'channel_sing_box',
        expiresAt: undefined,
      });
    });
    expect(await screen.findByText('one-time-public-token')).toBeInTheDocument();
    expect(screen.getByText(/cannot be shown again/)).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'I saved it' }));
    expect(screen.queryByText('one-time-public-token')).not.toBeInTheDocument();
  });

  it('shows persisted observability evidence and loads exact record details', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient();

    renderRoutes(client, '/observability');

    expect(
      await screen.findByRole('heading', { name: 'No invented zeroes.' }),
    ).toBeInTheDocument();
    expect(await screen.findByText('Available')).toBeInTheDocument();
    expect(await screen.findByText('2,048 B')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /The exact core process passed/ }));
    await waitFor(() => {
      expect(client.getLog).toHaveBeenCalledWith('log_runtime_ready');
    });
    expect(await screen.findByRole('heading', { name: 'runtime.ready' })).toBeInTheDocument();
  });

  it('explains unavailable metrics without rendering a synthetic zero', async () => {
    const client = createMockApiClient({
      getMetrics: vi.fn().mockResolvedValue({
        available: false,
        reason_code: 'process_only',
        applied_bundle_id: 'bundle_18',
        monitoring_tier: 'process_only',
        collected_at: '2026-08-26T07:34:00Z',
      }),
      getTrafficStatus: vi.fn().mockResolvedValue({
        available: false,
        reason_code: 'process_only',
        applied_bundle_id: 'bundle_18',
        monitoring_tier: 'process_only',
        collected_at: '2026-08-26T07:34:00Z',
      }),
      listTrafficPeriods: vi.fn().mockResolvedValue({ items: [] }),
    });

    renderRoutes(client, '/observability');

    expect(await screen.findByText('Process-only monitoring')).toBeInTheDocument();
    expect(screen.getByText(/without traffic counters/)).toBeInTheDocument();
    expect(screen.queryByText('0 B')).not.toBeInTheDocument();
  });

  it('loads the version rail and queues a verified exact-version install', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient({
      listCatalogAssets: vi.fn().mockResolvedValue(testCatalog),
    });

    renderRoutes(client, '/cores');

    expect(
      await screen.findByRole('heading', { name: 'Follow the version rail.' }),
    ).toBeInTheDocument();
    await waitFor(() => {
      expect(client.getCoreCapability).toHaveBeenCalledWith(
        '1.13.19',
        expect.any(AbortSignal),
      );
    });
    await user.click(await screen.findByRole('button', { name: 'Install' }));
    await waitFor(() => {
      expect(client.installCore).toHaveBeenCalledWith(201);
    });
    expect(await screen.findByText(/Verified install queued/)).toBeInTheDocument();
  });

  it('does not silently select the latest catalog version when no core is running', async () => {
    const user = userEvent.setup();
    const unselectedContext = {
      ...testDashboardContext,
      view: { exactVersion: 'Not selected' },
      running: null,
      capability: {
        level: 'unavailable' as const,
        label: 'No core selected',
        warning: 'Select an exact core version.',
      },
    };
    const client = createMockApiClient({
      getDashboardContext: vi.fn().mockResolvedValue(unselectedContext),
    });

    renderRoutes(client, '/cores');

    const picker = await screen.findByLabelText('Selected exact version');
    expect(picker).toHaveValue('');
    expect(client.getCoreCapability).not.toHaveBeenCalled();

    await screen.findByRole('option', { name: /1\.13\.19/ });
    await user.selectOptions(picker, '1.13.19');
    await waitFor(() => {
      expect(client.getCoreCapability).toHaveBeenCalledWith(
        '1.13.19',
        expect.any(AbortSignal),
      );
    });
  });
});
