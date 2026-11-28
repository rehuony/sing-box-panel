import { beforeAll } from 'vitest';

// Tests run once the language's translations have loaded, as the application
// renders. The module loads here, after a test file has registered its mocks,
// so a mocked module is never also loaded unmocked beside them.
beforeAll(async () => {
  const { i18nReady } = await import('@/i18n');
  await i18nReady;
});
