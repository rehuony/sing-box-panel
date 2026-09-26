// @vitest-environment jsdom
import { build } from 'vite';
import { resolve } from 'node:path';
import { runInNewContext } from 'node:vm';
import { describe, expect, it, vi } from 'vitest';

// Use the installed bundler's real preload helper, including its shared CSS cache.
// Only the page bodies are fixtures; the route loader is the production source.
describe('production CSS preload recovery', () => {
  it('reloads on navigation after Vite retains a failed stylesheet preload', async () => {
    const loaderPath = resolve('src/routes/page-loaders.ts');
    const result = await build({
      configFile: false,
      publicDir: false,
      logLevel: 'silent',
      base: './',
      plugins: [{
        name: 'preload-recovery-fixture',
        resolveId(id) {
          if (id.startsWith('virtual:') || id.startsWith('@/pages/')) return id;
        },
        load(id) {
          if (id === 'virtual:entry') {
            return `import { preloadPage, loadPage } from ${JSON.stringify(loaderPath)};
              window.routes = { preloadPage, loadPage };`;
          }
          if (id === 'virtual:page.css') return '.fixture { color: red; }';
          if (id.startsWith('@/pages/')) {
            return `import 'virtual:page.css';
              const Page = () => null;
              export { Page as DashboardPage, Page as ConfigurationPage, Page as CoresPage,
                Page as PanelSettingsPage, Page as SubscriptionsPage, Page as ObservabilityPage };`;
          }
        },
      }],
      build: {
        write: false,
        minify: false,
        rolldownOptions: { input: 'virtual:entry' },
      },
    });
    if (Array.isArray(result) || !('output' in result)) throw new Error('Expected one build output');
    const entry = result.output.find(output => output.type === 'chunk' && output.isEntry);
    if (!entry || entry.type !== 'chunk') throw new Error('Missing fixture entry');
    const testDocument = document.implementation.createHTMLDocument();
    const reload = vi.fn();
    const browser = {
      location: { reload },
      dispatchEvent: window.dispatchEvent.bind(window),
      routes: undefined as undefined | typeof import('@/routes/page-loaders'),
    };
    const code = entry.code.replaceAll('import.meta', 'importMeta');
    // The dynamic import cannot execute: Vite rejects on the stylesheet error first.
    runInNewContext(code, {
      window: browser,
      document: testDocument,
      Event,
      URL,
      importMeta: { url: 'https://panel.example/assets/entry.js' },
    });
    browser.routes!.preloadPage('/cores');
    const stylesheet = testDocument.querySelector('link[rel="stylesheet"]');
    expect(stylesheet).not.toBeNull();
    stylesheet!.dispatchEvent(new Event('error'));
    await new Promise(resolve => setImmediate(resolve));
    expect(reload).not.toHaveBeenCalled();
    void browser.routes!.loadPage('/panel');
    await vi.waitFor(() => expect(reload).toHaveBeenCalledOnce());
    expect(testDocument.querySelectorAll('link[rel="stylesheet"]')).toHaveLength(1);
  });
});
