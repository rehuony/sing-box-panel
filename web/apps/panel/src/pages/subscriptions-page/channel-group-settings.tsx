import type { CSSProperties, KeyboardEvent, ReactNode } from 'react';

import { cn } from '@shared/lib/utils';
import { useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { ChevronDown, Trash2 } from 'lucide-react';
import { Input } from '@shared/components/ui/input';
import { Button } from '@shared/components/ui/button';
import { InputGroup, InputGroupAddon, InputGroupInput } from '@shared/components/ui/input-group';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@shared/components/ui/dialog';
import { DropdownMenu, DropdownMenuCheckboxItem, DropdownMenuContent, DropdownMenuGroup, DropdownMenuTrigger } from '@shared/components/ui/dropdown-menu';

import type { ChannelGroupMatch, ChannelRuleGroup, SubscriptionFormat, SubscriptionNode } from '@/api/api-client';

import { queries } from '@/api/queries';
import { compareText } from '@/utils/compare-text';
import { useApiClient } from '@/api/api-client-context';

import { GROUP_ICONS } from './channel-group-icons';
import { SourceRuleInput } from './source-rule-input';
import { FLEET_ORIGIN } from './subscription-node-pool';
import { defaultGroupHealthCheck, groupTypeName, matchedNodes } from './channel-policy';

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
            className='group/choice flex h-(--control-height) w-full min-w-0 cursor-pointer items-center justify-between gap-1.5 rounded-lg border border-input bg-[var(--color-field)] px-3 text-sm outline-hidden transition-colors hover:border-[var(--color-border-strong)] focus-visible:border-primary'
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

const STRATEGIES = ['select', 'url-test', 'fallback'] as const;

/**
 * A number field in the data face without the browser's spinner, its unit at
 * its end and an optional label at its start.
 */
function NumberField({ disabled, id, label, max, min, onChange, unit, value }: {
  id: string;
  disabled?: boolean;
  /** A label inside the field, for the second field of a row. */
  label?: string;
  unit: string;
  value: number;
  min: number;
  max: number;
  onChange: (value: number) => void;
}) {
  return (
    <InputGroup className='min-w-0'>
      {label
        ? (
            <InputGroupAddon className='me-1 border-e pe-2.5 text-[var(--color-text-secondary)]'>
              <label htmlFor={id}>{label}</label>
            </InputGroupAddon>
          )
        : null}
      <InputGroupInput
        disabled={disabled}
        className='font-[family-name:var(--font-data)]'
        id={id}
        inputMode='numeric'
        max={max}
        min={min}
        onChange={event => onChange(Number(event.target.value))}
        required
        type='number'
        value={value}
      />
      <InputGroupAddon align='inline-end' className='text-[var(--color-text-faint)]'>{unit}</InputGroupAddon>
    </InputGroup>
  );
}

/**
 * The strategy as one choice of the client's in an inset bar, as the tab bars
 * draw theirs: each with its icon from the rail and the client's name for it,
 * the current one on a raised thumb that slides to it. One Tab stop that the
 * arrow keys move and choose with. A strategy the client lacks is left out,
 * unless the group holds it, as after the channel's client changed: it then
 * shows muted and is passed over until another is chosen.
 */
function StrategyChoice({ format, labelId, onChange, value }: {
  format: SubscriptionFormat;
  labelId: string;
  value: ChannelRuleGroup['type'];
  onChange: (value: ChannelRuleGroup['type']) => void;
}) {
  const unavailable = (type: ChannelRuleGroup['type']) => format === 'sing-box' && type === 'fallback';
  const shown = STRATEGIES.filter(type => !unavailable(type) || type === value);
  const index = shown.indexOf(value);

  function move(event: KeyboardEvent<HTMLDivElement>) {
    const step = { ArrowDown: 1, ArrowLeft: -1, ArrowRight: 1, ArrowUp: -1 }[event.key];
    if (!step) return;
    event.preventDefault();
    let next = index;
    do next = (next + step + shown.length) % shown.length;
    while (unavailable(shown[next]!));
    onChange(shown[next]!);
    event.currentTarget.querySelectorAll<HTMLElement>('[role="radio"]')[next]?.focus();
  }

  return (
    <div
      aria-labelledby={labelId}
      className='relative isolate grid h-(--control-height) grid-cols-[repeat(var(--strategies),minmax(0,1fr))] gap-0.5 rounded-lg border bg-[var(--color-canvas)] p-[3px]'
      onKeyDown={move}
      role='radiogroup'
      style={{ '--strategies': shown.length } as CSSProperties}
    >
      {/* The thumb sits behind the choices, so the current one's text stays on top of it. */}
      <span
        aria-hidden='true'
        className='absolute inset-y-[3px] start-[calc(3px+var(--strategy)*(100%-6px+2px)/var(--strategies))] -z-10 w-[calc((100%-6px-(var(--strategies)-1)*2px)/var(--strategies))] rounded-[calc(var(--radius-card)-4px)] bg-popover shadow-[var(--shadow-thumb)] ring-1 ring-border transition-[inset-inline-start] duration-[var(--dur-short)] ease-[var(--ease-out)] motion-reduce:transition-none'
        style={{ '--strategy': index } as CSSProperties}
      />
      {shown.map((type) => {
        const Icon = GROUP_ICONS[type];
        const off = unavailable(type);
        return (
          <button
            aria-checked={type === value}
            aria-disabled={off || undefined}
            className={cn(
              'flex min-w-0 items-center justify-center gap-1.5 rounded-[calc(var(--radius-card)-4px)] border border-transparent px-1.5 text-compact text-muted-foreground outline-none transition-colors duration-[var(--dur-micro)] ease-out focus-visible:border-primary motion-reduce:transition-none [&_svg]:size-3.5 [&_svg]:flex-none',
              off ? 'opacity-50' : 'cursor-pointer hover:text-foreground aria-checked:text-foreground',
              !off && type !== value && 'hover:bg-[var(--color-hover)]',
            )}
            key={type}
            onClick={() => {
              if (!off) onChange(type);
            }}
            role='radio'
            tabIndex={type === value ? 0 : -1}
            type='button'
          >
            <Icon aria-hidden='true' />
            <span className='truncate'>{groupTypeName(type, format)}</span>
          </button>
        );
      })}
    </div>
  );
}

/**
 * A strategy group's settings: its name and strategy, the health check that
 * unfolds below them for a strategy that tests, then what it takes from the
 * pool besides its members under a heading counting those nodes: the name
 * rules, the origins and the protocols a node must match, each counting every
 * value while it names none. Its delete sits at the footer's start. The
 * dialog's top stays where its tallest form centres it, so the health check
 * grows and folds below the strategy and nothing above it moves.
 */
export function ChannelGroupSettings({ busy, format, nodes, onApply, onChange, onClose, onDelete, value }: {
  value: ChannelRuleGroup;
  format: SubscriptionFormat;
  /** The pool, whose protocols the match offers. */
  nodes: readonly SubscriptionNode[];
  busy: boolean;
  onChange: (value: ChannelRuleGroup) => void;
  onApply: (value: ChannelRuleGroup) => void;
  onClose: () => void;
  /** Asks to delete the group. */
  onDelete: () => void;
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
  const matched = matchedNodes(value, nodes).length;
  const checking = value.type !== 'select';
  // The health check keeps the last testing strategy's form while it folds away under select.
  const [checkType, setCheckType] = useState<'url-test' | 'fallback'>(value.type === 'fallback' ? 'fallback' : 'url-test');
  if (value.type !== 'select' && value.type !== checkType) setCheckType(value.type);
  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className='top-[max(1rem,calc(50dvh-19rem))] flex max-h-[min(44rem,calc(100dvh-2rem))] translate-y-0 flex-col overflow-hidden max-[520px]:top-4'>
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
              <Row label={t('channels.groupType')} labelId={`${id}-type`}>
                <StrategyChoice format={format} labelId={`${id}-type`} onChange={(type) => onChange({ ...value, type })} value={value.type} />
              </Row>
            </div>
            {/* The health check eases open and shut, fading sooner; folded, it takes no focus. */}
            <div
              className={cn(
                'grid [transition:grid-template-rows_var(--duration-medium)_var(--ease-out),opacity_var(--dur-short)_ease-out] motion-reduce:[transition:none]',
                checking ? 'grid-rows-[1fr]' : 'grid-rows-[0fr] opacity-0',
              )}
              inert={!checking}
            >
              <div className='min-h-0 overflow-hidden'>
                <div className='flex min-w-0 flex-col gap-3 pt-3'>
                  <Row htmlFor={`${id}-url`} label={t('channels.testURL')}>
                    <Input disabled={busy || !checking} id={`${id}-url`} maxLength={4096} onChange={(event) => setHealth({ url: event.target.value })} required type='url' value={health.url} />
                  </Row>
                  <Row htmlFor={`${id}-interval`} label={t('channels.testInterval')}>
                    <div className={cn('grid gap-2', checkType === 'url-test' && 'grid-cols-2')}>
                      <NumberField disabled={!checking} id={`${id}-interval`} max={86400} min={60} onChange={interval => setHealth({ interval })} unit={t('channels.unitSeconds')} value={health.interval} />
                      {checkType === 'url-test'
                        ? <NumberField disabled={!checking} id={`${id}-tolerance`} label={t('channels.testTolerance')} max={65535} min={0} onChange={tolerance => setHealth({ tolerance })} unit='ms' value={health.tolerance} />
                        : null}
                    </div>
                  </Row>
                </div>
              </div>
            </div>
            <section aria-labelledby={`${id}-match`} className='mt-5 flex min-w-0 flex-col gap-3 border-t pt-3.5'>
              <div className='flex items-center justify-between gap-3'>
                <h3 className='text-xs text-[var(--color-text-secondary)]' id={`${id}-match`}>{t('channels.autoMatch')}</h3>
                <output className='text-xs text-[var(--color-text-faint)]'>
                  {t('channels.matchHits', { count: matched })}
                </output>
              </div>
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
            {/* Shaped as Cancel, only its text in danger, so it stays apart without standing out. */}
            {/* Below 520px it keeps its icon alone, square, so the footer stays on one line. */}
            <Button
              className='me-auto text-[var(--color-danger-text)] hover:border-[var(--color-danger-border)] hover:text-[var(--color-danger-text)] max-[520px]:w-(--control-height) max-[520px]:px-0'
              disabled={busy}
              onClick={onDelete}
              type='button'
              variant='outline'
            >
              <Trash2 aria-hidden='true' data-icon='inline-start' />
              <span className='max-[520px]:sr-only'>{t('channels.deleteGroup')}</span>
            </Button>
            <Button onClick={onClose} type='button' variant='outline'>{t('common.cancel')}</Button>
            <Button disabled={busy || !value.name.trim()} type='submit'>{t('channels.apply')}</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
