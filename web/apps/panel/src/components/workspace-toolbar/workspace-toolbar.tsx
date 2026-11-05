import type { ReactNode } from 'react';

import { createPortal } from 'react-dom';

import './workspace-toolbar.css';

export function WorkspaceToolbar({ children }: { children: ReactNode }) {
  return <div className='workspace-toolbar'>{children}</div>;
}

export function ToolbarActions({ children, target, active = true }: {
  children: ReactNode;
  target?: HTMLElement | null;
  active?: boolean;
}) {
  if (!active) return null;
  return target ? createPortal(children, target) : children;
}
