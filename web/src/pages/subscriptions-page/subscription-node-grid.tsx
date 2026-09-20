import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ChevronLeft, ChevronRight, Eye, EyeOff, MoreHorizontal } from 'lucide-react';

import type { SubscriptionNodeSummary } from '@/api/api-client';

import { cn } from '@/lib/utils';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { SelectField } from '@/components/select-field';

import { subscriptionNodeAddress } from './subscription-node-address';

interface NodeGridProps {
  busy?: boolean;
  search: string;
  readOnly?: boolean;
  selected?: Set<string>;
  onSelect?: (id: string) => void;
  nodes: SubscriptionNodeSummary[];
  onOpen: (node: SubscriptionNodeSummary) => void;
  onVisibility?: (node: SubscriptionNodeSummary) => void;
}

export function SubscriptionNodeGrid({
  nodes,
  search,
  busy,
  readOnly,
  selected,
  onSelect,
  onOpen,
  onVisibility,
}: NodeGridProps) {
  const { t } = useTranslation();
  const [size, setSize] = useState(10);
  const [pagination, setPagination] = useState({ search, size, page: 1 });
  const page = pagination.search === search && pagination.size === size ? pagination.page : 1;
  const setPage = (value: number) => setPagination({ search, size, page: value });
  const filtered = nodes.filter(
    (node) =>
      (!selected || (!node.hidden && node.available))
      && `${node.name} ${node.source_name} ${node.type}`.toLowerCase().includes(search.toLowerCase()),
  );
  const pages = Math.max(1, Math.ceil(filtered.length / size));
  const current = Math.min(page, pages);
  return (
    <div className='subscription-node-list'>
      <div className='subscription-node-list__scroll'>
        {filtered.length === 0
          ? (
              <p className='subscription-empty'>{t('subscriptions.nodes.empty')}</p>
            )
          : (
              <div className='subscription-node-grid'>
                {filtered.slice((current - 1) * size, current * size).map((node) => (
                  <article
                    className={cn('subscription-node-card', node.hidden && 'is-hidden')}
                    key={node.id}
                  >
                    <header>
                      <span
                        aria-hidden='true'
                        className={`subscription-node-dot${node.available ? '' : ' is-unavailable'}`}
                      />
                      <Button variant='ghost' size='content'
                        className='subscription-node-name'
                        onClick={() => onOpen(node)}
                        title={node.name}
                        type='button'
                      >
                        <span className='truncate'>{node.name}</span>
                      </Button>
                      {onSelect
                        ? (
                            <input
                              aria-label={t('subscriptions.nodes.select', { name: node.name })}
                              checked={selected?.has(node.id) ?? false}
                              disabled={busy || readOnly}
                              onChange={() => onSelect(node.id)}
                              type='checkbox'
                            />
                          )
                        : (
                            <Button
                              aria-label={t(
                                node.hidden ? 'subscriptions.nodes.show' : 'subscriptions.nodes.hide',
                                { name: node.name },
                              )}
                              disabled={busy}
                              onClick={() => onVisibility?.(node)}
                              size='icon-xs'
                              variant='outline'
                            >
                              {node.hidden ? <EyeOff aria-hidden='true' /> : <Eye aria-hidden='true' />}
                            </Button>
                          )}
                      <Button
                        aria-label={t('subscriptions.nodes.details', { name: node.name })}
                        onClick={() => onOpen(node)}
                        size='icon-xs'
                        variant='outline'
                      >
                        <MoreHorizontal aria-hidden='true' />
                      </Button>
                    </header>
                    <Button
                      aria-hidden={node.hidden || undefined}
                      className='subscription-node-card__body'
                      disabled={node.hidden}
                      onClick={() => onOpen(node)}
                      size='content'
                      tabIndex={node.hidden ? -1 : undefined}
                      type='button'
                      variant='ghost'
                    >
                      <Badge className='subscription-node-card__badge' variant='secondary' title={node.source_name}>
                        <span className='truncate'>
                          {node.origin === 'source' ? node.source_name : t('subscriptions.nodes.manual')}
                        </span>
                      </Badge>
                      <Badge className='subscription-node-card__badge' variant='success'>{node.type}</Badge>
                      <Badge className='subscription-node-card__badge' variant='info' title={subscriptionNodeAddress(node)}>
                        <span className='truncate'>
                          {subscriptionNodeAddress(node) || t('subscriptions.nodes.hostMissing')}
                        </span>
                      </Badge>
                      {node.tls && (
                        <Badge className='subscription-node-card__badge' variant='success'>
                          {node.reality ? 'Reality' : 'TLS'}
                        </Badge>
                      )}
                      {node.sni && (
                        <Badge className='subscription-node-card__badge' variant='success' title={node.sni}>
                          <span className='truncate'>
                            SNI ·
                            {node.sni}
                          </span>
                        </Badge>
                      )}
                    </Button>
                    {node.hidden && (
                      <div className='subscription-node-hidden'>
                        <EyeOff aria-hidden='true' />
                        <span>{t('subscriptions.nodes.hidden')}</span>
                      </div>
                    )}
                  </article>
                ))}
              </div>
            )}
      </div>
      <footer className='subscription-pagination'>
        <SelectField
          aria-label={t('subscriptions.keys.pageSize')}
          value={size}
          onValueChange={(value) => {
            setSize(value);
          }}
          items={[5, 10, 50].map((value) => ({ value, label: t('subscriptions.keys.perPage', { count: value }) }))}
        />
        <div>
          <Button
            aria-label={t('subscriptions.keys.previous')}
            disabled={current === 1}
            onClick={() => setPage(current - 1)}
            size='icon'
            variant='outline'
          >
            <ChevronLeft />
          </Button>
          <span aria-current='page'>{current}</span>
          <Button
            aria-label={t('subscriptions.keys.next')}
            disabled={current === pages}
            onClick={() => setPage(current + 1)}
            size='icon'
            variant='outline'
          >
            <ChevronRight />
          </Button>
        </div>
      </footer>
    </div>
  );
}
