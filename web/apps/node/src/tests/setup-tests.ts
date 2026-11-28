import '@testing-library/jest-dom/vitest';
import { afterEach, beforeAll } from 'vitest';
import { cleanup } from '@testing-library/react';

import { i18nReady } from '@/i18n';

// Components render once the language's translations have loaded, as in the application.
beforeAll(() => i18nReady);

afterEach(() => {
  cleanup();
});

if (typeof window.matchMedia !== 'function') {
  Object.defineProperty(window, 'matchMedia', {
    configurable: true,
    value: (query: string): MediaQueryList => ({
      addEventListener: () => undefined,
      addListener: () => undefined,
      dispatchEvent: () => false,
      matches: false,
      media: query,
      onchange: null,
      removeEventListener: () => undefined,
      removeListener: () => undefined,
    }),
  });
}

// jsdom implements no Web Animations; popups finish at once, as without the API.
if (typeof Element.prototype.getAnimations !== 'function') {
  Element.prototype.getAnimations = () => [];
  Object.defineProperty(globalThis, 'BASE_UI_ANIMATIONS_DISABLED', { configurable: true, value: true });
}

// jsdom draws nothing; the sign-in scene skips a canvas without a context.
Object.defineProperty(HTMLCanvasElement.prototype, 'getContext', { configurable: true, value: () => null });

// jsdom lays nothing out, so there is nothing to scroll into view.
if (typeof Element.prototype.scrollIntoView !== 'function') {
  Element.prototype.scrollIntoView = () => undefined;
}
