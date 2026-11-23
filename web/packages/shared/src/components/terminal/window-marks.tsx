import { cn } from '@shared/lib/utils';

/** A terminal window's three marks: close, minimize and zoom. They decorate and do nothing. */
export function WindowMarks({ className }: { className?: string }) {
  return (
    <span aria-hidden='true' className={cn('flex flex-none gap-2', className)}>
      <span className='size-3 rounded-full bg-[var(--color-window-close)]' />
      <span className='size-3 rounded-full bg-[var(--color-window-minimize)]' />
      <span className='size-3 rounded-full bg-[var(--color-window-zoom)]' />
    </span>
  );
}
