import type * as React from 'react';

import { cn } from '@shared/lib/utils';
import { ScrollArea as ScrollAreaPrimitive } from '@base-ui/react/scroll-area';

/**
 * A scroll region whose scrollbars overlay the content and appear only while
 * it scrolls or the pointer is over them, so they never take layout width.
 */
function ScrollArea({
  className,
  children,
  viewportClassName,
  viewportProps,
  viewportRef,
  ...props
}: ScrollAreaPrimitive.Root.Props & {
  viewportClassName?: string;
  /** Props of the scrolling element, such as its role, label and scroll handler. */
  viewportProps?: Omit<ScrollAreaPrimitive.Viewport.Props, 'className' | 'ref'>;
  viewportRef?: React.Ref<HTMLDivElement>;
}) {
  return (
    <ScrollAreaPrimitive.Root
      data-slot='scroll-area'
      className={cn('relative min-h-0 overflow-hidden', className)}
      {...props}
    >
      <ScrollAreaPrimitive.Viewport
        data-slot='scroll-area-viewport'
        {...viewportProps}
        ref={viewportRef}
        className={cn('size-full overscroll-contain outline-none', viewportClassName)}
      >
        {children}
      </ScrollAreaPrimitive.Viewport>
      <ScrollBar orientation='vertical' />
      <ScrollBar orientation='horizontal' />
      <ScrollAreaPrimitive.Corner />
    </ScrollAreaPrimitive.Root>
  );
}

function ScrollBar({ className, orientation = 'vertical', ...props }: ScrollAreaPrimitive.Scrollbar.Props) {
  return (
    <ScrollAreaPrimitive.Scrollbar
      data-slot='scroll-area-scrollbar'
      orientation={orientation}
      className={cn(
        'z-[var(--z-sticky)] flex touch-none p-0.5 opacity-0 transition-opacity duration-[var(--dur-short)] ease-out select-none data-[hovering]:opacity-100 data-[scrolling]:opacity-100 motion-reduce:transition-none data-[orientation=horizontal]:h-2.5 data-[orientation=horizontal]:flex-col data-[orientation=vertical]:w-2.5',
        className,
      )}
      {...props}
    >
      <ScrollAreaPrimitive.Thumb
        data-slot='scroll-area-thumb'
        className='relative flex-1 rounded-[var(--radius-pill)] bg-[var(--color-selection)]'
      />
    </ScrollAreaPrimitive.Scrollbar>
  );
}

export { ScrollArea, ScrollBar };
