'use client';

import * as React from 'react';
import { XIcon } from 'lucide-react';
import { cn } from '@shared/lib/utils';
import { useTranslation } from 'react-i18next';
import { Button } from '@shared/components/ui/button';
import { Dialog as DialogPrimitive } from '@base-ui/react/dialog';

function Dialog({ ...props }: DialogPrimitive.Root.Props) {
  return <DialogPrimitive.Root data-slot='dialog' {...props} />;
}

function DialogTrigger({ ...props }: DialogPrimitive.Trigger.Props) {
  return <DialogPrimitive.Trigger data-slot='dialog-trigger' {...props} />;
}

function DialogPortal({ ...props }: DialogPrimitive.Portal.Props) {
  return <DialogPrimitive.Portal data-slot='dialog-portal' {...props} />;
}

function DialogClose({ ...props }: DialogPrimitive.Close.Props) {
  return <DialogPrimitive.Close data-slot='dialog-close' {...props} />;
}

function DialogOverlay({
  className,
  ...props
}: DialogPrimitive.Backdrop.Props) {
  return (
    <DialogPrimitive.Backdrop
      data-slot='dialog-overlay'
      className={cn(
        'fixed inset-0 isolate z-[var(--z-modal)] bg-[var(--color-scrim)] opacity-100 transition-opacity duration-150 ease-out data-starting-style:opacity-0 data-ending-style:opacity-0',
        className,
      )}
      {...props}
    />
  );
}

/** Widths: forms and confirmations, a form with a preview or code beside its fields, and a workspace. */
const dialogSizes = {
  default: 'sm:max-w-[30rem]',
  wide: 'sm:max-w-[40rem]',
  workspace: 'sm:max-w-[60rem]',
} as const;

function DialogContent({
  className,
  children,
  showCloseButton = true,
  size = 'default',
  ...props
}: DialogPrimitive.Popup.Props & {
  showCloseButton?: boolean;
  size?: keyof typeof dialogSizes;
}) {
  const { t } = useTranslation();

  return (
    <DialogPortal>
      <DialogOverlay />
      <DialogPrimitive.Popup
        data-size={size}
        data-slot='dialog-content'
        className={cn(
          'fixed top-1/2 left-1/2 z-[calc(var(--z-modal)+1)] grid w-full max-h-[calc(100dvh-2rem)] overflow-y-auto max-w-[calc(100%-2rem)] -translate-x-1/2 -translate-y-1/2 gap-5 rounded-[var(--radius-window)] bg-popover p-6 text-sm text-popover-foreground shadow-[var(--shadow-panel)] ring-1 ring-border/60 opacity-100 transition-[opacity,scale] duration-150 ease-out outline-none data-starting-style:opacity-0 data-starting-style:scale-98 data-ending-style:opacity-0 data-ending-style:scale-98',
          dialogSizes[size],
          className,
        )}
        {...props}
      >
        {children}
        {showCloseButton && (
          <DialogPrimitive.Close
            data-slot='dialog-close'
            render={(
              <Button
                variant='ghost'
                className='absolute top-3 right-3'
                size='icon-sm'
              />
            )}
          >
            <XIcon
            />
            <span className='sr-only'>{t('common.close')}</span>
          </DialogPrimitive.Close>
        )}
      </DialogPrimitive.Popup>
    </DialogPortal>
  );
}

function DialogHeader({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot='dialog-header'
      className={cn('flex flex-col gap-1.5 text-left', className)}
      {...props}
    />
  );
}

/**
 * A form's rows: a 7rem label column and the control filling the rest,
 * each label stacked over its control on a phone.
 */
function DialogFields({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot='dialog-fields'
      className={cn(
        'grid grid-cols-[7rem_minmax(0,1fr)] items-center gap-x-4 gap-y-3 [&_[data-slot=select-trigger]]:w-full max-[520px]:grid-cols-1 max-[520px]:gap-y-1.5 max-[520px]:[&>:nth-child(odd):not(:first-child)]:mt-1.5',
        className,
      )}
      {...props}
    />
  );
}

function DialogFooter({
  className,
  showCloseButton = false,
  message,
  children,
  ...props
}: React.ComponentProps<'div'> & {
  showCloseButton?: boolean;
  /**
   * A note, such as progress or what a choice changes, in one line kept in
   * place so nothing moves when it appears; problems go to a toast.
   */
  message?: React.ReactNode;
}) {
  const { t } = useTranslation();

  return (
    <div
      data-slot='dialog-footer'
      className={cn(
        'mt-1 flex flex-wrap items-center justify-end gap-2 [&>button]:min-w-20',
        className,
      )}
      {...props}
    >
      {message === undefined
        ? null
        : <p className='me-auto min-w-0 flex-1 truncate text-compact text-muted-foreground' role='status' title={typeof message === 'string' && message ? message : undefined}>{message}</p>}
      {showCloseButton && (
        <DialogPrimitive.Close render={<Button variant='outline' />}>
          {t('common.cancel')}
        </DialogPrimitive.Close>
      )}
      {children}
    </div>
  );
}

function DialogTitle({ className, ...props }: DialogPrimitive.Title.Props) {
  return (
    <DialogPrimitive.Title
      data-slot='dialog-title'
      className={cn(
        'pr-9 font-heading text-base font-medium',
        className,
      )}
      {...props}
    />
  );
}

function DialogDescription({
  className,
  ...props
}: DialogPrimitive.Description.Props) {
  return (
    <DialogPrimitive.Description
      data-slot='dialog-description'
      className={cn(
        'text-sm text-muted-foreground *:[a]:underline *:[a]:underline-offset-3 *:[a]:hover:text-foreground',
        className,
      )}
      {...props}
    />
  );
}

export {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFields,
  DialogFooter,
  DialogHeader,
  DialogOverlay,
  DialogPortal,
  DialogTitle,
  DialogTrigger,
};
