import type { SubscriptionFormat } from '@/api/api-client';

/** A channel's client as its configuration names it, in a neutral pill. */
export function ChannelClientTag({ format }: { format: SubscriptionFormat }) {
  return (
    <span className='inline-flex h-5 w-fit flex-none items-center rounded-[var(--radius-pill)] border px-2 font-[family-name:var(--font-data)] text-xs whitespace-nowrap text-[var(--color-text-secondary)]'>
      {format}
    </span>
  );
}
