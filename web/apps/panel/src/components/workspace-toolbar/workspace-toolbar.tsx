import type { ReactNode, Ref } from 'react';

import { cn } from '@shared/lib/utils';
import { createPortal } from 'react-dom';

/** A view's toolbar row: tabs or filters at the start, actions at the end; actions wrap below when narrow. */
export function WorkspaceToolbar({ children }: { children: ReactNode }) {
  return (
    <div data-slot='workspace-toolbar' className='flex min-w-0 flex-none flex-wrap items-center gap-x-3 gap-y-3 *:data-[slot=tabs-list]:max-w-full *:data-[slot=tabs-list]:flex-none *:data-[slot=tabs-list]:overflow-x-auto *:data-[slot=tabs-list]:[scrollbar-width:none]'>
      {children}
    </div>
  );
}

/**
 * The end of a toolbar row, where the active tab renders its actions: a
 * container whose bar actions keep their icons alone below 24rem, so the row
 * keeps one line beside the tabs down to a phone.
 */
export function WorkspaceToolbarActions({ children, className, ref }: {
  children?: ReactNode;
  className?: string;
  ref?: Ref<HTMLDivElement>;
}) {
  return (
    <div
      className={cn('@container/strip-actions flex min-w-[min(100%,16rem)] flex-1 items-center justify-end gap-2', className)}
      ref={ref}
    >
      {children}
    </div>
  );
}

export function ToolbarActions({ children, target, active = true }: {
  children: ReactNode;
  target?: HTMLElement | null;
  active?: boolean;
}) {
  if (!active) return null;
  return target ? createPortal(children, target) : children;
}
