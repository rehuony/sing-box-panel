import * as React from 'react';
import { XIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { AlertDialog as AlertDialogPrimitive } from '@base-ui/react/alert-dialog';

import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';

function AlertDialog({ ...props }: AlertDialogPrimitive.Root.Props) {
  return <AlertDialogPrimitive.Root data-slot='alert-dialog' {...props} />;
}

function AlertDialogTrigger({ ...props }: AlertDialogPrimitive.Trigger.Props) {
  return (
    <AlertDialogPrimitive.Trigger data-slot='alert-dialog-trigger' {...props} />
  );
}

function AlertDialogPortal({ ...props }: AlertDialogPrimitive.Portal.Props) {
  return (
    <AlertDialogPrimitive.Portal data-slot='alert-dialog-portal' {...props} />
  );
}

function AlertDialogOverlay({
  className,
  ...props
}: AlertDialogPrimitive.Backdrop.Props) {
  return (
    <AlertDialogPrimitive.Backdrop
      data-slot='alert-dialog-overlay'
      className={cn(
        'fixed inset-0 isolate z-[var(--z-modal)] bg-[var(--color-scrim)] duration-100 supports-backdrop-filter:backdrop-blur-xs data-open:animate-in data-open:fade-in-0 data-closed:animate-out data-closed:fade-out-0',
        className,
      )}
      {...props}
    />
  );
}

function AlertDialogContent({
  className,
  children,
  size = 'default',
  showCloseButton = true,
  ...props
}: AlertDialogPrimitive.Popup.Props & {
  size?: 'default' | 'sm';
  showCloseButton?: boolean;
}) {
  const { t } = useTranslation();

  return (
    <AlertDialogPortal>
      <AlertDialogOverlay />
      <AlertDialogPrimitive.Popup
        data-slot='alert-dialog-content'
        data-size={size}
        className={cn(
          'fixed top-1/2 left-1/2 z-[calc(var(--z-modal)+1)] grid max-h-[calc(100dvh-2rem)] w-[calc(100%-2rem)] -translate-x-1/2 -translate-y-1/2 gap-4 overflow-y-auto rounded-[var(--radius-window)] bg-popover p-6 text-popover-foreground shadow-[var(--shadow-panel)] ring-1 ring-border/60 duration-100 outline-none data-[size=default]:max-w-[30rem] data-[size=sm]:max-w-xs data-open:animate-in data-open:fade-in-0 data-open:zoom-in-95 data-closed:animate-out data-closed:fade-out-0 data-closed:zoom-out-95',
          className,
        )}
        {...props}
      >
        {children}
        {showCloseButton && (
          <AlertDialogPrimitive.Close
            data-slot='alert-dialog-close'
            render={(
              <Button
                variant='ghost'
                className='absolute top-3 right-3'
                size='icon-sm'
              />
            )}
          >
            <XIcon />
            <span className='sr-only'>{t('common.close')}</span>
          </AlertDialogPrimitive.Close>
        )}
      </AlertDialogPrimitive.Popup>
    </AlertDialogPortal>
  );
}

function AlertDialogHeader({
  className,
  ...props
}: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot='alert-dialog-header'
      className={cn(
        'flex flex-col gap-4 text-left',
        className,
      )}
      {...props}
    />
  );
}

function AlertDialogFooter({
  className,
  ...props
}: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot='alert-dialog-footer'
      className={cn(
        'flex flex-wrap items-center justify-end gap-3 [&>button]:min-w-20',
        className,
      )}
      {...props}
    />
  );
}

function AlertDialogTitle({
  className,
  ...props
}: React.ComponentProps<typeof AlertDialogPrimitive.Title>) {
  return (
    <AlertDialogPrimitive.Title
      data-slot='alert-dialog-title'
      className={cn(
        'pr-9 font-heading text-base font-medium',
        className,
      )}
      {...props}
    />
  );
}

function AlertDialogDescription({
  className,
  ...props
}: React.ComponentProps<typeof AlertDialogPrimitive.Description>) {
  return (
    <AlertDialogPrimitive.Description
      data-slot='alert-dialog-description'
      className={cn(
        'text-sm text-balance text-muted-foreground md:text-pretty *:[a]:underline *:[a]:underline-offset-3 *:[a]:hover:text-foreground',
        className,
      )}
      {...props}
    />
  );
}

function AlertDialogAction({
  className,
  ...props
}: React.ComponentProps<typeof Button>) {
  return (
    <Button
      data-slot='alert-dialog-action'
      className={cn(className)}
      {...props}
    />
  );
}

function AlertDialogCancel({
  className,
  variant = 'secondary',
  size = 'default',
  ...props
}: AlertDialogPrimitive.Close.Props
  & Pick<React.ComponentProps<typeof Button>, 'variant' | 'size'>) {
  return (
    <AlertDialogPrimitive.Close
      data-slot='alert-dialog-cancel'
      className={cn(className)}
      render={<Button variant={variant} size={size} />}
      {...props}
    />
  );
}

export {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogOverlay,
  AlertDialogPortal,
  AlertDialogTitle,
  AlertDialogTrigger,
};
