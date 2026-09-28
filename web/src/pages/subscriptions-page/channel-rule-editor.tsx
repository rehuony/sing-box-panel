import { useState } from 'react';
import { Zap } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import type {
  ChannelRemoteRuleSet,
  ChannelRule,
  SubscriptionFormat,
} from '@/api/api-client';

import { Button } from '@/components/ui/button';
import { toast } from '@/components/ui/toast-manager';
import { InfoTooltip } from '@/components/info-tooltip';
import { SelectField } from '@/components/select-field';
import { useUnsavedChanges } from '@/hooks/use-unsaved-changes';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';

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
      <DialogContent className='channel-rule-dialog'>
        <DialogHeader>
          <DialogTitle>{t(remote ? 'channels.editRemote' : 'channels.editRule')}</DialogTitle>
          <DialogDescription className='sr-only'>{t('channels.matches')}</DialogDescription>
        </DialogHeader>
        <div className='channel-dialog-scroll'>
          <div className='subscription-settings-fields'>
            <div className='flex items-center gap-1'>
              <label htmlFor='rule-sort-index'>{t('channels.sortIndex')}</label>
              <InfoTooltip label={t('channels.sortIndexHelp')}>{t('channels.sortIndexHint')}</InfoTooltip>
            </div>
            <input id='rule-sort-index' type='text'
              aria-invalid={attempted && !validIndex}
              value={sortIndex}
              onChange={event => setSortIndex(event.target.value)} />
            {remote
              ? (
                  <>
                    <label htmlFor='rule-name'>{t('channels.ruleName')}</label>
                    <input
                      id='rule-name'
                      aria-invalid={duplicateName}
                      aria-describedby={duplicateName ? 'rule-name-error' : undefined}
                      value={remote.name}
                      onChange={(event) => updateRemote({ name: event.target.value })}
                    />
                    {duplicateName && <p id='rule-name-error' role='alert' className='col-span-full text-sm text-destructive'>{t('channels.duplicateRuleName')}</p>}
                    <label htmlFor='rule-url'>{t('channels.url')}</label>
                    <div className='channel-rule-url'>
                      <input
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
                    <input
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
