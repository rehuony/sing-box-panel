import { Link } from 'react-router-dom';
import { CircleCheck } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { formatRelativeTime } from '@shared/lib/format-time';
import { ScrollArea } from '@shared/components/ui/scroll-area';

import type { Attention } from '@/utils/fleet-summary';

import { nodePath } from '@/utils/node-path';

const dot = {
  danger: 'bg-[var(--color-danger)]',
  neutral: 'bg-[var(--color-text-faint)]',
  warning: 'bg-[var(--color-warning)]',
} as const;

const text = {
  danger: 'text-[var(--color-danger-text)]',
  neutral: 'text-muted-foreground',
  warning: 'text-[var(--color-warning-text)]',
} as const;

/** What in the fleet needs a look, most severe first; one line each, naming its node. */
export function AttentionList({ items }: { items: readonly Attention[] }) {
  const { i18n, t } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language ?? 'en';
  const percent = new Intl.NumberFormat(locale, { maximumFractionDigits: 0, style: 'percent' });
  if (items.length === 0) {
    return (
      <p className='flex flex-1 items-center justify-center gap-2 px-4 py-3 text-sm text-muted-foreground' role='status'>
        <CircleCheck aria-hidden='true' className='size-4 text-[var(--color-success)]' />
        {t('overview.attention.none')}
      </p>
    );
  }
  return (
    <ScrollArea className='min-h-0 flex-1'>
      <ul className='divide-y'>
        {items.map(({ item, kind, percent: share, tone }) => (
          <li className='flex min-w-0 items-center gap-3 px-4 py-2.5 text-sm' key={`${item.node.id}-${kind}`}>
            <span aria-hidden='true' className={`size-1.5 shrink-0 rounded-full ${dot[tone]}`} />
            <Link className='max-w-[40%] shrink-0 truncate font-medium outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring' title={item.node.name} to={nodePath(item.node.id)}>
              {item.node.name}
            </Link>
            <span className={`min-w-0 truncate ${text[tone]}`}>
              {t(`overview.attention.kind.${kind}`, {
                percent: share === undefined ? '' : percent.format(share / 100),
                time: formatRelativeTime(item.node.last_seen_at, locale),
              })}
            </span>
          </li>
        ))}
      </ul>
    </ScrollArea>
  );
}
