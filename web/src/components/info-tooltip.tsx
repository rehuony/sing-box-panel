import type { ReactNode } from 'react';

import { Info } from 'lucide-react';
import { useId, useState } from 'react';

import { Button } from '@/components/ui/button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';

/** Uses the shared tooltip's hover/focus behavior; clicking also opens it immediately. */
export function InfoTooltip({ label, children }: { label: string; children: ReactNode }) {
  const triggerId = useId();
  const contentId = useId();
  const [open, setOpen] = useState(false);
  return (
    <Tooltip open={open} onOpenChange={setOpen} triggerId={triggerId}>
      <TooltipTrigger
        id={triggerId}
        aria-label={label}
        aria-describedby={open ? contentId : undefined}
        closeOnClick={false}
        onClick={() => setOpen(true)}
        delay={200}
        closeDelay={100}
        render={<Button size='icon-xs' type='button' variant='ghost' />}
      >
        <Info aria-hidden />
      </TooltipTrigger>
      <TooltipContent id={contentId} role='tooltip' className='max-w-[min(20rem,calc(100vw-2rem))]'>
        <span className='max-h-[calc(var(--available-height)-1rem)] overflow-y-auto wrap-anywhere'>
          {children}
        </span>
      </TooltipContent>
    </Tooltip>
  );
}
