'use client';

import { Switch as SwitchPrimitive } from '@base-ui/react/switch';

import { cn } from '@/lib/utils';

function Switch({
  className,
  size = 'default',
  ...props
}: SwitchPrimitive.Root.Props & {
  size?: 'sm' | 'default';
}) {
  return (
    <SwitchPrimitive.Root
      data-slot='switch'
      data-size={size}
      className={cn(
        'peer group/switch relative inline-flex h-11 shrink-0 items-center rounded-full border border-transparent bg-transparent outline-none transition-[border-color,box-shadow] data-[size=default]:w-14 data-[size=sm]:w-11 aria-invalid:border-destructive aria-invalid:ring-3 aria-invalid:ring-destructive/20 dark:aria-invalid:border-destructive/50 dark:aria-invalid:ring-destructive/40 data-disabled:cursor-not-allowed data-disabled:opacity-50',
        className,
      )}
      {...props}
    >
      <span
        aria-hidden='true'
        className='absolute top-1/2 -translate-y-1/2 rounded-full transition-colors group-data-[size=default]/switch:left-3 group-data-[size=default]/switch:h-[18.4px] group-data-[size=default]/switch:w-8 group-data-[size=sm]/switch:left-2.5 group-data-[size=sm]/switch:h-[14px] group-data-[size=sm]/switch:w-6 group-data-checked/switch:bg-primary group-data-unchecked/switch:bg-input dark:group-data-unchecked/switch:bg-input/80'
      />
      <SwitchPrimitive.Thumb
        data-slot='switch-thumb'
        className='pointer-events-none absolute top-1/2 z-10 -translate-y-1/2 rounded-full bg-background ring-0 transition-transform group-data-[size=default]/switch:left-[13px] group-data-[size=default]/switch:size-4 group-data-[size=sm]/switch:left-[11px] group-data-[size=sm]/switch:size-3 group-data-[size=default]/switch:data-checked:translate-x-[14px] group-data-[size=sm]/switch:data-checked:translate-x-[10px] dark:data-checked:bg-primary-foreground dark:data-unchecked:bg-foreground'
      />
    </SwitchPrimitive.Root>
  );
}

export { Switch };
