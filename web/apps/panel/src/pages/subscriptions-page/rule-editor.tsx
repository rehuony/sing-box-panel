import type { FormEvent, ReactNode } from 'react';

import { Zap } from 'lucide-react';
import { cn } from '@shared/lib/utils';
import { useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Input } from '@shared/components/ui/input';
import { Button } from '@shared/components/ui/button';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@shared/components/ui/dialog';

import type { ChannelRemoteRuleSet, SubscriptionFormat, SubscriptionRule } from '@/api/api-client';

import { messageLine } from '@/utils/message-line';
import { IconAction } from '@/components/icon-action';
import { SelectField } from '@/components/select-field';

import { ruleType } from './rule-text';
import { canAccelerateRuleURL, detectRuleFormat, directRuleURL, effectiveRuleURL, ruleFormats } from './channel-policy';

type MatchKind = Exclude<SubscriptionRule['kind'], 'remote'>;
type Behavior = NonNullable<ChannelRemoteRuleSet['behavior']>;

const MATCH_KINDS: readonly MatchKind[] = ['domain', 'domain_suffix', 'domain_keyword', 'ip_cidr'];
const FORMAT_LABELS: Record<ChannelRemoteRuleSet['format'], string> = {
  source: 'JSON (source)', binary: 'SRS (binary)', yaml: 'YAML', text: 'TEXT', mrs: 'MRS', loon: 'loon',
};
const fieldLabel = 'text-sm';

/** A form row: the label column, then the control filling the rest; stacked below 520px. */
function Row({ children, htmlFor, label }: { children: ReactNode; htmlFor: string; label: string }) {
  return (
    <div className='grid min-w-0 grid-cols-[7rem_minmax(0,1fr)] items-center gap-x-4 gap-y-1.5 max-[520px]:grid-cols-1'>
      <label className={fieldLabel} htmlFor={htmlFor}>{label}</label>
      <div className='min-w-0 *:data-[slot=select-trigger]:w-full'>{children}</div>
    </div>
  );
}

/** The last segment of a link's path without its extension, a remote rule set's default name. */
function nameFromURL(raw: string): string {
  try {
    const segment = decodeURIComponent(new URL(directRuleURL(raw)).pathname).split('/').filter(Boolean).at(-1) ?? '';
    return segment.replace(/\.[^.]+$/, '').slice(0, 128);
  } catch {
    return '';
  }
}

/**
 * Adds or edits one rule of a client: a match on a domain, part of one or an
 * IP range, or a remote rule set the client fetches, whose formats are the
 * client's. A remote rule set's name defaults to its link's file name and
 * must differ from the others in the same list.
 */
export function RuleEditor({ client, onClose, onSave, remote, rule, takenNames }: {
  client: SubscriptionFormat;
  /** The rule being edited, or null to add one. */
  rule: SubscriptionRule | null;
  /** Whether the rule is a remote rule set. */
  remote: boolean;
  /** Remote rule set names the list's other rules use. */
  takenNames: readonly string[];
  onSave: (rule: SubscriptionRule) => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const id = useId();
  const [kind, setKind] = useState<MatchKind>(rule && rule.kind !== 'remote' ? rule.kind : 'domain_suffix');
  const [value, setValue] = useState(rule?.value ?? '');
  const [name, setName] = useState(rule?.remote?.name ?? '');
  const [url, setURL] = useState(() => rule?.remote ? effectiveRuleURL(rule.remote.url, rule.remote.accelerated) : '');
  const [format, setFormat] = useState<ChannelRemoteRuleSet['format'] | ''>(rule?.remote?.format ?? '');
  const [behavior, setBehavior] = useState<Behavior>(rule?.remote?.behavior ?? 'domain');
  const [message, setMessage] = useState('');
  const direct = directRuleURL(url);
  const accelerated = direct !== url.trim();
  const titleKey = remote ? (rule ? 'editRemote' : 'addRemote') : (rule ? 'editRule' : 'addRule');

  function submit(event: FormEvent) {
    event.preventDefault();
    if (!remote) {
      const trimmed = value.trim();
      if (!trimmed || (kind !== 'ip_cidr' && /[\s/:@,]/.test(trimmed))) {
        setMessage(t('subscriptions.ruleSets.validation.value'));
        return;
      }
      onSave({ kind, value: trimmed });
      return;
    }
    const trimmedName = name.trim() || nameFromURL(url);
    if (!URL.canParse(direct) || !['http:', 'https:'].includes(new URL(direct).protocol)) {
      setMessage(t('subscriptions.ruleSets.validation.url'));
    } else if (!trimmedName || trimmedName.includes(',')) {
      setMessage(t('subscriptions.ruleSets.validation.name'));
    } else if (takenNames.includes(trimmedName)) {
      setMessage(t('subscriptions.ruleSets.validation.nameTaken', { name: trimmedName }));
    } else if (!format) {
      setMessage(t('subscriptions.ruleSets.validation.format'));
    } else if (format === 'mrs' && behavior === 'classical') {
      setMessage(t('subscriptions.ruleSets.validation.mrsClassical'));
    } else {
      onSave({
        kind: 'remote',
        remote: { name: trimmedName, url: direct, format, accelerated, ...(client === 'mihomo' ? { behavior } : {}) },
      });
    }
  }

  return (
    <Dialog open onOpenChange={open => !open && onClose()}>
      <DialogContent className='flex max-h-[min(40rem,calc(100dvh-2rem))] flex-col'>
        <form className='flex min-h-0 min-w-0 flex-1 flex-col gap-5' noValidate onSubmit={submit}>
          <DialogHeader>
            <DialogTitle>{t(`subscriptions.ruleSets.${titleKey}`)}</DialogTitle>
            <DialogDescription className='sr-only'>{client}</DialogDescription>
          </DialogHeader>
          <div className='flex min-w-0 flex-col gap-3'>
            {remote
              ? (
                  <>
                    <Row htmlFor={`${id}-url`} label={t('subscriptions.ruleSets.field.url')}>
                      <div className='relative'>
                        <Input
                          autoComplete='off'
                          className='pe-11 font-[family-name:var(--font-data)]'
                          id={`${id}-url`}
                          onChange={(event) => {
                            const next = event.target.value;
                            setMessage('');
                            // The format follows the link until it is chosen by hand.
                            const detected = detectRuleFormat(url, client);
                            if (!format || format === detected) setFormat(detectRuleFormat(next, client));
                            setURL(next);
                          }}
                          placeholder='https://'
                          spellCheck={false}
                          value={url}
                        />
                        <span className='absolute end-1.5 top-1/2 -translate-y-1/2'>
                          <IconAction
                            aria-pressed={accelerated}
                            className={cn(accelerated && 'text-foreground [&_svg]:fill-current')}
                            label={t('subscriptions.ruleSets.accelerate')}
                            onClick={() => {
                              if (!accelerated && !canAccelerateRuleURL(direct)) {
                                setMessage(t('subscriptions.ruleSets.accelerateUnavailable'));
                                return;
                              }
                              setURL(effectiveRuleURL(direct, !accelerated));
                            }}
                            size='icon-xs'
                          >
                            <Zap aria-hidden='true' />
                          </IconAction>
                        </span>
                      </div>
                    </Row>
                    <Row htmlFor={`${id}-name`} label={t('subscriptions.ruleSets.field.name')}>
                      <Input
                        autoComplete='off'
                        className='font-[family-name:var(--font-data)]'
                        id={`${id}-name`}
                        maxLength={128}
                        onChange={(event) => {
                          setMessage('');
                          setName(event.target.value);
                        }}
                        placeholder={nameFromURL(url)}
                        spellCheck={false}
                        value={name}
                      />
                    </Row>
                    <Row htmlFor={`${id}-format`} label={t('subscriptions.ruleSets.field.format')}>
                      <SelectField<ChannelRemoteRuleSet['format'] | ''>
                        id={`${id}-format`}
                        items={[
                          { label: t('subscriptions.ruleSets.chooseFormat'), value: '' },
                          ...ruleFormats(client).map(value => ({ label: FORMAT_LABELS[value], value })),
                        ]}
                        onValueChange={(next) => {
                          setMessage('');
                          setFormat(next);
                        }}
                        value={format}
                      />
                    </Row>
                    {client === 'mihomo'
                      ? (
                          <Row htmlFor={`${id}-behavior`} label={t('subscriptions.ruleSets.field.behavior')}>
                            <SelectField<Behavior>
                              id={`${id}-behavior`}
                              items={(['domain', 'ipcidr', 'classical'] as const).map(value => ({
                                disabled: value === 'classical' && format === 'mrs',
                                label: value,
                                value,
                              }))}
                              onValueChange={setBehavior}
                              value={behavior}
                            />
                          </Row>
                        )
                      : null}
                  </>
                )
              : (
                  <>
                    <Row htmlFor={`${id}-kind`} label={t('subscriptions.ruleSets.field.kind')}>
                      <SelectField<MatchKind>
                        id={`${id}-kind`}
                        items={MATCH_KINDS.map(value => ({ label: ruleType(value), value }))}
                        onValueChange={setKind}
                        value={kind}
                      />
                    </Row>
                    <Row htmlFor={`${id}-value`} label={t('subscriptions.ruleSets.field.value')}>
                      <Input
                        autoComplete='off'
                        className='font-[family-name:var(--font-data)]'
                        id={`${id}-value`}
                        maxLength={128}
                        onChange={(event) => {
                          setMessage('');
                          setValue(event.target.value);
                        }}
                        placeholder={kind === 'ip_cidr' ? '192.168.0.0/16' : 'example.com'}
                        spellCheck={false}
                        value={value}
                      />
                    </Row>
                  </>
                )}
          </div>
          <DialogFooter className='flex-nowrap' message={messageLine(message)}>
            <Button onClick={onClose} type='button' variant='outline'>{t('common.cancel')}</Button>
            <Button type='submit'>{t('subscriptions.ruleSets.apply')}</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
