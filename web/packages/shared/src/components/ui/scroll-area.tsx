import type * as React from 'react';

import { cn } from '@shared/lib/utils';
import { ScrollArea as ScrollAreaPrimitive } from '@base-ui/react/scroll-area';

/**
 * A scroll region whose scrollbars overlay the content and appear only while
 * it scrolls or the pointer is over them, so they never take layout width;
 * scrolling it to its end never scrolls the page behind it. Its content
 * reports its own size, so the scrollbars follow content that grows or
 * shrinks while the region keeps its size, as when a view replaces another.
 */
function ScrollArea({
  className,
  children,
  contentClassName,
  viewportClassName,
  viewportProps,
  viewportRef,
  ...props
}: ScrollAreaPrimitive.Root.Props & {
  /** Classes of the scrolling element itself, such as a height cap or its share of a flex column. */
  viewportClassName?: string;
  /**
   * Classes of the content inside it, such as its padding and its children's
   * layout; the content is at least as tall as the region, so a child can fill it.
   */
  contentClassName?: string;
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
        // Overscroll stays inside only along an axis that scrolls, so a region with nothing to
        // scroll hands the wheel and touch on to the page around it. As a column the region lets
        // its content grow to its own height but never below the region's, a height definite
        // enough for children that fill it or are sized in percentages.
        className={cn('flex size-full flex-col outline-none data-has-overflow-x:overscroll-x-contain data-has-overflow-y:overscroll-y-contain', viewportClassName)}
      >
        <ScrollAreaPrimitive.Content data-slot='scroll-area-content' className={cn('shrink-0 grow', contentClassName)}>
          {children}
        </ScrollAreaPrimitive.Content>
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
