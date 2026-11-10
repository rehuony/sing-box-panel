import { Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { CellMeter } from '@shared/components/cell-meter';
import { ScrollArea } from '@shared/components/ui/scroll-area';
import { formatBytes, formatRate } from '@shared/lib/telemetry-format';

import type { NodeStatus } from '@/utils/fleet-summary';

import { nodePath } from '@/utils/node-path';
import { UpDown } from '@/components/up-down';
import { hostLoad, loadTone, nodeRates, percentOf, periodUsage } from '@/utils/fleet-summary';

const stateDot = {
  offline: 'bg-[var(--color-danger)]',
  online: 'bg-[var(--color-success)]',
  pending: 'bg-[var(--color-text-faint)]',
} as const;

function Share({ percent, tone }: { percent: number | null; tone?: 'warning' | 'danger' }) {
  const { i18n } = useTranslation();
  if (percent === null) return <span className='text-muted-foreground'>—</span>;
  const label = new Intl.NumberFormat(i18n.resolvedLanguage ?? 'en', { maximumFractionDigits: 0, style: 'percent' }).format(percent / 100);
  return (
    <span className='flex items-center gap-2'>
      <CellMeter className='@max-5xl:hidden' percent={percent} tone={tone} />
      <span className='w-[4ch] text-end'>{label}</span>
    </span>
  );
}

function Row({ item }: { item: NodeStatus }) {
  const { i18n, t } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language ?? 'en';
  const { metrics, node } = item;
  const { cpu, memory } = hostLoad(item);
  const rates = nodeRates(item);
  const used = periodUsage(metrics);
  const quota = percentOf(used, metrics.quota_bytes);
  const version = node.runtime?.enabled_core?.exact_core_version;
  const running = node.state === 'online' && node.runtime?.observation_state === 'running';
  return (
    <tr className='border-b last:border-b-0 hover:bg-[var(--color-surface-raised)]'>
      <td className='py-2.5 ps-4 pe-3'>
        <div className='flex min-w-0 items-center gap-2'>
          <span aria-hidden='true' className={`size-1.5 shrink-0 rounded-full ${stateDot[node.state]}`} />
          <div className='min-w-0'>
            <Link className='block truncate font-[family-name:var(--font-body)] font-medium outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring' title={node.name} to={nodePath(node.id)}>
              {node.name}
            </Link>
            <span className='block truncate text-xs text-muted-foreground'>
              {t(`nodes.state.${node.state}`)}
              {node.public_ip ? ` · ${node.public_ip}` : ''}
            </span>
          </div>
        </div>
      </td>
      <td className='px-3'>
        <span className={running ? '' : 'text-muted-foreground'}>{version ? `v${version.replace(/^v/i, '')}` : '—'}</span>
      </td>
      <td className='px-3 @max-2xl:hidden'><Share percent={cpu} tone={loadTone(cpu)} /></td>
      <td className='px-3 @max-2xl:hidden'><Share percent={memory} tone={loadTone(memory)} /></td>
      <td className='px-3 @max-xl:hidden'>
        <UpDown download={formatRate(rates.download, locale)} upload={formatRate(rates.upload, locale)} />
      </td>
      <td className='py-2.5 ps-3 pe-4'>
        <span className='flex items-center justify-end gap-2'>
          {quota === null ? null : <CellMeter className='@max-5xl:hidden' percent={quota} tone={metrics.quota_exceeded ? 'danger' : quota >= 90 ? 'warning' : undefined} />}
          <span className='whitespace-nowrap'>
            {formatBytes(used, locale)}
            <span className='text-muted-foreground'>{` / ${metrics.quota_bytes ? formatBytes(metrics.quota_bytes, locale) : '∞'}`}</span>
          </span>
        </span>
      </td>
    </tr>
  );
}

/** Every node's state, core, load, traffic and period usage, in rows that scroll inside the region. */
export function NodeLoadTable({ items }: { items: readonly NodeStatus[] }) {
  const { t } = useTranslation();
  const header = 'sticky top-0 z-1 h-9 bg-card px-3 text-start text-xs font-medium text-muted-foreground';
  return (
    <ScrollArea className='min-h-0 flex-1'>
      <table className='w-full border-collapse font-[family-name:var(--font-data)] text-compact tabular-nums'>
        <thead>
          <tr className='border-b'>
            <th className={`${header} ps-4 font-[family-name:var(--font-body)]`} scope='col'>{t('overview.nodes.column.node')}</th>
            <th className={`${header} font-[family-name:var(--font-body)]`} scope='col'>{t('overview.nodes.column.core')}</th>
            <th className={`${header} font-[family-name:var(--font-body)] @max-2xl:hidden`} scope='col'>{t('overview.nodes.column.cpu')}</th>
            <th className={`${header} font-[family-name:var(--font-body)] @max-2xl:hidden`} scope='col'>{t('overview.nodes.column.memory')}</th>
            <th className={`${header} font-[family-name:var(--font-body)] @max-xl:hidden`} scope='col'>{t('overview.nodes.column.traffic')}</th>
            <th className={`${header} pe-4 text-end font-[family-name:var(--font-body)]`} scope='col'>{t('overview.nodes.column.usage')}</th>
          </tr>
        </thead>
        <tbody>
          {items.map(item => <Row item={item} key={item.node.id} />)}
        </tbody>
      </table>
    </ScrollArea>
  );
}
