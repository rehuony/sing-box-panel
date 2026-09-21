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
  directRuleURL,
  effectiveRuleURL,
  ruleFormats,
} from './channel-policy';

interface Props {
  rule: ChannelRule;
  onClose: () => void;
  format: SubscriptionFormat;
  onSave: (value: ChannelRule) => Promise<void>;
}
export function ChannelRuleEditor({ rule, format, onClose, onSave }: Props) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState(() => structuredClone(rule));
  const [sourceFormat, setSourceFormat] = useState<string>(() =>
    rule.remote?.url && ruleFormats(format).includes(rule.remote!.format) ? rule.remote.format : '',
  );
  const [busy, setBusy] = useState(false);
  const [attempted, setAttempted] = useState(false);
  const originalFormat = rule.remote?.url && ruleFormats(format).includes(rule.remote.format) ? rule.remote.format : '';
  useUnsavedChanges(JSON.stringify(draft) !== JSON.stringify(rule) || sourceFormat !== originalFormat, onClose, busy);
  const remote = draft.remote;
  function updateRemote(value: Partial<ChannelRemoteRuleSet>) {
    setDraft((current) => ({ ...current, remote: { ...current.remote!, ...value } }));
  }
  async function save() {
    setAttempted(true);
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
          || !Number.isInteger(remote.update_interval)
          || remote.update_interval < 60
          || remote.update_interval > 2592000
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
      const value: ChannelRule = { ...draft, enabled: true, exit: { kind: 'group-default' } };
      await onSave(
        remote
          ? {
              ...value,
              remote: {
                ...remote,
                name: remote.name.trim(),
                url: directRuleURL(remote.url),
                format: sourceFormat as ChannelRemoteRuleSet['format'],
                behavior: format === 'sing-box' ? undefined : (remote.behavior ?? 'domain'),
              },
            }
          : { ...value, value: draft.value!.trim() },
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
            {remote
              ? (
                  <>
                    <label htmlFor='rule-name'>{t('channels.ruleName')}</label>
                    <input
                      id='rule-name'
                      value={remote.name}
                      onChange={(event) => updateRemote({ name: event.target.value })}
                    />
                    <label htmlFor='rule-url'>{t('channels.url')}</label>
                    <div className='channel-rule-url'>
                      <input
                        id='rule-url'
                        autoComplete='off'
                        title={directRuleURL(remote.url)}
                        value={effectiveRuleURL(remote.url, remote.accelerated)}
                        onChange={(event) => {
                          const raw = event.target.value;
                          updateRemote({
                            url: directRuleURL(raw),
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
                    <label htmlFor='rule-interval'>{t('channels.interval')}</label>
                    <input
                      id='rule-interval'
                      type='number'
                      min={60}
                      max={2592000}
                      step={1}
                      value={remote.update_interval}
                      onChange={(event) =>
                        updateRemote({ update_interval: Number(event.target.value) })
                      }
                    />
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
            {t('channels.done')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
