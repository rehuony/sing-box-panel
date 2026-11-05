import { describe, expect, it } from 'vitest';

import { createConfigurationSessionStore } from '@/stores/configuration-session.store';

const file = {
  revision: 1,
  content: '{"log":{"level":"info"}}',
  syntax_valid: true,
};

describe('configuration session store', () => {
  it('prefers the enabled version, retains a valid choice, and falls back when it disappears', () => {
    const store = createConfigurationSessionStore();
    expect(store.getState().selectedVersion).toBeNull();
    store.getState().reconcileVersion(['1.14.0', '1.13.19'], '1.13.19');
    expect(store.getState().selectedVersion).toBe('1.13.19');

    store.getState().setSelectedVersion('1.14.0');
    store.getState().reconcileVersion(['1.15.0', '1.14.0'], '1.15.0');
    expect(store.getState().selectedVersion).toBe('1.14.0');

    store.getState().reconcileVersion(['1.15.0'], '1.15.0');
    expect(store.getState().selectedVersion).toBe('1.15.0');
    store.getState().reconcileVersion([], undefined);
    expect(store.getState().selectedVersion).toBeNull();

    const nextSession = createConfigurationSessionStore();
    nextSession.getState().reconcileVersion(['1.15.0', '1.14.0'], '1.13.19');
    expect(nextSession.getState().selectedVersion).toBe('1.15.0');
  });

  it('updates one draft atomically and resets it to the saved baseline', () => {
    const store = createConfigurationSessionStore();
    store.getState().replaceDraft({ file, content: file.content });
    store.getState().updateDraftContent(content => content.replace('info', 'debug'));
    expect(store.getState().draft?.content).toContain('debug');
    store.getState().resetDraft();
    expect(store.getState().draft?.content).toBe(file.content);
  });
});
