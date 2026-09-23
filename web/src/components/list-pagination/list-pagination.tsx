import { useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ChevronLeft, ChevronRight } from 'lucide-react';

import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { SelectField } from '@/components/select-field';

import './list-pagination.css';

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
    <footer className='list-pagination'>
      <SelectField
        aria-label={t('pagination.pageSize')}
        value={pageSize}
        onValueChange={onPageSizeChange}
        items={[5, 10, 50].map(value => ({ value, label: t('pagination.perPage', { count: value }) }))}
      />
      <nav className='list-pagination__navigation' aria-label={t('pagination.label')}>
        <Button
          variant='ghost'
          size='icon-sm'
          aria-label={t('pagination.previous')}
          disabled={disabled || page <= 1}
          onClick={() => onPageChange(page - 1)}
        >
          <ChevronLeft aria-hidden='true' />
        </Button>
        <div className='list-pagination__field' data-disabled={disabled || undefined}>
          <Input
            aria-label={t('pagination.currentPage')}
            aria-describedby={totalId}
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
          <span className='list-pagination__total' aria-hidden='true'>
            <span>/</span>
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
