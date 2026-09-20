import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ChevronLeft, ChevronRight, Eye, EyeOff, MoreHorizontal } from 'lucide-react';

import type { SubscriptionNodeSummary } from '@/api/api-client';

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
                    className={`subscription-node-card${node.hidden ? ' is-hidden' : ''}`}
                    key={node.id}
                  >
                    <header>
                      <span
                        aria-hidden='true'
                        className={`subscription-node-dot${node.available ? '' : ' is-unavailable'}`}
                      />
                      <button
                        className='subscription-node-name'
                        onClick={() => onOpen(node)}
                        title={node.name}
                        type='button'
                      >
                        {node.name}
                      </button>
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
                              size='icon'
                              variant='ghost'
                            >
                              {node.hidden ? <EyeOff aria-hidden='true' /> : <Eye aria-hidden='true' />}
                            </Button>
                          )}
                      <Button
                        aria-label={t('subscriptions.nodes.details', { name: node.name })}
                        onClick={() => onOpen(node)}
                        size='icon'
                        variant='ghost'
                      >
                        <MoreHorizontal aria-hidden='true' />
                      </Button>
                    </header>
                    {node.hidden
                      ? (
                          <div className='subscription-node-hidden'>
                            <EyeOff aria-hidden='true' />
                            <span>{t('subscriptions.nodes.hidden')}</span>
                          </div>
                        )
                      : (
                          <button
                            className='subscription-node-card__body'
                            onClick={() => onOpen(node)}
                            type='button'
                          >
                            <span className='subscription-node-badge is-source' title={node.source_name}>
                              {node.origin === 'source'
                                ? node.source_name
                                : t('subscriptions.nodes.manual')}
                            </span>
                            <span className='subscription-node-badge is-protocol'>{node.type}</span>
                            <span className='subscription-node-badge is-address' title={node.server}>
                              {subscriptionNodeAddress(node) || t('subscriptions.nodes.hostMissing')}
                            </span>
                            {node.tls && (
                              <span className='subscription-node-badge is-security'>
                                {node.reality ? 'Reality' : 'TLS'}
                              </span>
                            )}
                            {node.sni && (
                              <span className='subscription-node-badge is-security' title={node.sni}>
                                SNI ·
                                {node.sni}
                              </span>
                            )}
                          </button>
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
            variant='ghost'
          >
            <ChevronLeft />
          </Button>
          <span aria-current='page'>{current}</span>
          <Button
            aria-label={t('subscriptions.keys.next')}
            disabled={current === pages}
            onClick={() => setPage(current + 1)}
            size='icon'
            variant='ghost'
          >
            <ChevronRight />
          </Button>
        </div>
      </footer>
    </div>
  );
}
