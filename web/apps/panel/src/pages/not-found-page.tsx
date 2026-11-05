import { SearchX } from 'lucide-react';
import { Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { buttonVariants } from '@shared/components/ui/button-variants';
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia } from '@shared/components/ui/empty';

import { ShellNavigationTrigger } from '@/components/app-shell';

export function NotFoundPage() {
  const { t } = useTranslation();

  return (
    <section className='flex min-h-0 flex-1 flex-col gap-4'>
      <div className='flex md:hidden'><ShellNavigationTrigger /></div>
      <Empty className='rounded-[var(--radius-card)] border border-solid bg-card'>
        <EmptyHeader>
          <EmptyMedia className='mb-3 size-12 rounded-[var(--radius-card)] border bg-[var(--color-surface-raised)] text-muted-foreground [&_svg]:size-6'>
            <SearchX aria-hidden='true' strokeWidth={1.5} />
          </EmptyMedia>
          <h1 className='text-sm font-medium' data-slot='empty-title'>{t('notFound.title')}</h1>
          <EmptyDescription>{t('notFound.description')}</EmptyDescription>
        </EmptyHeader>
        <EmptyContent>
          <Link className={buttonVariants()} to='/nodes'>{t('notFound.return')}</Link>
        </EmptyContent>
      </Empty>
    </section>
  );
}
