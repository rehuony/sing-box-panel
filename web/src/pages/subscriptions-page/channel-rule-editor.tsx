import { useState } from 'react';
import { Zap } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import type {
  ChannelRemoteRuleSet,
  ChannelRule,
  SubscriptionFormat,
  SubscriptionNodeSummary,
} from '@/api/api-client';

import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';

import { ChannelRouteExitSelect } from './channel-route-exit';
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
  nodes: SubscriptionNodeSummary[];
  onSave: (value: ChannelRule) => Promise<void>;
}
export function ChannelRuleEditor({ rule, format, nodes, onClose, onSave }: Props) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState(() => structuredClone(rule));
  const [sourceFormat, setSourceFormat] = useState<string>(() =>
    rule.remote?.url && ruleFormats(format).includes(rule.remote!.format) ? rule.remote.format : '',
  );
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [attempted, setAttempted] = useState(false);
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
        setError(t('channels.invalidRule'));
        return;
      }
      if (sourceFormat === 'mrs' && remote.behavior === 'classical') return;
    } else if (!draft.value?.trim()) {
      setError(t('channels.invalidRule'));
      return;
    }
    setBusy(true);
    setError('');
    try {
      await onSave(
        remote
          ? {
              ...draft,
              remote: {
                ...remote,
                name: remote.name.trim(),
                url: directRuleURL(remote.url),
                format: sourceFormat as ChannelRemoteRuleSet['format'],
                behavior: format === 'sing-box' ? undefined : (remote.behavior ?? 'domain'),
              },
            }
          : { ...draft, value: draft.value!.trim() },
      );
      onClose();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : String(reason));
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
                        variant='outline'
                        size='icon'
                        aria-label={t('channels.acceleration')}
                        title={t('channels.acceleration')}
                        aria-pressed={remote.accelerated}
                        disabled={!canAccelerateRuleURL(remote.url)}
                        onClick={() =>
                          updateRemote({
                            url: directRuleURL(remote.url),
                            accelerated: !remote.accelerated,
                          })
                        }
                      >
                        <Zap />
                      </Button>
                    </div>
                    <label htmlFor='rule-format'>{t('channels.sourceFormat')}</label>
                    <select
                      id='rule-format'
                      aria-invalid={attempted && !sourceFormat}
                      value={sourceFormat}
                      onChange={(event) => setSourceFormat(event.target.value)}
                    >
                      <option value=''>{t('channels.chooseFormat')}</option>
                      {ruleFormats(format).map((value) => (
                        <option key={value} value={value}>
                          {
                            {
                              source: 'JSON (source)',
                              binary: 'SRS (binary)',
                              yaml: 'YAML / YML',
                              text: 'TEXT',
                              mrs: 'MRS',
                            }[value]
                          }
                        </option>
                      ))}
                    </select>
                    {format === 'mihomo' && (
                      <>
                        <label htmlFor='rule-behavior'>{t('channels.behavior')}</label>
                        <select
                          id='rule-behavior'
                          aria-invalid={
                            attempted && sourceFormat === 'mrs' && remote.behavior === 'classical'
                          }
                          value={remote.behavior ?? 'domain'}
                          onChange={(event) =>
                            updateRemote({
                              behavior: event.target.value as ChannelRemoteRuleSet['behavior'],
                            })
                          }
                        >
                          <option value='domain'>Domain</option>
                          <option value='ipcidr'>IP CIDR</option>
                          <option disabled={sourceFormat === 'mrs'} value='classical'>
                            Classical
                          </option>
                        </select>
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
                    <select
                      id='rule-kind'
                      value={draft.kind}
                      onChange={(event) =>
                        setDraft({ ...draft, kind: event.target.value as ChannelRule['kind'] })
                      }
                    >
                      {(['domain', 'domain_suffix', 'domain_keyword', 'ip_cidr'] as const).map(
                        (kind) => (
                          <option key={kind} value={kind}>
                            {t(`channels.${kind}`)}
                          </option>
                        ),
                      )}
                    </select>
                    <label htmlFor='rule-value'>{t('channels.matchValue')}</label>
                    <input
                      id='rule-value'
                      value={draft.value ?? ''}
                      onChange={(event) => setDraft({ ...draft, value: event.target.value })}
                    />
                  </>
                )}
            <label htmlFor='rule-exit'>{t('channels.exit')}</label>
            <ChannelRouteExitSelect
              id='rule-exit'
              value={draft.exit}
              nodes={nodes}
              follow
              onChange={(exit) => setDraft({ ...draft, exit })}
            />
            <label htmlFor='rule-enabled'>{t('channels.enabled')}</label>
            <input
              id='rule-enabled'
              type='checkbox'
              checked={draft.enabled}
              onChange={(event) => setDraft({ ...draft, enabled: event.target.checked })}
            />
          </div>
          {error && (
            <p role='alert' className='subscription-form-error'>
              {error}
            </p>
          )}
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
