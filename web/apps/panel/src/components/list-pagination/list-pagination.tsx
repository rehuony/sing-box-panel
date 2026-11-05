import { useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Input } from '@shared/components/ui/input';
import { Button } from '@shared/components/ui/button';
import { ChevronLeft, ChevronRight } from 'lucide-react';

import { SelectField } from '@/components/select-field';

export function ListPagination({ page, pages, pageSize, disabled = false, onPageChange, onPageSizeChange }: {
  page: number;
  pages: number;
  pageSize: number;
  disabled?: boolean;
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
    <footer className='flex flex-none items-center justify-between gap-4 pt-2'>
      <SelectField
        aria-label={t('pagination.pageSize')}
        className='border-0 bg-transparent px-2 text-muted-foreground dark:bg-transparent'
        disabled={disabled}
        value={pageSize}
        onValueChange={onPageSizeChange}
        items={[5, 10, 50].map(value => ({ value, label: t('pagination.perPage', { count: value }) }))}
      />
      <nav className='flex flex-none items-center gap-1 *:data-[slot=button]:rounded-full' aria-label={t('pagination.label')}>
        <Button
          variant='ghost'
          size='icon-sm'
          aria-label={t('pagination.previous')}
          disabled={disabled || page <= 1}
          onClick={() => onPageChange(page - 1)}
        >
          <ChevronLeft aria-hidden='true' />
        </Button>
        <div className='flex h-[var(--control-height-sm)] items-center rounded-[var(--radius-control)] border border-transparent bg-[color-mix(in_oklch,var(--foreground)_4%,var(--color-surface))] tabular-nums focus-within:border-[var(--input-focus)] data-disabled:opacity-50' data-disabled={disabled || undefined}>
          <Input
            aria-label={t('pagination.currentPage')}
            aria-describedby={totalId}
            className='h-full min-h-0 w-[4ch] min-w-9 [appearance:textfield] rounded-[inherit] border-0 bg-transparent px-1 text-center text-sm font-medium shadow-none outline-none focus-visible:outline-none dark:bg-transparent [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none'
            type='number'
            inputMode='numeric'
            min={1}
            max={pages}
            step={1}
            disabled={disabled}
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
          <span className='flex items-center gap-2 pr-3 text-sm whitespace-nowrap text-muted-foreground' aria-hidden='true'>
            <span className='opacity-55'>/</span>
            {pages}
          </span>
        </div>
        <span className='sr-only' id={totalId}>{t('pagination.totalPages', { count: pages })}</span>
        <Button
          variant='ghost'
          size='icon-sm'
          aria-label={t('pagination.next')}
          disabled={disabled || page >= pages}
          onClick={() => onPageChange(page + 1)}
        >
          <ChevronRight aria-hidden='true' />
        </Button>
      </nav>
    </footer>
  );
}
