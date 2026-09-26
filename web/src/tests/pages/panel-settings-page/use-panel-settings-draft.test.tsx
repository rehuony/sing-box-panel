import type { PropsWithChildren } from 'react';

import { describe, expect, it, vi } from 'vitest';
import { act, renderHook } from '@testing-library/react';

import type { ApiClient, PanelSettingsView } from '@/api/api-client';

import '@/i18n';
import { ThemeProvider } from '@/theme';
import { deferred } from '@/tests/deferred';
import { TestRouter } from '@/tests/test-router';
import { useTheme } from '@/theme/theme-context';
import { ApiClientProvider } from '@/api/api-client-context';
import { usePanelSettings } from '@/stores/panel-settings.store';
import { createMockApiClient } from '@/tests/api/mock-api-client';
import { PanelSettingsProvider } from '@/stores/panel-settings-provider';
import { usePanelSettingsDraft } from '@/pages/panel-settings-page/use-panel-settings-draft';

async function mount(client: ApiClient = createMockApiClient()) {
  const initial = await client.getPanelSettings();
  const hook = renderHook(() => ({
    draft: usePanelSettingsDraft(initial), settings: usePanelSettings(), theme: useTheme(),
  }), {
    wrapper: ({ children }: PropsWithChildren) => (
      <TestRouter initialEntries={['/panel']}>
        <ApiClientProvider client={client}>
          <ThemeProvider><PanelSettingsProvider>{children}</PanelSettingsProvider></ThemeProvider>
        </ApiClientProvider>
      </TestRouter>
    ),
  });
  await act(async () => {});
  return { ...hook, initial, client };
}

describe('panel settings draft', () => {
  it('submits the revision and hidden fields without ever treating the token display mask as input', async () => {
    const { result, initial, client } = await mount();
    act(() => result.current.draft.updateService('catalog_refresh_interval_hours', 24));
    await act(() => result.current.draft.submit());
    expect(client.savePanelSettings).toHaveBeenCalledExactlyOnceWith({
      revision: initial.revision, preferences: initial.preferences,
      service: { ...initial.service, catalog_refresh_interval_hours: 24 },
      github_token: '', clear_github_token: false, management_token: undefined,
    });
  });
  it('keeps failed edits and leaves the accepted settings unchanged', async () => {
    const client = createMockApiClient({ savePanelSettings: vi.fn().mockRejectedValue(new Error('conflict')) });
    const { result, initial } = await mount(client);
    act(() => {
      result.current.draft.updateService('base_path', '/draft');
      result.current.draft.appearance({ color: '#123456' });
    });
    await act(() => result.current.draft.submit());
    expect(result.current.draft.service.base_path).toBe('/draft');
    expect(result.current.draft.preferences.appearance.color).toBe('#123456');
    expect(result.current.draft.dirty).toBe(true);
    expect(result.current.settings.view).toEqual(initial);
    expect(result.current.draft.saving).toBe(false);
  });
  it('prevents saves for invalid fields and selects their owning category', async () => {
    const { result, client } = await mount();
    act(() => {
      result.current.draft.updateService('traffic_period_months', 0);
      result.current.draft.setCategory('interface');
    });
    await act(() => result.current.draft.submit());
    expect(result.current.draft.invalidField).toBe('traffic-period');
    expect(result.current.draft.category).toBe('traffic');
    expect(client.savePanelSettings).not.toHaveBeenCalled();
  });
  it('locks duplicate submission until success and clears sensitive input afterwards', async () => {
    const pending = deferred<PanelSettingsView>();
    const client = createMockApiClient({ savePanelSettings: vi.fn().mockReturnValue(pending.promise) });
    const { result, initial } = await mount(client);
    act(() => {
      result.current.draft.setGithub('test-github');
      result.current.draft.setToken('test-token');
    });
    let saving!: Promise<void>;
    act(() => {
      saving = result.current.draft.submit(undefined, 'test-token');
    });
    await act(() => result.current.draft.submit());
    expect(client.savePanelSettings).toHaveBeenCalledOnce();
    expect(result.current.draft.saving).toBe(true);
    await act(async () => {
      pending.resolve({ ...initial, revision: initial.revision + 1 });
      await saving;
    });
    expect(result.current.draft.github).toBe('');
    expect(result.current.draft.token).toBe('');
    expect(result.current.draft.saving).toBe(false);
  });
  it('keeps one draft across categories and accepts restored settings atomically', async () => {
    const { result, initial } = await mount();
    act(() => {
      result.current.draft.updateService('base_path', '/draft');
      result.current.draft.setCategory('maintenance');
      result.current.draft.setGithub('pending-token');
    });
    expect(result.current.draft.service.base_path).toBe('/draft');
    const restored = { ...initial, revision: initial.revision + 1, service: { ...initial.service, base_path: '/restored' } };
    await act(() => result.current.draft.restored(restored));
    expect(result.current.settings.view).toEqual(restored);
    expect(result.current.draft.service).toEqual(restored.service);
    expect(result.current.draft.github).toBe('');
  });
});
