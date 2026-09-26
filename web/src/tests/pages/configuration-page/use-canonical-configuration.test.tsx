import type { PropsWithChildren } from 'react';

import { describe, expect, it, vi } from 'vitest';
import { act, renderHook } from '@testing-library/react';

import type { ApiClient, ConfigurationFile } from '@/api/api-client';

import '@/i18n';
import { deferred } from '@/tests/deferred';
import { TestRouter } from '@/tests/test-router';
import { ApiRequestError } from '@/api/api-client';
import { ApiClientProvider } from '@/api/api-client-context';
import { createMockApiClient } from '@/tests/api/mock-api-client';
import { useCanonicalConfiguration } from '@/pages/configuration-page/use-canonical-configuration';
import { ConfigurationSessionContext, createConfigurationSessionStore } from '@/stores/configuration-session.store';

const initial: ConfigurationFile = { revision: 4, content: '{"log":{"level":"info"}}', syntax_valid: true };

async function mount(client: ApiClient, store = createConfigurationSessionStore()) {
  const hook = renderHook(useCanonicalConfiguration, {
    wrapper: ({ children }: PropsWithChildren) => (
      <TestRouter>
        <ApiClientProvider client={client}>
          <ConfigurationSessionContext value={store}>{children}</ConfigurationSessionContext>
        </ApiClientProvider>
      </TestRouter>
    ),
  });
  await act(async () => {});
  return { ...hook, store };
}

describe('canonical configuration lifecycle', () => {
  it('saves with the current file revision and reloads persisted content', async () => {
    let file = initial;
    const client = createMockApiClient({
      getConfigurationFile: vi.fn(async () => file),
      saveConfigurationFile: vi.fn(async input => {
        file = { ...file, revision: input.revision + 1, content: input.content };
        return file;
      }),
    });
    const first = await mount(client);
    act(() => first.result.current.update(draft => ({ ...draft, log: { level: 'debug' } })));
    const content = first.result.current.state.content;
    await act(() => first.result.current.save());
    expect(client.saveConfigurationFile).toHaveBeenCalledExactlyOnceWith({ revision: 4, content });
    expect(first.result.current.dirty).toBe(false);
    first.unmount();
    const second = await mount(client, first.store);
    expect(client.getConfigurationFile).toHaveBeenCalledTimes(2);
    expect(second.result.current.state).toMatchObject({ status: 'ready', content, file: { revision: 5 } });
  });

  it.each(['{}', '{"log":'])('saves exact text even for an empty or incomplete document: %s', async content => {
    const client = createMockApiClient({
      getConfigurationFile: vi.fn().mockResolvedValue({ ...initial, revision: 0, content }),
    });
    const { result } = await mount(client);
    await act(() => result.current.save());
    expect(client.saveConfigurationFile).toHaveBeenCalledExactlyOnceWith({ revision: 0, content });
  });

  it('preserves unknown fields and numeric spelling through structured and text edits', async () => {
    const content = '{"future":{"large":900719925474099312345,"threshold":4.2000e+99},"log":{"level":"info"}}';
    const client = createMockApiClient({ getConfigurationFile: vi.fn().mockResolvedValue({ ...initial, content }) });
    const { result } = await mount(client);
    act(() => result.current.update(draft => ({ ...draft, log: { level: 'debug' } })));
    expect(result.current.state.content).toContain('900719925474099312345');
    expect(result.current.state.content).toContain('4.2000e+99');
    act(() => result.current.updateText(result.current.state.content.replace('"debug"', '"warn"')));
    expect(result.current.draft?.log).toEqual({ level: 'warn' });
    await act(() => result.current.save());
    expect(client.saveConfigurationFile.mock.calls[0][0].content).toBe(result.current.state.content);
  });

  it.each([
    new Error('offline'),
    new ApiRequestError('Changed elsewhere', { status: 412, code: 'configuration_file_conflict' }),
  ])('retains the unsaved draft and CAS revision after %s', async error => {
    const client = createMockApiClient({
      getConfigurationFile: vi.fn().mockResolvedValue(initial),
      saveConfigurationFile: vi.fn().mockRejectedValue(error),
    });
    const { result } = await mount(client);
    act(() => result.current.updateText('{"future":true}'));
    await act(async () => expect(await result.current.save()).toBeNull());
    expect(result.current.state).toMatchObject({ content: '{"future":true}', file: initial });
    expect(result.current.dirty).toBe(true);
    expect(result.current.saving).toBe(false);
    act(() => result.current.reset());
    expect(result.current.state.content).toBe(initial.content);
  });

  it('locks updates while saving and adopts the returned revision only on success', async () => {
    const pending = deferred<ConfigurationFile>();
    const client = createMockApiClient({
      getConfigurationFile: vi.fn().mockResolvedValue(initial),
      saveConfigurationFile: vi.fn().mockReturnValue(pending.promise),
    });
    const { result } = await mount(client);
    act(() => result.current.updateText('{"log":{}}'));
    let saving!: ReturnType<typeof result.current.save>;
    act(() => {
      saving = result.current.save();
    });
    expect(result.current.saving).toBe(true);
    act(() => {
      result.current.updateText('ignored');
      result.current.update(() => ({ ignored: true }));
    });
    expect(result.current.state.content).toBe('{"log":{}}');
    await act(async () => {
      pending.resolve({ ...initial, revision: 5, content: '{"log":{}}' });
      await saving;
    });
    expect(result.current.saving).toBe(false);
    expect(result.current.state.file?.revision).toBe(5);
  });

  it('restores a dirty session and discards it before the next mount', async () => {
    const store = createConfigurationSessionStore();
    store.getState().replaceDraft({ file: initial, content: '{"unsaved":' });
    const client = createMockApiClient({ getConfigurationFile: vi.fn().mockResolvedValue(initial) });
    const first = await mount(client, store);
    expect(first.result.current.editorError).not.toBeNull();
    expect(client.getConfigurationFile).not.toHaveBeenCalled();
    act(() => first.result.current.reset());
    first.unmount();
    const second = await mount(client, store);
    expect(second.result.current.state.content).toBe(initial.content);
    expect(second.result.current.dirty).toBe(false);
  });

  it('aborts loading and ignores a late result after unmount', async () => {
    const pending = deferred<ConfigurationFile>();
    const client = createMockApiClient({ getConfigurationFile: vi.fn().mockReturnValue(pending.promise) });
    const { result, unmount, store } = await mount(client);
    expect(result.current.state.status).toBe('loading');
    unmount();
    expect(client.getConfigurationFile.mock.calls[0][0]?.aborted).toBe(true);
    await act(async () => pending.resolve(initial));
    expect(store.getState().draft).toBeNull();
  });

  it('exposes a failed read without creating a saveable draft', async () => {
    const error = new Error('read failed');
    const client = createMockApiClient({ getConfigurationFile: vi.fn().mockRejectedValue(error) });
    const { result } = await mount(client);
    expect(result.current.state).toMatchObject({ status: 'error', error });
    await act(() => result.current.save());
    expect(client.saveConfigurationFile).not.toHaveBeenCalled();
  });
});
