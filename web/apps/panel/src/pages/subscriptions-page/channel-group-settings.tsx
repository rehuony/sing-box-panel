import type { ReactNode } from 'react';

import { useId } from 'react';
import { cn } from '@shared/lib/utils';
import { ChevronDown } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { Input } from '@shared/components/ui/input';
import { Button } from '@shared/components/ui/button';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@shared/components/ui/dialog';
import { DropdownMenu, DropdownMenuCheckboxItem, DropdownMenuContent, DropdownMenuGroup, DropdownMenuTrigger } from '@shared/components/ui/dropdown-menu';

import type { ChannelGroupMatch, ChannelRuleGroup, SubscriptionFormat, SubscriptionNode } from '@/api/api-client';

import { queries } from '@/api/queries';
import { compareText } from '@/utils/compare-text';
import { useApiClient } from '@/api/api-client-context';
import { SelectField } from '@/components/select-field';

import { SourceRuleInput } from './source-rule-input';
import { FLEET_ORIGIN } from './subscription-node-pool';
import { defaultGroupHealthCheck, groupTypeName } from './channel-policy';

const fieldLabel = 'text-sm';
const noMatch: ChannelGroupMatch = { include: [], exclude: [], origins: [], protocols: [] };

/** A form row: the label column, then the control filling the rest; stacked below 520px. */
function Row({ children, htmlFor, label, labelId }: {
  children: ReactNode;
  htmlFor?: string;
  label: string;
  labelId?: string;
}) {
  return (
    <div className='grid min-w-0 grid-cols-[7rem_minmax(0,1fr)] items-start gap-x-4 gap-y-1.5 max-[520px]:grid-cols-1'>
      <div className='flex min-h-(--control-height) items-center'>
        <label className={fieldLabel} htmlFor={htmlFor} id={labelId}>{label}</label>
      </div>
      <div className='min-w-0 *:data-[slot=select-trigger]:w-full'>{children}</div>
    </div>
  );
}

/**
 * Picks any number of values from a list through a select's trigger, which
 * shows the chosen names or that every value counts; chosen values no longer
 * listed stay choosable by their value.
 */
function MultiChoice({ allLabel, id, items, labelId, onChange, value }: {
  id: string;
  labelId: string;
  allLabel: string;
  items: { value: string; label: string }[];
  value: readonly string[];
  onChange: (value: string[]) => void;
}) {
  const { t } = useTranslation();
  const valueId = useId();
  const listed = new Set(items.map(item => item.value));
  const choices = [...items, ...value.filter(each => !listed.has(each)).map(each => ({ value: each, label: each }))];
  const names = value.map(each => choices.find(item => item.value === each)?.label ?? each);
  const shown = names.length ? names.join(t('subscriptions.sources.protocolSeparator')) : allLabel;
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        aria-labelledby={`${labelId} ${valueId}`}
        id={id}
        render={(
          <button
            className='group/choice flex h-(--control-height) w-full min-w-0 cursor-pointer items-center justify-between gap-1.5 rounded-lg border border-input bg-[var(--color-field)] px-3 text-sm outline-hidden transition-colors hover:border-[var(--color-border-strong)] focus-visible:outline-1 focus-visible:-outline-offset-1 focus-visible:outline-input-focus focus-visible:outline-solid'
            type='button'
          />
        )}
      >
        <span className={cn('min-w-0 truncate', !names.length && 'text-muted-foreground')} id={valueId} title={shown}>{shown}</span>
        <ChevronDown aria-hidden='true' className='size-4 flex-none text-muted-foreground transition-transform duration-[var(--dur-short)] ease-[var(--ease-out)] group-data-popup-open/choice:rotate-180 motion-reduce:transition-none' />
      </DropdownMenuTrigger>
      <DropdownMenuContent className='max-h-[min(17.5rem,var(--available-height))]'>
        <DropdownMenuGroup>
          {choices.map(item => (
            <DropdownMenuCheckboxItem
              checked={value.includes(item.value)}
              key={item.value}
              onCheckedChange={checked => onChange(checked
                ? [...value, item.value]
                : value.filter(each => each !== item.value))}
            >
              <span className='min-w-0 break-words'>{item.label}</span>
            </DropdownMenuCheckboxItem>
          ))}
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

/**
 * A strategy group's settings: its name, strategy and health check, then
 * what it takes from the pool besides its members: the name rules, the
 * origins and the protocols a node must match, each counting every value
 * while it names none.
 */
export function ChannelGroupSettings({ busy, format, nodes, onApply, onChange, onClose, value }: {
  value: ChannelRuleGroup;
  format: SubscriptionFormat;
  /** The pool, whose protocols the match offers. */
  nodes: readonly SubscriptionNode[];
  busy: boolean;
  onChange: (value: ChannelRuleGroup) => void;
  onApply: (value: ChannelRuleGroup) => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const client = useApiClient();
  const id = useId();
  const sources = useQuery(queries.sources(client)).data;
  const health = value.health_check ?? defaultGroupHealthCheck;
  const match = value.match ?? noMatch;
  const setHealth = (next: Partial<typeof health>) => onChange({ ...value, health_check: { ...health, ...next } });
  const setMatch = (next: Partial<ChannelGroupMatch>) => onChange({ ...value, match: { ...match, ...next } });
  const origins = [
    { value: FLEET_ORIGIN, label: t('subscriptions.pool.fleetOrigin') },
    ...[...sources ?? []].sort((left, right) => compareText(left.name, right.name)).map(source => ({ value: `source:${source.id}`, label: source.name })),
  ];
  const present = [...new Set(nodes.map(node => node.protocol))].sort(compareText);
  const protocols = present.map(protocol => ({ value: protocol, label: protocol }));
  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className='flex max-h-[min(44rem,calc(100dvh-2rem))] flex-col overflow-hidden'>
        <form
          className='flex min-h-0 min-w-0 flex-1 flex-col gap-5'
          onSubmit={(event) => {
            event.preventDefault();
            if (!busy && value.name.trim()) onApply(value);
          }}
        >
          <DialogHeader>
            <DialogTitle>{t('channels.groupSettingsTitle')}</DialogTitle>
            <DialogDescription className='sr-only'>{value.name}</DialogDescription>
          </DialogHeader>
          <div className='-mx-6 min-h-0 flex-1 overflow-y-auto overscroll-contain px-6'>
            <div className='flex min-w-0 flex-col gap-3'>
              <Row htmlFor={`${id}-name`} label={t('channels.groupName')}>
                <Input autoComplete='off' disabled={busy} id={`${id}-name`} maxLength={128} onChange={(event) => onChange({ ...value, name: event.target.value })} value={value.name} />
              </Row>
              <Row htmlFor={`${id}-type`} label={t('channels.groupType')}>
                <SelectField<ChannelRuleGroup['type']>
                  disabled={busy}
                  id={`${id}-type`}
                  items={(['select', 'url-test', 'fallback'] as const).map((type) => ({ value: type, label: groupTypeName(type, format), disabled: format === 'sing-box' && type === 'fallback' }))}
                  onValueChange={(type) => onChange({ ...value, type })}
                  value={value.type}
                />
              </Row>
              {value.type !== 'select'
                ? (
                    <>
                      <Row htmlFor={`${id}-url`} label={t('channels.testURL')}>
                        <Input disabled={busy} id={`${id}-url`} maxLength={4096} onChange={(event) => setHealth({ url: event.target.value })} required type='url' value={health.url} />
                      </Row>
                      <Row htmlFor={`${id}-interval`} label={t('channels.testInterval')}>
                        <Input disabled={busy} id={`${id}-interval`} max={86400} min={60} onChange={(event) => setHealth({ interval: Number(event.target.value) })} required type='number' value={health.interval} />
                      </Row>
                      {value.type === 'url-test'
                        ? (
                            <Row htmlFor={`${id}-tolerance`} label={t('channels.testTolerance')}>
                              <Input disabled={busy} id={`${id}-tolerance`} max={65535} min={0} onChange={(event) => setHealth({ tolerance: Number(event.target.value) })} required type='number' value={health.tolerance} />
                            </Row>
                          )
                        : null}
                    </>
                  )
                : null}
            </div>
            <section aria-labelledby={`${id}-match`} className='mt-5 flex min-w-0 flex-col gap-3 border-t pt-4'>
              <h3 className='text-xs text-[var(--color-text-secondary)]' id={`${id}-match`}>{t('channels.autoMatch')}</h3>
              <div className='flex min-w-0 flex-col gap-3'>
                <Row htmlFor={`${id}-include`} label={t('channels.matchInclude')}>
                  <SourceRuleInput id={`${id}-include`} invalidRules={[]} onChange={include => setMatch({ include })} value={match.include} />
                </Row>
                <Row htmlFor={`${id}-exclude`} label={t('channels.matchExclude')}>
                  <SourceRuleInput id={`${id}-exclude`} invalidRules={[]} onChange={exclude => setMatch({ exclude })} value={match.exclude} />
                </Row>
                <Row htmlFor={`${id}-origins`} label={t('channels.matchOrigins')} labelId={`${id}-origins-label`}>
                  <MultiChoice allLabel={t('channels.matchAll')} id={`${id}-origins`} items={origins} labelId={`${id}-origins-label`} onChange={next => setMatch({ origins: next })} value={match.origins} />
                </Row>
                <Row htmlFor={`${id}-protocols`} label={t('channels.matchProtocols')} labelId={`${id}-protocols-label`}>
                  <MultiChoice allLabel={t('channels.matchAll')} id={`${id}-protocols`} items={protocols} labelId={`${id}-protocols-label`} onChange={next => setMatch({ protocols: next })} value={match.protocols} />
                </Row>
              </div>
            </section>
          </div>
          <DialogFooter>
            <Button onClick={onClose} type='button' variant='outline'>{t('common.cancel')}</Button>
            <Button disabled={busy || !value.name.trim()} type='submit'>{t('channels.apply')}</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
