import { afterEach, describe, expect, it, vi } from 'vitest';

import { createBrowserApiClient } from '@/api/browser-api-client';

const {
  createDemoApiClient,
  createHttpApiClient,
  demoClient,
  demoModuleLoaded,
  httpClient,
} = vi.hoisted(() => ({
  createDemoApiClient: vi.fn(),
  createHttpApiClient: vi.fn(),
  demoClient: { kind: 'demo' },
  demoModuleLoaded: vi.fn(),
  httpClient: { kind: 'http' },
}));

vi.mock('@/api/demo/create-demo-api-client', () => {
  demoModuleLoaded();
  return { createDemoApiClient };
});
vi.mock('@/api/http-api-client', () => ({ createHttpApiClient }));

describe('createBrowserApiClient', () => {
  afterEach(() => {
    vi.unstubAllEnvs();
    vi.resetAllMocks();
  });

  it('uses the same-origin HTTP client in every other mode', async () => {
    vi.stubEnv('MODE', 'development');
    createHttpApiClient.mockReturnValue(httpClient);

    await expect(createBrowserApiClient('/panel')).resolves.toBe(httpClient);
    expect(createHttpApiClient).toHaveBeenCalledWith({ baseUrl: '/panel/api/v1' });
    expect(demoModuleLoaded).not.toHaveBeenCalled();
    expect(createDemoApiClient).not.toHaveBeenCalled();
  });

  it('loads the in-memory client only in demo mode', async () => {
    vi.stubEnv('MODE', 'demo');
    createDemoApiClient.mockReturnValue(demoClient);

    await expect(createBrowserApiClient('/panel')).resolves.toBe(demoClient);
    expect(demoModuleLoaded).toHaveBeenCalledOnce();
    expect(createDemoApiClient).toHaveBeenCalledOnce();
    expect(createHttpApiClient).not.toHaveBeenCalled();
  });
});
