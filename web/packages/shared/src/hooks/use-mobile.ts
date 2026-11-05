import * as React from 'react';

const MOBILE_BREAKPOINT = 768;

function getIsMobile(): boolean {
  return typeof window !== 'undefined' && window.innerWidth < MOBILE_BREAKPOINT;
}

function subscribeToViewport(onStoreChange: () => void): () => void {
  if (typeof window === 'undefined') return () => {};

  if (typeof window.matchMedia !== 'function') {
    window.addEventListener('resize', onStoreChange);
    return () => window.removeEventListener('resize', onStoreChange);
  }

  const mediaQuery = window.matchMedia(`(max-width: ${MOBILE_BREAKPOINT - 1}px)`);
  mediaQuery.addEventListener('change', onStoreChange);
  return () => mediaQuery.removeEventListener('change', onStoreChange);
}

export function useIsMobile() {
  return React.useSyncExternalStore(subscribeToViewport, getIsMobile, () => false);
}
