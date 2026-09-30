import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const sdk = vi.hoisted(() => ({ load: vi.fn(), get: vi.fn() }));
vi.mock('@fingerprintjs/fingerprintjs', () => ({ load: sdk.load }));

beforeEach(() => {
  vi.resetModules();
  vi.resetAllMocks();
  vi.stubGlobal('window', {});
  sdk.load.mockResolvedValue({ get: sdk.get });
  sdk.get.mockResolvedValue({ visitorId: 'a'.repeat(32), components: { private: 'never transmitted' } });
});
afterEach(() => {
  vi.useRealTimers();
});

describe('login fingerprint collection', () => {
  it('collects once per page with monitoring disabled and returns only the visitor ID', async () => {
    const { loginFingerprint } = await import('@/api/http/login-fingerprint');
    await expect(Promise.all([loginFingerprint(), loginFingerprint()])).resolves.toEqual(['a'.repeat(32), 'a'.repeat(32)]);
    expect(sdk.load).toHaveBeenCalledExactlyOnceWith({ monitoring: false });
    expect(sdk.get).toHaveBeenCalledOnce();
    await expect(loginFingerprint()).resolves.toBe('a'.repeat(32));
    expect(sdk.get).toHaveBeenCalledOnce();
  });

  it.each(['load', 'get', 'invalid ID'])('allows IP-limited login when collection fails: %s', async failure => {
    if (failure === 'load') sdk.load.mockRejectedValue(new Error('blocked'));
    else if (failure === 'get') sdk.get.mockRejectedValue(new Error('unavailable'));
    else sdk.get.mockResolvedValue({ visitorId: 'invalid' });
    const { loginFingerprint } = await import('@/api/http/login-fingerprint');
    await expect(loginFingerprint()).resolves.toBeUndefined();
  });

  it('bounds collection time and keeps the fallback for the rest of the page', async () => {
    vi.useFakeTimers();
    sdk.load.mockReturnValue(new Promise(() => {}));
    const { loginFingerprint } = await import('@/api/http/login-fingerprint');
    const pending = loginFingerprint();
    await vi.advanceTimersByTimeAsync(1500);
    await expect(pending).resolves.toBeUndefined();
    await expect(loginFingerprint()).resolves.toBeUndefined();
  });

  it('skips browser collection outside a browser', async () => {
    vi.stubGlobal('window', undefined);
    const { loginFingerprint } = await import('@/api/http/login-fingerprint');
    await expect(loginFingerprint()).resolves.toBeUndefined();
    expect(sdk.load).not.toHaveBeenCalled();
  });
});
