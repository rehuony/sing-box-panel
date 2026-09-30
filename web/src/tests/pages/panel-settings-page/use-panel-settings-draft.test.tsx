import type { PropsWithChildren } from 'react';

import { describe, expect, it, vi } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';

import type { ApiClient, PanelSettingsSaveResult } from '@/api/api-client';

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
  const hook = renderHook(() => {
    const settings = usePanelSettings();
    return { draft: usePanelSettingsDraft(settings.view ?? initial), settings, theme: useTheme() };
  }, {
    wrapper: ({ children }: PropsWithChildren) => (
      <TestRouter initialEntries={['/panel']}>
        <ApiClientProvider client={client}>
          <ThemeProvider><PanelSettingsProvider>{children}</PanelSettingsProvider></ThemeProvider>
        </ApiClientProvider>
      </TestRouter>
    ),
  });
  await waitFor(() => expect(hook.result.current.settings.view).not.toBeNull());
  return { ...hook, initial, client };
}

describe('panel settings draft', () => {
  it('adopts a background revision while clean and preserves dirty fields and their conflict revision', async () => {
    const client = createMockApiClient({ savePanelSettings: vi.fn().mockRejectedValue(new Error('conflict')) });
    const { result, initial } = await mount(client);
    const next = { ...initial, revision: initial.revision + 1,
      preferences: { ...initial.preferences, appearance: { ...initial.preferences.appearance, color: '#135791' } },
      service: { ...initial.service, base_path: '/remote' } };
    client.getPanelSettings.mockResolvedValue(next);
    act(() => result.current.settings.reload());
    await waitFor(() => {
      expect(result.current.draft.service.base_path).toBe('/remote');
      expect(result.current.draft.preferences.appearance.color).toBe('#135791');
      expect(result.current.draft.dirty).toBe(false);
    });
    act(() => {
      result.current.draft.updateService('base_path', '/local');
      result.current.draft.appearance({ color: '#246802' });
    });
    client.getPanelSettings.mockResolvedValue({ ...next, revision: next.revision + 1,
      preferences: { ...next.preferences, appearance: { ...next.preferences.appearance, color: '#aaaaaa' } } });
    act(() => result.current.settings.reload());
    await waitFor(() => expect(result.current.settings.view?.revision).toBe(next.revision + 1));
    expect(result.current.draft.service.base_path).toBe('/local');
    expect(result.current.draft.preferences.appearance.color).toBe('#246802');
    await act(() => result.current.draft.submit());
    expect(client.savePanelSettings).toHaveBeenCalledWith(expect.objectContaining({ revision: next.revision }));
    expect(result.current.draft.dirty).toBe(true);
  });

  it('submits the revision and hidden fields without ever treating the token display mask as input', async () => {
    const { result, initial, client } = await mount();
    act(() => result.current.draft.updateService('catalog_refresh_interval_hours', 24));
    await act(() => result.current.draft.submit());
    expect(client.savePanelSettings).toHaveBeenCalledExactlyOnceWith({
      revision: initial.revision, preferences: initial.preferences,
      service: { ...initial.service, catalog_refresh_interval_hours: 24 },
      github_token: '', clear_github_token: false, credentials: undefined,
    });
  });
  it('keeps failed edits and leaves the accepted settings unchanged', async () => {
    const client = createMockApiClient({ savePanelSettings: vi.fn().mockRejectedValue(new Error('conflict')) });
    const { result, initial } = await mount(client);
    act(() => {
      result.current.draft.updateService('base_path', '/draft');
      result.current.draft.appearance({ color: '#123456' });
      result.current.draft.setEmail('draft@example.com');
      result.current.draft.setPassword('draft-password-123');
    });
    await act(() => result.current.draft.submit());
    expect(result.current.draft.service.base_path).toBe('/draft');
    expect(result.current.draft.email).toBe('draft@example.com');
    expect(result.current.draft.password).toBe('draft-password-123');
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
    const pending = deferred<PanelSettingsSaveResult>();
    const client = createMockApiClient({ savePanelSettings: vi.fn().mockReturnValue(pending.promise) });
    const { result, initial } = await mount(client);
    act(() => {
      result.current.draft.setGithub('test-github');
      result.current.draft.setPassword('test-password-123');
    });
    let saving!: Promise<void>;
    act(() => {
      saving = result.current.draft.submit();
    });
    await act(() => result.current.draft.submit());
    expect(client.savePanelSettings).toHaveBeenCalledOnce();
    expect(result.current.draft.saving).toBe(true);
    await act(async () => {
      pending.resolve({ settings: { ...initial, revision: initial.revision + 1 }, reauthentication_required: false });
      await saving;
    });
    expect(result.current.draft.github).toBe('');
    expect(result.current.draft.password).toBe('');
    expect(result.current.draft.saving).toBe(false);
  });
  it('keeps one draft across categories and accepts restored settings atomically', async () => {
    const { result, initial } = await mount();
    act(() => {
      result.current.draft.updateService('base_path', '/draft');
      result.current.draft.setCategory('maintenance');
      result.current.draft.setGithub('pending-token');
      result.current.draft.setEmail('draft@example.com');
      result.current.draft.setPassword('draft-password-123');
    });
    expect(result.current.draft.service.base_path).toBe('/draft');
    const restored = { ...initial, revision: initial.revision + 1, service: { ...initial.service, base_path: '/restored' } };
    await act(() => result.current.draft.restored(restored));
    await waitFor(() => expect(result.current.settings.view).toEqual(restored));
    expect(result.current.draft.service).toEqual(restored.service);
    expect(result.current.draft.email).toBe(restored.admin_email);
    expect(result.current.draft.password).toBe('');
    expect(result.current.draft.github).toBe('');
  });
});
