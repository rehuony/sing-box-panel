import { cn } from '@shared/lib/utils';
import { useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Input } from '@shared/components/ui/input';
import { Button } from '@shared/components/ui/button';
import { ChevronLeft, ChevronRight } from 'lucide-react';

import { SelectField } from '@/components/select-field';

export function ListPagination({ className, page, pages, pageSize, onPageChange, onPageSizeChange }: {
  /** Places the footer on its list's surface, such as with a hairline above it. */
  className?: string;
  page: number;
  pages: number;
  pageSize: number;
  onPageChange: (page: number) => void;
  onPageSizeChange: (size: number) => void;
}) {
  const { t } = useTranslation();
  const totalId = useId();
  const [draft, setDraft] = useState({ page, pages, value: String(page) });
  if (draft.page !== page || draft.pages !== pages) {
    setDraft({ page, pages, value: String(page) });
  }

  function commit(value: string) {
    const parsed = Number(value);
    const next = value.trim() && Number.isSafeInteger(parsed)
      ? Math.max(1, Math.min(parsed, pages))
      : page;
    setDraft({ page: next, pages, value: String(next) });
    if (next !== page) onPageChange(next);
  }

  return (
    <footer className={cn('flex flex-none items-center justify-between gap-4 pt-2', className)}>
      <SelectField
        aria-label={t('pagination.pageSize')}
        className='border-0 bg-transparent px-2 text-muted-foreground dark:bg-transparent'
        value={pageSize}
        onValueChange={onPageSizeChange}
        items={[5, 10, 50].map(value => ({ value, label: t('pagination.perPage', { count: value }) }))}
      />
      <nav className='flex flex-none items-center gap-1' aria-label={t('pagination.label')}>
        <Button
          variant='ghost'
          size='icon-sm'
          aria-label={t('pagination.previous')}
          disabled={page <= 1}
          onClick={() => onPageChange(page - 1)}
        >
          <ChevronLeft aria-hidden='true' />
        </Button>
        {/*
          A flat field, as the bar's other controls: the hover wash on hover, and while
          focused the wash over the field surface, as the strip's search. A press on the
          total puts the caret in the page.
        */}
        <div
          className='flex h-[var(--control-height-sm)] cursor-text items-center rounded-[var(--radius-control)] tabular-nums transition-colors hover:bg-[var(--color-hover)] focus-within:bg-[color-mix(in_oklab,var(--foreground)_6%,var(--color-field))]'
          onPointerDown={(event) => {
            if (!(event.target instanceof HTMLInputElement)) {
              event.preventDefault();
              event.currentTarget.querySelector('input')?.focus();
            }
          }}
        >
          <Input
            aria-label={t('pagination.currentPage')}
            aria-describedby={totalId}
            className='h-full min-h-0 w-[4ch] min-w-9 [appearance:textfield] rounded-[inherit] border-0 bg-transparent px-1 text-center font-medium shadow-none outline-none focus-visible:outline-none dark:bg-transparent [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none'
            type='number'
            inputMode='numeric'
            min={1}
            max={pages}
            step={1}
            value={draft.value}
            onChange={event => setDraft({ page, pages, value: event.target.value })}
            onBlur={event => commit(event.currentTarget.value)}
            onKeyDown={event => {
              if (event.key === 'Enter') {
                event.preventDefault();
                commit(event.currentTarget.value);
              } else if (event.key === 'Escape') {
                event.preventDefault();
                setDraft({ page, pages, value: String(page) });
              }
            }}
          />
          <span className='flex items-center gap-2 pr-3 text-base whitespace-nowrap text-muted-foreground md:text-sm' aria-hidden='true'>
            <span className='text-[var(--color-text-faint)]'>/</span>
            {pages}
          </span>
        </div>
        <span className='sr-only' id={totalId}>{t('pagination.totalPages', { count: pages })}</span>
        <Button
          variant='ghost'
          size='icon-sm'
          aria-label={t('pagination.next')}
          disabled={page >= pages}
          onClick={() => onPageChange(page + 1)}
        >
          <ChevronRight aria-hidden='true' />
        </Button>
      </nav>
    </footer>
  );
}
