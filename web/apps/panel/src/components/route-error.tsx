import type { ReactNode } from 'react';

import { Component } from 'react';
import { cn } from '@shared/lib/utils';
import { TriangleAlert } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@shared/components/ui/button';
import { Empty, EmptyContent, EmptyHeader, EmptyMedia, EmptyTitle } from '@shared/components/ui/empty';

/**
 * What failed, in one line, and reloading the document as the next step: a
 * route module that failed to load stays failed until the document loads it
 * again.
 */
function ErrorState({ className, title }: { className?: string; title: string }) {
  const { t } = useTranslation();
  return (
    <Empty className={cn('rounded-[var(--radius-card)] border border-solid bg-[var(--color-surface-translucent)]', className)} role='alert'>
      <EmptyHeader>
        <EmptyMedia className='mb-3 size-12 rounded-[var(--radius-card)] border bg-[var(--color-surface-raised)] text-muted-foreground [&_svg]:size-6'>
          <TriangleAlert aria-hidden='true' strokeWidth={1.5} />
        </EmptyMedia>
        <EmptyTitle>{title}</EmptyTitle>
      </EmptyHeader>
      <EmptyContent>
        <Button onClick={() => window.location.reload()}>{t('shell.error.reload')}</Button>
      </EmptyContent>
    </Empty>
  );
}

function ViewError() {
  const { t } = useTranslation();
  return (
    <div className='flex min-h-0 flex-1 flex-col'>
      <ErrorState title={t('shell.error.view')} />
    </div>
  );
}

interface ViewErrorBoundaryProps {
  /** Another value leaves the error, as opening another window does. */
  resetKey: string;
  children: ReactNode;
}

/**
 * Shows a view that failed, such as one whose module did not load, in its
 * place inside the shell, so the status line stays to leave it.
 */
export class ViewErrorBoundary extends Component<ViewErrorBoundaryProps, { failed: boolean; resetKey: string }> {
  state = { failed: false, resetKey: this.props.resetKey };

  static getDerivedStateFromError() {
    return { failed: true };
  }

  static getDerivedStateFromProps(props: ViewErrorBoundaryProps, state: { failed: boolean; resetKey: string }) {
    return props.resetKey === state.resetKey ? null : { failed: false, resetKey: props.resetKey };
  }

  render() {
    return this.state.failed ? <ViewError /> : this.props.children;
  }
}

/** The page's own fallback for a failure outside the shell, centred in the window. */
export function RootRouteError() {
  const { t } = useTranslation();
  return (
    <main className='flex min-h-svh min-w-0 flex-col items-center justify-center p-4 sm:p-6'>
      <ErrorState className='max-w-sm flex-none p-8 shadow-[var(--shadow-card)]' title={t('shell.error.page')} />
    </main>
  );
}
