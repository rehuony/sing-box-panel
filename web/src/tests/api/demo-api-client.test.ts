import { afterEach, describe, expect, it, vi } from 'vitest';

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
      structured: false,
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

    const request = client.getCanonical(controller.signal);
    controller.abort();

    await expect(request).rejects.toMatchObject({ name: 'AbortError' });
  });

  it('enables manual imports on the reported platform without a trust lifecycle', async () => {
    vi.useFakeTimers();
    const client = createDemoApiClient();
    const platform = (await client.getSystemStatus()).platform!;
    const matchingTask = await client.enableCore('core_demo_113');
    await vi.advanceTimersByTimeAsync(700);
    await expect(client.getTask(matchingTask.id)).resolves.toMatchObject({ status: 'succeeded' });

    for (const architecture of ['amd64', 'arm64'] as const) {
      const task = await client.importCoreArchive({
        archive: new File(['archive'], 'sing-box.tar.gz'),
        exactVersion: '1.14.0', sourceDescription: 'Local build', variant: 'plain', architecture,
      });
      await vi.advanceTimersByTimeAsync(700);
      await expect(client.getTask(task.id)).resolves.toMatchObject({ status: 'succeeded' });
      const imported = (await client.listCoreArtifacts()).items.find(
        (core) => core.source_kind === 'user_verified' && core.arch === architecture,
      )!;
      expect(imported).not.toHaveProperty('verification_state');
      if (architecture !== platform.arch) {
        expect(() => client.enableCore(imported.id)).toThrow('match the demo platform');
        continue;
      }
      const enabled = await client.enableCore(imported.id);
      await vi.advanceTimersByTimeAsync(700);
      await expect(client.getTask(enabled.id)).resolves.toMatchObject({ status: 'succeeded' });
      await expect(client.getRuntimeStatus()).resolves.toMatchObject({
        observation_state: 'running', running: { core_artifact_id: imported.id },
      });
    }
  });

  it('persists replacement and JSON Pointer patch mutations as new canonical revisions', async () => {
    const client = createDemoApiClient();
    const initial = await client.getCanonical();
    const replacementDocument = {
      ...initial.document,
      experimental: { cache_file: { enabled: true } },
    };

    const replaced = await client.replaceCanonical(JSON.stringify(replacementDocument), initial.id);
    expect(replaced).toMatchObject({
      no_change: false,
      revision: {
        parent_id: initial.id,
        sequence: initial.sequence + 1,
      },
    });
    expect(replaced.task_id).toMatch(/^task_demo_/u);

    const patched = await client.patchCanonical(
      [
        { op: 'set', path: '/log/level', value_json: '"debug"' },
        { op: 'unset', path: '/dns/strategy' },
      ],
      replaced.revision.id,
    );
    const current = await client.getCanonical();

    expect(patched.revision.id).toBe(current.id);
    expect(current.sequence).toBe(initial.sequence + 2);
    expect(current.document).toMatchObject({
      experimental: { cache_file: { enabled: true } },
      log: { level: 'debug', timestamp: true },
    });
    expect(current.document.dns).not.toHaveProperty('strategy');
  });

  it('preserves integers outside the JavaScript safe range in Advanced JSON', async () => {
    const client = createDemoApiClient();
    const initial = await client.getCanonical();
    const documentJSON = '{"experimental":{"cache_file":{"cache_id":9007199254740993}}}';

    const saved = await client.replaceCanonical(documentJSON, initial.id);
    const current = await client.getCanonical();

    expect(saved.revision.document_json).toBe(documentJSON);
    expect(current.document_json).toBe(documentJSON);
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

  it('settles stop, start, and restart tasks and rotates the running process identity', async () => {
    vi.useFakeTimers({ now: new Date('2026-09-01T08:00:00.000Z') });
    const client = createDemoApiClient();
    const initialRuntime = await client.getRuntimeStatus();
    const initialToken = initialRuntime.running!.process_start_token;

    const stopTask = await client.stopRuntime();
    expect(stopTask.status).toBe('queued');
    await vi.advanceTimersByTimeAsync(350);
    await expect(client.getTask(stopTask.id)).resolves.toMatchObject({ status: 'running' });
    await vi.advanceTimersByTimeAsync(350);
    await expect(client.getTask(stopTask.id)).resolves.toMatchObject({ status: 'succeeded' });
    await expect(client.getRuntimeStatus()).resolves.toMatchObject({
      desired_running: false,
      observation_state: 'stopped',
      running: undefined,
    });

    const startTask = await client.startRuntime();
    await vi.advanceTimersByTimeAsync(700);
    await expect(client.getTask(startTask.id)).resolves.toMatchObject({ status: 'succeeded' });
    const startedRuntime = await client.getRuntimeStatus();
    expect(startedRuntime.running?.process_start_token).not.toBe(initialToken);

    const restartTask = await client.restartRuntime();
    await vi.advanceTimersByTimeAsync(700);
    await expect(client.getTask(restartTask.id)).resolves.toMatchObject({ status: 'succeeded' });
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
