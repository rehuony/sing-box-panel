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
    const load = vi.spyOn(routes.pageLoaders, '/cores').mockImplementation(() =>
      new Promise(resolve => {
        finish = resolve;
      }));
    routes.preloadPage('/cores');
    routes.preloadPage('/cores');
    const navigation = routes.loadPage('/cores');
    finish(page);
    await expect(navigation).resolves.toBe(page);
    await expect(routes.loadPage('/cores')).resolves.toBe(page);
    expect(load).toHaveBeenCalledOnce();
    expect(routes.reload).not.toHaveBeenCalled();
  });

  it('defers recovery until navigation, including another route with shared CSS', async () => {
    const routes = await fixture();
    const failed = vi.spyOn(routes.pageLoaders, '/cores').mockRejectedValue(new Error('CSS failed'));
    const next = vi.spyOn(routes.pageLoaders, '/panel');
    routes.preloadPage('/cores');
    await Promise.resolve();
    routes.preloadPage('/cores');
    routes.preloadPage('/panel');
    expect(failed).toHaveBeenCalledOnce();
    expect(next).not.toHaveBeenCalled();
    expect(routes.reload).not.toHaveBeenCalled();
    void routes.loadPage('/panel');
    expect(routes.reload).toHaveBeenCalledOnce();
    expect(next).not.toHaveBeenCalled();
  });

  it('recovers if the pending preload fails after navigation starts', async () => {
    const routes = await fixture();
    let fail!: (error: Error) => void;
    vi.spyOn(routes.pageLoaders, '/cores').mockImplementation(() =>
      new Promise((_, reject) => {
        fail = reject;
      }));
    routes.preloadPage('/cores');
    void routes.loadPage('/cores');
    fail(new Error('CSS failed'));
    await vi.waitFor(() => expect(routes.reload).toHaveBeenCalledOnce());
  });

  it('does not create a reload loop for a direct navigation failure', async () => {
    const routes = await fixture();
    vi.spyOn(routes.pageLoaders, '/cores').mockRejectedValue(new Error('offline'));
    await expect(routes.loadPage('/cores')).rejects.toThrow('offline');
    expect(routes.reload).not.toHaveBeenCalled();
  });
});
