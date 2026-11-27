import type { ReactNode } from 'react';

import { cn } from '@shared/lib/utils';
import { useId, useState } from 'react';
import { Info, TriangleAlert } from 'lucide-react';
import { Button } from '@shared/components/ui/button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@shared/components/ui/tooltip';

/**
 * Pointer-accessible field help that stays outside the form's Tab sequence.
 * The warning tone marks something that works differently than it reads,
 * with the warning sign in the warning colour.
 */
export function InfoTooltip({ children, className, label, tone }: {
  label: string;
  children: ReactNode;
  tone?: 'warning';
  className?: string;
}) {
  const triggerId = useId();
  const contentId = useId();
  const [open, setOpen] = useState(false);
  const Icon = tone === 'warning' ? TriangleAlert : Info;
  return (
    <Tooltip open={open} onOpenChange={setOpen} triggerId={triggerId}>
      <TooltipTrigger
        id={triggerId}
        aria-label={label}
        aria-describedby={open ? contentId : undefined}
        tabIndex={-1}
        closeOnClick={false}
        onClick={() => setOpen(true)}
        delay={200}
        closeDelay={100}
        render={(
          <Button
            className={cn(tone === 'warning' && 'text-[var(--color-warning-text)] hover:text-[var(--color-warning-text)]', className)}
            size='icon-xs'
            type='button'
            variant='ghost'
          />
        )}
      >
        <Icon aria-hidden />
      </TooltipTrigger>
      <TooltipContent id={contentId} role='tooltip' className='max-w-[min(20rem,calc(100vw-2rem))]'>
        <span className='max-h-[calc(var(--available-height)-1rem)] overflow-y-auto wrap-anywhere'>
          {children}
        </span>
      </TooltipContent>
    </Tooltip>
  );
}
