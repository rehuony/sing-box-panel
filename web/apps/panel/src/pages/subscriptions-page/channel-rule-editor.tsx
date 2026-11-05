import { useState } from 'react';
import { Zap } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Input } from '@shared/components/ui/input';
import { Button } from '@shared/components/ui/button';
import { toast } from '@shared/components/ui/toast-manager';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@shared/components/ui/dialog';

import type {
  ChannelRemoteRuleSet,
  ChannelRule,
  SubscriptionFormat,
} from '@/api/api-client';

import { InfoTooltip } from '@/components/info-tooltip';
import { SelectField } from '@/components/select-field';
import { useUnsavedChanges } from '@/hooks/use-unsaved-changes';

import {
  canAccelerateRuleURL,
  detectRuleFormat,
  directRuleURL,
  effectiveRuleURL,
  ruleFormats,
} from './channel-policy';

interface Props {
  rule: ChannelRule;
  onClose: () => void;
  otherRuleNames: string[];
  format: SubscriptionFormat;
  onSave: (value: ChannelRule) => Promise<void>;
}
export function ChannelRuleEditor({ rule, format, onClose, onSave, otherRuleNames }: Props) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState(() => structuredClone(rule));
  const [sourceFormat, setSourceFormat] = useState<string>(() =>
    rule.remote?.url && ruleFormats(format).includes(rule.remote!.format) ? rule.remote.format : '',
  );
  const [sortIndex, setSortIndex] = useState(String(rule.sort_index ?? 0));
  const [busy, setBusy] = useState(false);
  const [attempted, setAttempted] = useState(false);
  const originalFormat = rule.remote?.url && ruleFormats(format).includes(rule.remote.format) ? rule.remote.format : '';
  const dirty = JSON.stringify(draft) !== JSON.stringify(rule)
    || sourceFormat !== originalFormat || sortIndex !== String(rule.sort_index ?? 0);
  useUnsavedChanges(dirty, onClose, busy);
  const remote = draft.remote;
  const validIndex = /^-?\d+$/.test(sortIndex) && Number.isSafeInteger(Number(sortIndex));
  const duplicateName = Boolean(remote && otherRuleNames.includes(remote.name.trim()));
  function updateRemote(value: Partial<ChannelRemoteRuleSet>) {
    setDraft((current) => ({ ...current, remote: { ...current.remote!, ...value } }));
  }
  async function save() {
    setAttempted(true);
    if (!validIndex || duplicateName) return;
    if (remote && !sourceFormat) return;
    if (remote) {
      try {
        const url = new URL(remote.url);
        if (
          !remote.name.trim()
          || !['http:', 'https:'].includes(url.protocol)
          || url.username
          || url.password
          || url.hash
        ) {
          throw new Error(t('channels.invalidRule'));
        }
      } catch {
        toast.add({ title: t('channels.invalidRule'), type: 'error' });
        return;
      }
      if (sourceFormat === 'mrs' && remote.behavior === 'classical') return;
    } else if (!draft.value?.trim()) {
      toast.add({ title: t('channels.invalidRule'), type: 'error' });
      return;
    }
    setBusy(true);
    try {
      await onSave(
        remote
          ? {
              ...draft,
              sort_index: Number(sortIndex),
              remote: {
                ...remote,
                name: remote.name.trim(),
                url: directRuleURL(remote.url),
                format: sourceFormat as ChannelRemoteRuleSet['format'],
                behavior: format === 'mihomo' ? (remote.behavior ?? 'domain') : undefined,
              },
            }
          : { ...draft, sort_index: Number(sortIndex), value: draft.value!.trim() },
      );
      onClose();
    } catch (reason) {
      toast.add({ title: reason instanceof Error ? reason.message : String(reason), type: 'error' });
    } finally {
      setBusy(false);
    }
  }
  return (
    <Dialog open onOpenChange={(open) => !open && !busy && onClose()}>
      <DialogContent className='flex max-h-[min(576px,calc(100dvh-48px))] w-170 max-w-[calc(100vw-48px)] flex-col overflow-hidden sm:max-w-170 [&>[data-slot=dialog-footer]]:flex-none [&>[data-slot=dialog-header]]:flex-none'>
        <DialogHeader>
          <DialogTitle>{t(remote ? 'channels.editRemote' : 'channels.editRule')}</DialogTitle>
          <DialogDescription className='sr-only'>{t('channels.matches')}</DialogDescription>
        </DialogHeader>
        <div className='-mx-6 flex min-h-0 flex-1 flex-col overflow-auto overscroll-contain px-6 py-0.5'>
          <div className='grid grid-cols-[minmax(120px,1fr)_minmax(200px,2fr)] items-center gap-x-6 gap-y-(--form-field-gap) pt-2 pb-(--form-field-gap) max-[720px]:grid-cols-[minmax(100px,1fr)_minmax(0,2fr)] max-[720px]:gap-3 max-[480px]:grid-cols-1 max-[480px]:gap-2 [&_[aria-invalid=true]]:outline-1 [&_[aria-invalid=true]]:outline-[var(--color-danger)] [&_[data-slot=select-trigger]]:w-full [&_input:not([type=checkbox])]:w-full [&_input:not([type=checkbox])]:min-w-0 [&_input[type=checkbox]]:justify-self-start [&_textarea]:max-h-45 [&_textarea]:min-h-[90px] [&_textarea]:resize-y [&_textarea]:rounded-[var(--radius-control)] [&_textarea]:border-0 [&_textarea]:bg-[var(--color-surface-raised)] [&_textarea]:p-3'>
            <div className='flex items-center gap-1'>
              <label htmlFor='rule-sort-index'>{t('channels.sortIndex')}</label>
              <InfoTooltip label={t('channels.sortIndexHelp')}>{t('channels.sortIndexHint')}</InfoTooltip>
            </div>
            <Input id='rule-sort-index' type='text'
              aria-invalid={attempted && !validIndex}
              value={sortIndex}
              onChange={event => setSortIndex(event.target.value)} />
            {remote
              ? (
                  <>
                    <label htmlFor='rule-name'>{t('channels.ruleName')}</label>
                    <Input
                      id='rule-name'
                      aria-invalid={duplicateName}
                      aria-describedby={duplicateName ? 'rule-name-error' : undefined}
                      value={remote.name}
                      onChange={(event) => updateRemote({ name: event.target.value })}
                    />
                    {duplicateName && <p id='rule-name-error' role='alert' className='col-span-full text-sm text-destructive'>{t('channels.duplicateRuleName')}</p>}
                    <label htmlFor='rule-url'>{t('channels.url')}</label>
                    <div className='relative min-w-0 rounded-[var(--radius-control)] bg-[var(--color-surface-raised)] [&>input]:w-full [&>input]:min-w-0 [&>input]:pe-11 [&>[data-slot=button]]:absolute [&>[data-slot=button]]:end-1 [&>[data-slot=button]]:top-1/2 [&>[data-slot=button]]:-translate-y-1/2 [&>[data-slot=button]]:border-transparent [&>[data-slot=button]]:bg-transparent [&>[data-slot=button]]:text-muted-foreground [&>[data-slot=button]:hover]:text-primary [&>[data-slot=button][aria-pressed=true]]:text-primary [&>[data-slot=button][aria-pressed=true]>svg]:fill-current'>
                      <Input
                        id='rule-url'
                        autoComplete='off'
                        title={directRuleURL(remote.url)}
                        value={effectiveRuleURL(remote.url, remote.accelerated)}
                        onChange={(event) => {
                          const raw = event.target.value;
                          const url = directRuleURL(raw);
                          if (url !== directRuleURL(remote.url)) setSourceFormat(detectRuleFormat(url, format));
                          updateRemote({
                            url,
                            accelerated:
                          directRuleURL(raw) !== raw
                          || (remote.accelerated && canAccelerateRuleURL(raw)),
                          });
                        }}
                      />
                      <Button
                        variant='ghost'
                        size='icon-sm'
                        type='button'
                        aria-label={t('channels.acceleration')}
                        title={t('channels.acceleration')}
                        aria-pressed={remote.accelerated}
                        disabled={busy}
                        onClick={() => {
                          if (!remote.accelerated && !canAccelerateRuleURL(remote.url)) {
                            toast.add({ title: t('channels.accelerationUnavailable'), type: 'info' });
                            return;
                          }
                          updateRemote({
                            url: directRuleURL(remote.url),
                            accelerated: !remote.accelerated,
                          });
                        }}
                      >
                        <Zap />
                      </Button>
                    </div>
                    <label htmlFor='rule-format'>{t('channels.sourceFormat')}</label>
                    <SelectField
                      id='rule-format'
                      aria-invalid={attempted && !sourceFormat}
                      value={sourceFormat}
                      onValueChange={setSourceFormat}
                      items={[
                        { value: '', label: t('channels.chooseFormat') },
                        ...ruleFormats(format).map((value) => ({
                          value,
                          label: {
                            source: 'JSON (source)',
                            binary: 'SRS (binary)',
                            yaml: 'YAML / YML',
                            text: 'TEXT',
                            mrs: 'MRS',
                            loon: 'Loon',
                          }[value],
                        })),
                      ]}
                    />
                    {format === 'mihomo' && (
                      <>
                        <label htmlFor='rule-behavior'>{t('channels.behavior')}</label>
                        <SelectField<NonNullable<ChannelRemoteRuleSet['behavior']>>
                          id='rule-behavior'
                          aria-invalid={
                            attempted && sourceFormat === 'mrs' && remote.behavior === 'classical'
                          }
                          value={remote.behavior ?? 'domain'}
                          onValueChange={(value) =>
                            updateRemote({
                              behavior: value,
                            })
                          }
                          items={[
                            { value: 'domain', label: 'Domain' },
                            { value: 'ipcidr', label: 'IP CIDR' },
                            { value: 'classical', label: 'Classical', disabled: sourceFormat === 'mrs' },
                          ]}
                        />
                      </>
                    )}
                  </>
                )
              : (
                  <>
                    <label htmlFor='rule-kind'>{t('channels.ruleKind')}</label>
                    <SelectField<ChannelRule['kind']>
                      id='rule-kind'
                      value={draft.kind}
                      onValueChange={(value) =>
                        setDraft({ ...draft, kind: value })
                      }
                      items={(['domain', 'domain_suffix', 'domain_keyword', 'ip_cidr'] as const).map(
                        (value) => ({ value, label: t(`channels.${value}`) }),
                      )}
                    />
                    <label htmlFor='rule-value'>{t('channels.matchValue')}</label>
                    <Input
                      id='rule-value'
                      value={draft.value ?? ''}
                      onChange={(event) => setDraft({ ...draft, value: event.target.value })}
                    />
                  </>
                )}

          </div>
        </div>
        <DialogFooter>
          <Button variant='outline' disabled={busy} onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button disabled={busy} variant='default' onClick={() => void save()}>
            {t('channels.apply')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
