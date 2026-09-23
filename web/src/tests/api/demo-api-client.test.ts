import { afterEach, describe, expect, it, vi } from 'vitest';

import { createDemoData, demoMetrics } from '@/api/demo/demo-data';
import { createDemoApiClient } from '@/api/demo/create-demo-api-client';

describe('createDemoApiClient', () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it('starts authenticated with representative 1.14 core and Schema data', async () => {
    const client = createDemoApiClient();

    await expect(client.getSession()).resolves.toEqual({ displayName: 'Demo administrator' });

    const cores = await client.listCoreArtifacts();
    const current = cores.items.find((item) => item.exact_version === '1.14.0');
    const legacy = cores.items.find((item) => item.exact_version === '1.13.19');

    expect(current).toMatchObject({
      arch: 'amd64',
      id: 'core_demo_114',
    });
    expect(legacy).toBeDefined();
    await expect(client.getConfigurationSupport(current!.id)).resolves.toEqual({
      exact_version: '1.14.0',
      structured: true,
    });
    await expect(client.getConfigurationSupport(legacy!.id)).resolves.toMatchObject({
      exact_version: '1.13.19',
      structured: true,
    });

    const contract = await client.getConfigurationSchema(current!.id);
    expect(contract).toMatchObject({
      exact_version: '1.14.0',
      schema: {
        additionalProperties: false,
        type: 'object',
      },
    });
    expect(contract.schema_sha256).toMatch(/^[a-f\d]{64}$/u);
  });

  it('honors an AbortSignal while a response is in flight', async () => {
    const client = createDemoApiClient();
    const controller = new AbortController();

    const request = client.getConfigurationFile(controller.signal);
    controller.abort();

    await expect(request).rejects.toMatchObject({ name: 'AbortError' });
  });

  it('uses saved traffic quotas in metrics, traffic status and an already-open stream', async () => {
    vi.useFakeTimers();
    const client = createDemoApiClient();
    const controller = new AbortController();
    const stream = client.streamMetrics(controller.signal)[Symbol.asyncIterator]();
    let settings = await client.getPanelSettings();
    const initial = (await stream.next()).value!.metrics;
    expect(initial.quota_bytes).toBe(settings.preferences.traffic_quota_gib! * 2 ** 30);

    try {
      for (const quotaGiB of [2, null, 0, 250]) {
        const before = await client.getMetrics();
        settings = await client.savePanelSettings({
          revision: settings.revision,
          preferences: { ...settings.preferences, traffic_quota_gib: quotaGiB },
        });
        const quotaBytes = quotaGiB ? quotaGiB * 2 ** 30 : undefined;
        const metrics = await client.getMetrics();
        expect(metrics.quota_bytes).toBe(quotaBytes);
        expect(metrics.current_traffic_period).toEqual(before.current_traffic_period);
        expect((await client.getTrafficStatus()).quota_bytes).toBe(quotaBytes);
        const next = stream.next();
        await vi.advanceTimersByTimeAsync(2000);
        expect((await next).value!.metrics.quota_bytes).toBe(quotaBytes);
        expect((await client.getPanelSettings()).preferences.traffic_quota_gib).toBe(quotaGiB);
      }
    } finally {
      controller.abort();
      await stream.return?.();
    }
  });

  it('derives quota exhaustion from both traffic counters and preserves unlimited quotas', () => {
    const data = createDemoData();
    data.trafficPeriods[0].inbound_bytes = 768 * 2 ** 20;
    data.trafficPeriods[0].outbound_bytes = 256 * 2 ** 20;
    expect(demoMetrics(data, 1).quota_exceeded).toBe(true);
    expect(demoMetrics(data, 2).quota_exceeded).toBe(false);
    for (const quota of [0, null]) {
      const snapshot = demoMetrics(data, quota);
      expect(snapshot.quota_bytes).toBeUndefined();
      expect(snapshot.quota_exceeded).toBe(false);
    }
  });

  it('fills the full 24-hour chart and keeps overlapping samples stable as the window advances', async () => {
    const client = createDemoApiClient();
    const filter = {
      from: '2026-09-19T08:00:00Z',
      to: '2026-09-20T08:00:00Z',
      bucketSeconds: 300,
    };
    const history = await client.getMetricsHistory(filter);
    const advanced = await client.getMetricsHistory({
      ...filter,
      from: '2026-09-19T08:05:00Z',
      to: '2026-09-20T08:05:00Z',
    });

    expect(history.buckets).toHaveLength(288);
    expect(Date.parse(history.buckets.at(-1)!.to)).toBe(Date.parse(filter.to));
    expect(advanced.buckets.slice(0, -1)).toEqual(history.buckets.slice(1));
    expect(Date.parse(advanced.buckets.at(-1)!.to)).toBe(Date.parse(advanced.to));
  });

  it.each([true, false])('rotates an enabled=%s key without changing its state or usage', async (enabled) => {
    const client = createDemoApiClient();
    const { items: [key] } = await client.listSubscriptionTokens();
    const original = await client.setSubscriptionTokenEnabled(key.id, enabled);
    const rotation = await client.rotateSubscriptionToken(key.id);

    expect(rotation.created).toEqual({
      ...original,
      id: expect.any(String),
      created_at: expect.any(String),
      expires_at: original.expires_at,
      revoked_at: undefined,
    });
    expect(rotation.created.id).not.toBe(original.id);
    expect(rotation.revoked).toMatchObject({ id: original.id, active: false, revoked_at: expect.any(String) });
    expect(rotation.token).toBeTruthy();
    expect(() => client.rotateSubscriptionToken(original.id)).toThrow('revoked');
    await expect(client.setSubscriptionTokenEnabled(rotation.created.id, true))
      .resolves
      .toMatchObject({ active: true });
  });

  it('rotates expired keys while retaining their expiry and inactive state', async () => {
    vi.useFakeTimers();
    const client = createDemoApiClient();
    const expiresAt = new Date(Date.now() + 60_000).toISOString();
    const { metadata } = await client.createSubscriptionToken({ label: 'Expiring', expiresAt, downloadLimit: 3 });
    await vi.advanceTimersByTimeAsync(60_000);

    const rotation = await client.rotateSubscriptionToken(metadata.id);
    expect(rotation.created).toMatchObject({ enabled: true, active: false, expires_at: expiresAt, download_limit: 3 });
  });

  it('enables manual imports on the reported platform without a trust lifecycle', async () => {
    vi.useFakeTimers();
    const client = createDemoApiClient();
    const platform = (await client.getSystemStatus()).platform!;
    expect(await client.enableCore('core_demo_113')).toMatchObject({ enabled_core: { core_artifact_id: 'core_demo_113' } });
    await vi.advanceTimersByTimeAsync(700);

    for (const architecture of ['amd64', 'arm64'] as const) {
      const result = await client.importCoreArchive({
        archive: new File(['archive'], 'sing-box.tar.gz'),
        exactVersion: '1.14.0', sourceDescription: 'Local build', variant: 'musl', architecture,
      });
      await vi.advanceTimersByTimeAsync(700);
      const imported = (await client.listCoreArtifacts()).items.find(
        (core) => core.source_kind === 'user_verified' && core.arch === architecture,
      )!;
      expect(result.id).toBe(imported.id);
      expect(imported).not.toHaveProperty('verification_state');
      if (architecture !== platform.arch) {
        expect(() => client.enableCore(imported.id)).toThrow('match the demo platform');
        continue;
      }
      expect((await client.enableCore(imported.id)).enabled_core?.core_artifact_id).toBe(imported.id);
      await vi.advanceTimersByTimeAsync(700);
      await expect(client.getRuntimeStatus()).resolves.toMatchObject({
        observation_state: 'running', running: { core_artifact_id: imported.id },
      });
    }
  });

  it('saves one editable configuration with numeric CAS and invalid drafts', async () => {
    const client = createDemoApiClient();
    const initial = await client.getConfigurationFile();
    const draft = await client.saveConfigurationFile({ revision: initial.revision, content: '{' });
    expect(draft).toMatchObject({ revision: initial.revision + 1, content: '{', syntax_valid: false });
    expect(draft.canonical_revision_id).toBeUndefined();
    await expect(client.saveConfigurationFile({ revision: initial.revision, content: '{}' })).rejects.toThrow();
    const saved = await client.saveConfigurationFile({ revision: draft.revision, content: '{"log":{"level":"debug"}}' });
    expect(saved.syntax_valid).toBe(true);
    expect(saved.canonical_revision_id).toBeTruthy();
    expect(await client.getConfigurationFile()).toEqual(saved);
  });

  it('preserves integers outside the JavaScript safe range in Advanced JSON', async () => {
    const client = createDemoApiClient();
    const initial = await client.getConfigurationFile();
    const documentJSON = '{"experimental":{"cache_file":{"cache_id":9007199254740993}}}';

    const saved = await client.saveConfigurationFile({ revision: initial.revision, content: documentJSON });
    const current = await client.getConfigurationFile();

    expect(saved.content).toBe(documentJSON);
    expect(current.content).toBe(documentJSON);
  });

  it('reads and replaces node grants with stable, de-duplicated node keys', async () => {
    const client = createDemoApiClient();
    const catalog = await client.getSubscriptionNodeCatalog();
    const initial = await client.getSubscriptionUserGrants('user_demo_primary');
    const selectedKeys = catalog.nodes.slice(0, 2).map((node) => node.key);

    expect(initial.grants).toEqual([
      'source_demo_remote:0',
      'source_demo_remote:1',
      'source_demo_local:0',
    ]);
    expect(new Set(catalog.nodes.map((node) => node.key)).size).toBe(catalog.nodes.length);

    const replaced = await client.replaceSubscriptionUserGrants(
      initial.user.id,
      [selectedKeys[0]!, selectedKeys[0]!, selectedKeys[1]!],
      initial.user.updated_at,
    );

    expect(replaced.grants).toEqual(selectedKeys);
    await expect(client.getSubscriptionUserGrants(initial.user.id)).resolves.toMatchObject({
      grants: selectedKeys,
    });
  });

  it('uses the returned log cursor to load the next older page without overlap', async () => {
    const client = createDemoApiClient();
    const newestPage = await client.listLogs({ limit: 2 });

    expect(newestPage.items.map((item) => item.id)).toEqual([
      'log_demo_runtime',
      'log_demo_config',
    ]);
    expect(newestPage.next).toEqual({
      id: newestPage.items[1]!.id,
      time: newestPage.items[1]!.time,
    });

    const olderPage = await client.listLogs({
      afterID: newestPage.next!.id,
      afterTime: newestPage.next!.time,
      limit: 2,
    });

    expect(olderPage.items.map((item) => item.id)).toEqual(['log_demo_login', 'log_demo_catalog']);
    expect(olderPage.next).toBeUndefined();
    expect(olderPage.items).not.toEqual(expect.arrayContaining(newestPage.items));
  });

  it('selects a version while stopped and starts that version later', async () => {
    vi.useFakeTimers();
    const client = createDemoApiClient();
    await client.stopRuntime();
    await vi.advanceTimersByTimeAsync(700);
    const stopped = await client.getRuntimeStatus();
    expect(stopped.enabled_core?.core_artifact_id).toBe('core_demo_114');
    await client.enableCore('core_demo_113');
    await vi.advanceTimersByTimeAsync(700);
    expect(await client.getRuntimeStatus()).toMatchObject({
      observation_state: 'stopped', desired_running: false, running: undefined,
      enabled_core: { core_artifact_id: 'core_demo_113', exact_core_version: '1.13.19' },
    });
    await client.startRuntime();
    await vi.advanceTimersByTimeAsync(700);
    expect((await client.getRuntimeStatus()).running?.core_artifact_id).toBe('core_demo_113');
  });

  it.each(['running', 'stopped'])('disables the selected version while %s and requires selection before starting again', async (state) => {
    vi.useFakeTimers();
    const client = createDemoApiClient();
    if (state === 'stopped') {
      await client.stopRuntime();
      await vi.advanceTimersByTimeAsync(700);
    }
    expect(() => client.disableCore('core_demo_113')).toThrow('no longer enabled');
    await client.disableCore('core_demo_114');
    await vi.advanceTimersByTimeAsync(700);
    expect(await client.getRuntimeStatus()).toMatchObject({
      observation_state: 'stopped', enabled_core: undefined, running: undefined,
      desired_running: false, applied_bundle_id: undefined, rollback_bundle_id: undefined,
    });
    expect(() => client.startRuntime()).toThrow('No core version');
    expect(() => client.restartRuntime()).toThrow('No core version');
    await client.enableCore('core_demo_113');
    await vi.advanceTimersByTimeAsync(700);
    expect((await client.getRuntimeStatus()).enabled_core?.core_artifact_id).toBe('core_demo_113');
    await client.startRuntime();
    await vi.advanceTimersByTimeAsync(700);
    expect((await client.getRuntimeStatus()).running?.core_artifact_id).toBe('core_demo_113');
  });

  it('completes stop, start, and restart operations and rotates the running process identity', async () => {
    vi.useFakeTimers({ now: new Date('2026-09-01T08:00:00.000Z') });
    const client = createDemoApiClient();
    const initialRuntime = await client.getRuntimeStatus();
    const initialToken = initialRuntime.running!.process_start_token;

    expect(await client.stopRuntime()).toMatchObject({ observation_state: 'stopped' });
    await expect(client.getRuntimeStatus()).resolves.toMatchObject({
      desired_running: false,
      observation_state: 'stopped',
      running: undefined,
    });

    await client.startRuntime();
    const startedRuntime = await client.getRuntimeStatus();
    expect(startedRuntime.running?.process_start_token).not.toBe(initialToken);

    await client.restartRuntime();
    const restartedRuntime = await client.getRuntimeStatus();

    expect(restartedRuntime).toMatchObject({
      desired_running: true,
      observation_state: 'running',
    });
    expect(restartedRuntime.running?.process_start_token).not.toBe(
      startedRuntime.running?.process_start_token,
    );
  });
});
