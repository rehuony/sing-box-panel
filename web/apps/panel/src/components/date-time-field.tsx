import { cn } from '@shared/lib/utils';
import { useTranslation } from 'react-i18next';
import { useEffect, useRef, useState } from 'react';
import { Button } from '@shared/components/ui/button';
import { CalendarDays, ChevronLeft, ChevronRight, X } from 'lucide-react';
import { Popover, PopoverContent, PopoverTrigger } from '@shared/components/ui/popover';

import { IconAction } from '@/components/icon-action';

const pad = (value: number) => String(value).padStart(2, '0');
const dayKey = (day: Date) => `${day.getFullYear()}-${pad(day.getMonth() + 1)}-${pad(day.getDate())}`;
const startOfDay = (value: Date) => new Date(value.getFullYear(), value.getMonth(), value.getDate());
const startOfMonth = (value: Date) => new Date(value.getFullYear(), value.getMonth(), 1);

/** `H:mm` or `HH:mm` as `HH:mm`, or null. */
function readTime(text: string): string | null {
  const match = /^([01]?\d|2[0-3]):([0-5]\d)$/.exec(text.trim());
  return match ? `${pad(Number(match[1]))}:${match[2]}` : null;
}

function at(day: Date, time: string): Date {
  const [hours, minutes] = time.split(':').map(Number);
  return new Date(day.getFullYear(), day.getMonth(), day.getDate(), hours, minutes);
}

/**
 * A date and time chosen from a calendar: the field shows it in the data
 * face with how many days away it is, and its popover holds the month, its
 * days from Monday and the time, with an action at the end that clears it.
 */
export function DateTimeField({
  clearLabel, defaultOpen = false, id, invalid, min, onChange, onClear, value,
}: {
  /** The clear action's name, which says what clearing returns to. */
  clearLabel: string;
  defaultOpen?: boolean;
  id?: string;
  invalid?: boolean;
  /** The earliest day it offers. */
  min: Date;
  onChange: (value: Date | null) => void;
  onClear: () => void;
  value: Date | null;
}) {
  const { t, i18n } = useTranslation();
  const language = i18n.resolvedLanguage;
  const [open, setOpen] = useState(defaultOpen);
  const [month, setMonth] = useState(() => startOfMonth(value ?? min));
  const [time, setTime] = useState(() => value ? `${pad(value.getHours())}:${pad(value.getMinutes())}` : '23:59');
  const [timeInvalid, setTimeInvalid] = useState(false);
  const [focused, setFocused] = useState(() => startOfDay(value ?? min));
  const focusRef = useRef<HTMLButtonElement>(null);
  // Today as of opening the form; days before min stay out of reach anyway.
  const [today] = useState(() => startOfDay(new Date()));
  const first = startOfDay(min);
  const selected = value ? startOfDay(value) : null;
  const days = new Date(month.getFullYear(), month.getMonth() + 1, 0).getDate();
  const lead = (month.getDay() + 6) % 7;
  // 2024-01-01 was a Monday; the week starts there.
  const weekdays = Array.from({ length: 7 }, (_, index) => new Date(2024, 0, 1 + index))
    .map(day => ({ key: dayKey(day), label: day.toLocaleDateString(language, { weekday: 'narrow' }) }));
  const away = value ? Math.round((startOfDay(value).getTime() - today.getTime()) / 86_400_000) : null;

  // Keyboard moves keep the focused day in view and focused.
  useEffect(() => {
    if (open) focusRef.current?.focus();
  }, [focused, open]);

  function moveFocus(target: Date) {
    if (target < first) return;
    setFocused(target);
    setMonth(startOfMonth(target));
  }
  function choose(day: Date) {
    setFocused(day);
    onChange(at(day, readTime(time) ?? '23:59'));
  }
  function commitTime(text = time): boolean {
    const read = readTime(text);
    setTimeInvalid(!read);
    if (!read) return false;
    setTime(read);
    if (selected) onChange(at(selected, read));
    return true;
  }

  return (
    <div
      className={cn(
        'flex h-(--control-height) min-w-0 items-center rounded-lg border border-input bg-[var(--color-field)] transition-colors has-focus-visible:border-[var(--input-focus)]',
        invalid && 'border-destructive',
      )}
    >
      <Popover
        onOpenChange={(next) => {
          if (!next) commitTime();
          setOpen(next);
        }}
        open={open}
      >
        <PopoverTrigger
          aria-invalid={invalid || undefined}
          className='flex h-full min-w-0 flex-1 cursor-pointer items-center gap-2 ps-3 pe-1 text-start outline-none'
          id={id}
          onClick={() => {
            setFocused(selected ?? first);
            setMonth(startOfMonth(selected ?? first));
          }}
        >
          <CalendarDays aria-hidden='true' className='size-4 flex-none text-muted-foreground' />
          {value
            ? <span className='min-w-0 truncate font-[family-name:var(--font-data)] text-compact'>{`${dayKey(value)} ${pad(value.getHours())}:${pad(value.getMinutes())}`}</span>
            : <span className='min-w-0 truncate text-muted-foreground'>{t('common.dateTime.placeholder')}</span>}
          {away !== null && away >= 0
            ? <span className='ms-auto flex-none ps-2 text-xs text-muted-foreground'>{away === 0 ? t('common.dateTime.today') : t('common.dateTime.inDays', { count: away })}</span>
            : null}
        </PopoverTrigger>
        <PopoverContent align='start' aria-label={t('common.dateTime.placeholder')} className='w-[18.5rem] gap-0 p-0' initialFocus={focusRef} sideOffset={6}>
          <div className='flex h-11 items-center justify-between ps-3.5 pe-2'>
            <span className='text-sm font-medium'>{month.toLocaleDateString(language, { year: 'numeric', month: 'long' })}</span>
            <span className='flex gap-0.5'>
              <IconAction
                label={t('common.dateTime.previousMonth')}
                onClick={() => {
                  if (month > startOfMonth(first)) setMonth(new Date(month.getFullYear(), month.getMonth() - 1, 1));
                }}
                size='icon-xs'
              >
                <ChevronLeft aria-hidden='true' />
              </IconAction>
              <IconAction label={t('common.dateTime.nextMonth')} onClick={() => setMonth(new Date(month.getFullYear(), month.getMonth() + 1, 1))} size='icon-xs'>
                <ChevronRight aria-hidden='true' />
              </IconAction>
            </span>
          </div>
          <div aria-hidden='true' className='grid grid-cols-7 px-2 text-center text-xs leading-6 text-[var(--color-text-faint)]'>
            {weekdays.map(weekday => <span key={weekday.key}>{weekday.label}</span>)}
          </div>
          <div className='grid grid-cols-7 gap-y-0.5 px-2 pb-2' role='group'>
            {Array.from({ length: lead }, (_, index) => <span key={`lead-${index}`} />)}
            {Array.from({ length: days }, (_, index) => {
              const day = new Date(month.getFullYear(), month.getMonth(), index + 1);
              const past = day < first;
              const current = selected !== null && day.getTime() === selected.getTime();
              const tabbable = day.getTime() === focused.getTime();
              return (
                <button
                  aria-label={day.toLocaleDateString(language, { dateStyle: 'full' })}
                  aria-pressed={current}
                  className={cn(
                    'relative flex h-8 items-center justify-center rounded-[var(--radius-control)] border border-transparent font-[family-name:var(--font-data)] text-compact tabular-nums outline-none focus-visible:bg-[var(--color-hover)]',
                    past ? 'cursor-default text-[var(--color-text-faint)] opacity-50' : 'cursor-pointer hover:bg-[var(--color-hover)]',
                    current && 'border-primary/50 bg-accent font-medium text-primary hover:bg-accent',
                    day.getTime() === today.getTime() && 'after:absolute after:bottom-1 after:size-[3px] after:rounded-full after:bg-primary',
                  )}
                  disabled={past}
                  key={index}
                  onClick={() => choose(day)}
                  onKeyDown={(event) => {
                    const step = { ArrowLeft: -1, ArrowRight: 1, ArrowUp: -7, ArrowDown: 7 }[event.key];
                    if (step === undefined) return;
                    event.preventDefault();
                    moveFocus(new Date(day.getFullYear(), day.getMonth(), day.getDate() + step));
                  }}
                  ref={tabbable ? focusRef : undefined}
                  tabIndex={tabbable ? 0 : -1}
                  type='button'
                >
                  {index + 1}
                </button>
              );
            })}
          </div>
          <div className='flex items-center gap-2 border-t py-2 ps-3.5 pe-2'>
            <label className='flex-none text-compact whitespace-nowrap text-[var(--color-text-secondary)]' htmlFor={id ? `${id}-time` : undefined}>{t('common.dateTime.time')}</label>
            <input
              aria-invalid={timeInvalid || undefined}
              className='h-(--control-height-sm) w-18 rounded-[var(--radius-control)] border border-input bg-[var(--color-field)] text-center font-[family-name:var(--font-data)] text-compact outline-none focus-visible:border-[var(--input-focus)] aria-invalid:border-destructive'
              id={id ? `${id}-time` : undefined}
              inputMode='numeric'
              maxLength={5}
              onBlur={() => commitTime()}
              onChange={(event) => {
                setTime(event.target.value);
                setTimeInvalid(false);
              }}
              onKeyDown={(event) => {
                if (event.key === 'Enter' && !event.nativeEvent.isComposing) {
                  event.preventDefault();
                  if (commitTime()) setOpen(false);
                }
              }}
              value={time}
            />
            <span className='flex min-w-0 flex-1'>
              {['00:00', '23:59'].map(preset => (
                <button
                  className='h-7 cursor-pointer rounded-[var(--radius-pill)] px-1.5 font-[family-name:var(--font-data)] text-xs text-[var(--color-text-secondary)] outline-none hover:bg-[var(--color-hover)] focus-visible:bg-[var(--color-hover)]'
                  key={preset}
                  onClick={() => commitTime(preset)}
                  type='button'
                >
                  {preset}
                </button>
              ))}
            </span>
            <Button
              onClick={() => {
                if (commitTime()) setOpen(false);
              }}
              size='sm'
              type='button'
            >
              {t('common.dateTime.done')}
            </Button>
          </div>
        </PopoverContent>
      </Popover>
      <IconAction className='me-1' label={clearLabel} onClick={onClear} size='icon-xs'>
        <X aria-hidden='true' />
      </IconAction>
    </div>
  );
}
