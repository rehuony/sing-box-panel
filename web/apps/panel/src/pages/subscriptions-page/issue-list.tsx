import { cn } from '@shared/lib/utils';
import { useId, useState } from 'react';
import { Link } from 'react-router-dom';
import { ArrowUpRight, ChevronRight } from 'lucide-react';

/** One place an issue is about. */
export interface IssueItem {
  key: string;
  /** Where the issue is fixed. */
  to?: string;
  /** The node it is about. */
  name: string;
  /** Where in the node, such as an inbound tag or a field. */
  detail?: string;
}

/** The issues of one cause. */
export interface IssueGroup {
  key: string;
  title: string;
  items: IssueItem[];
}

/** A heading's band: the hover wash made opaque over the popover, so rows scrolling under it never show through. */
const band = 'bg-[color-mix(in_oklab,var(--foreground)_5%,var(--popover))] hover:bg-[color-mix(in_oklab,var(--foreground)_9%,var(--popover))]';
const row = 'grid h-7.5 min-w-0 grid-cols-[minmax(0,1fr)_auto_1rem] items-center gap-2 rounded-[var(--radius-control)] px-2 text-compact';

function IssueRow({ item, showDetail }: { item: IssueItem; showDetail: boolean }) {
  const body = (
    <>
      <span className='min-w-0 truncate'>{item.name}</span>
      <span className='font-[family-name:var(--font-data)] text-xs text-[var(--color-text-faint)]'>{showDetail ? item.detail : null}</span>
    </>
  );
  return (
    <li>
      {item.to
        ? (
            <Link
              className={cn(row, 'group/issue outline-none transition-colors duration-[var(--dur-micro)] ease-out hover:bg-[var(--color-hover)] focus-visible:bg-[var(--color-hover)]')}
              to={item.to}
            >
              {body}
              <ArrowUpRight aria-hidden='true' className='invisible size-3.5 text-muted-foreground group-hover/issue:visible group-focus-visible/issue:visible' />
            </Link>
          )
        : <div className={row}>{body}</div>}
    </li>
  );
}

/**
 * Issues grouped by cause, one group open at a time, for a popover that
 * scrolls without padding. A group's heading holds its cause, with the
 * detail its items share, and their count, on an opaque band that stays
 * pinned while the open group's items scroll under it; the items hang from
 * a guide under the chevron, each the node it is about, opening where the
 * issue is fixed, with its own detail only where the group's differ.
 */
export function IssueList({ groups }: { groups: IssueGroup[] }) {
  const id = useId();
  const [open, setOpen] = useState<string | null>(groups[0]?.key ?? null);
  return (
    <div className='flex flex-col'>
      {groups.map((group, index) => {
        const expanded = open === group.key;
        const first = group.items[0]?.detail;
        const shared = group.items.every(item => item.detail === first) ? first : undefined;
        const panel = `${id}-${index}`;
        return (
          <section aria-label={group.title} key={group.key}>
            <button
              aria-controls={panel}
              aria-expanded={expanded}
              className={cn(
                'sticky top-0 z-1 grid h-10 w-full cursor-pointer grid-cols-[1rem_minmax(0,1fr)_auto] items-center gap-2 border-b ps-3 pe-3.5 text-left outline-none transition-colors duration-[var(--dur-micro)] ease-out focus-visible:bg-[var(--color-hover)]',
                band,
                // An open group's last row meets the next heading, which then draws the hairline between them.
                index > 0 && open === groups[index - 1]!.key && 'border-t',
              )}
              onClick={() => setOpen(expanded ? null : group.key)}
              type='button'
            >
              <ChevronRight aria-hidden='true' className={cn('size-3.5 text-muted-foreground transition-transform duration-[var(--dur-short)] ease-out', expanded && 'rotate-90')} />
              <span className='min-w-0 truncate font-medium'>
                {group.title}
                {shared ? <code className='ms-1.5 font-[family-name:var(--font-data)] text-xs font-normal text-[var(--color-text-secondary)]'>{shared}</code> : null}
              </span>
              <span className='font-[family-name:var(--font-data)] text-xs text-[var(--color-text-faint)] tabular-nums'>{group.items.length}</span>
            </button>
            {expanded
              ? (
                  <ul className='relative ps-7 pe-1.5 pt-1 pb-1.5 before:absolute before:start-[1.1875rem] before:top-0 before:bottom-2 before:w-px before:bg-border' id={panel}>
                    {group.items.map(item => <IssueRow item={item} key={item.key} showDetail={shared === undefined} />)}
                  </ul>
                )
              : null}
          </section>
        );
      })}
    </div>
  );
}
