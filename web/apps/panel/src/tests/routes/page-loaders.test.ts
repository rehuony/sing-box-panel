import { createElement } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

async function fixture() {
  vi.resetModules();
  const routes = await import('@/routes/page-loaders');
  const reload = vi.fn();
  vi.stubGlobal('window', { location: { reload } });
  return { ...routes, reload };
}

describe('route preloading recovery', () => {
  it('shares an in-flight preload with navigation and keeps successful imports', async () => {
    const routes = await fixture();
    const page = { default: () => createElement('div') };
    let finish!: (value: typeof page) => void;
    const load = vi.spyOn(routes.pageLoaders, 'node-cores').mockImplementation(() =>
      new Promise(resolve => {
        finish = resolve;
      }));
    routes.preloadPage('node-cores');
    routes.preloadPage('node-cores');
    const navigation = routes.loadPage('node-cores');
    finish(page);
    await expect(navigation).resolves.toBe(page);
    await expect(routes.loadPage('node-cores')).resolves.toBe(page);
    expect(load).toHaveBeenCalledOnce();
    expect(routes.reload).not.toHaveBeenCalled();
  });

  it('defers recovery until navigation, including another route with shared CSS', async () => {
    const routes = await fixture();
    const failed = vi.spyOn(routes.pageLoaders, 'node-cores').mockRejectedValue(new Error('CSS failed'));
    const next = vi.spyOn(routes.pageLoaders, 'settings');
    routes.preloadPage('node-cores');
    await Promise.resolve();
    routes.preloadPage('node-cores');
    routes.preloadPage('settings');
    expect(failed).toHaveBeenCalledOnce();
    expect(next).not.toHaveBeenCalled();
    expect(routes.reload).not.toHaveBeenCalled();
    void routes.loadPage('settings');
    expect(routes.reload).toHaveBeenCalledOnce();
    expect(next).not.toHaveBeenCalled();
  });

  it('recovers if the pending preload fails after navigation starts', async () => {
    const routes = await fixture();
    let fail!: (error: Error) => void;
    vi.spyOn(routes.pageLoaders, 'node-cores').mockImplementation(() =>
      new Promise((_, reject) => {
        fail = reject;
      }));
    routes.preloadPage('node-cores');
    void routes.loadPage('node-cores');
    fail(new Error('CSS failed'));
    await vi.waitFor(() => expect(routes.reload).toHaveBeenCalledOnce());
  });

  it('does not create a reload loop for a direct navigation failure', async () => {
    const routes = await fixture();
    vi.spyOn(routes.pageLoaders, 'node-cores').mockRejectedValue(new Error('offline'));
    await expect(routes.loadPage('node-cores')).rejects.toThrow('offline');
    expect(routes.reload).not.toHaveBeenCalled();
  });
});
