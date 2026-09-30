import '@testing-library/jest-dom/vitest';
import { afterEach, vi } from 'vitest';
import { webcrypto } from 'node:crypto';
import { cleanup } from '@testing-library/react';

// jsdom supplies Crypto but not the browser's SubtleCrypto implementation.
if (!globalThis.crypto.subtle) {
  Object.defineProperty(globalThis.crypto, 'subtle', { value: webcrypto.subtle });
}

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

afterEach(() => {
  cleanup();
  if (vi.isFakeTimers()) {
    vi.clearAllTimers();
    vi.useRealTimers();
  }
});

// Layout observers are supplied by the browser, but not by jsdom.
if (typeof ResizeObserver === 'undefined') {
  Object.defineProperty(globalThis, 'ResizeObserver', {
    configurable: true,
    writable: true,
    value: class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  });
}

// CodeMirror measures text ranges; jsdom has no text layout engine.
if (typeof Range.prototype.getClientRects !== 'function') {
  Range.prototype.getClientRects = () => [] as unknown as DOMRectList;
  Range.prototype.getBoundingClientRect = () => new DOMRect();
}
