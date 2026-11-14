import type { ReactNode } from 'react';

import { CSPProvider as CSPProviderPrimitive } from '@base-ui/react/csp-provider';

/**
 * Keeps Base UI from adding inline `<style>` elements, which both servers'
 * Content Security Policies refuse; `base.css` carries the rules they held.
 */
export function CSPProvider({ children }: { children: ReactNode }) {
  return <CSPProviderPrimitive disableStyleElements>{children}</CSPProviderPrimitive>;
}
