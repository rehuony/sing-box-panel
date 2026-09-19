import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ArrowDown, ArrowUp, Pencil, Trash2 } from 'lucide-react';

import type {
  ChannelRule,
  ChannelRuleGroup,
  SubscriptionFormat,
  SubscriptionNodeSummary,
} from '@/api/api-client';

import { Button } from '@/components/ui/button';
import { describeRequestError } from '@/components/error-notice';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';

import { ruleFormats } from './channel-policy';
import { ChannelRuleEditor } from './channel-rule-editor';
import { ChannelRouteExitSelect } from './channel-route-exit';

interface Props {
  onClose: () => void;
  group: ChannelRuleGroup;
  format: SubscriptionFormat;
  nodes: SubscriptionNodeSummary[];
  onSave: (group: ChannelRuleGroup) => Promise<void>;
}
export function ChannelGroupEditor({ group, nodes, format, onClose, onSave }: Props) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState(() => structuredClone(group));
  const [tab, setTab] = useState<'nodes' | 'rules'>('nodes');
  const [rule, setRule] = useState<ChannelRule | null>(null);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const candidates = nodes.filter((node) => draft.node_ids.includes(node.id));
  async function save() {
    if (!draft.name.trim()) {
      setError(t('channels.nameRequired'));
      return;
    }
    setBusy(true);
    setError('');
    try {
      await onSave({ ...draft, name: draft.name.trim() });
      onClose();
    } catch (reason) {
      setError(describeRequestError(reason));
    } finally {
      setBusy(false);
    }
  }
  function move(index: number, delta: number) {
    const rules = [...draft.rules];
    [rules[index], rules[index + delta]] = [rules[index + delta], rules[index]];
    setDraft({ ...draft, rules });
  }
  function newRule(remote: boolean) {
    setRule({
      id: crypto.randomUUID(),
      enabled: true,
      kind: remote ? 'remote' : 'domain_suffix',
      exit: { kind: 'group-default' },
      ...(remote
        ? {
            remote: {
              name: '',
              url: '',
              format: ruleFormats(format)[0],
              behavior: format === 'mihomo' ? 'domain' : undefined,
              accelerated: false,
              update_interval: 86400,
            },
          }
        : { value: '' }),
    });
  }
  return (
    <Dialog open onOpenChange={(open) => !open && !busy && onClose()}>
      <DialogContent className='channel-group-dialog'>
        <DialogHeader>
          <DialogTitle>{t('channels.editGroup')}</DialogTitle>
          <DialogDescription className='sr-only'>{t('channels.groups')}</DialogDescription>
        </DialogHeader>
        <div className='channel-group-header'>
          <label htmlFor='group-name'>{t('channels.groupName')}</label>
          <input
            id='group-name'
            value={draft.name}
            onChange={(event) => setDraft({ ...draft, name: event.target.value })}
          />
          <label>
            <input
              type='checkbox'
              checked={draft.enabled}
              onChange={(event) => setDraft({ ...draft, enabled: event.target.checked })}
            />
            {t('channels.enabled')}
          </label>
        </div>
        <div className='channel-group-tabs' role='tablist' aria-label={t('channels.editGroup')}>
          <Button
            role='tab'
            aria-selected={tab === 'nodes'}
            variant={tab === 'nodes' ? 'secondary' : 'ghost'}
            onClick={() => setTab('nodes')}
          >
            {t('channels.candidates')}
          </Button>
          <Button
            role='tab'
            aria-selected={tab === 'rules'}
            variant={tab === 'rules' ? 'secondary' : 'ghost'}
            onClick={() => setTab('rules')}
          >
            {t('channels.matches')}
          </Button>
        </div>
        <div className='channel-dialog-scroll'>
          {tab === 'nodes'
            ? (
                <>
                  <div className='channel-selection-actions'>
                    <Button
                      variant='ghost'
                      onClick={() =>
                        setDraft({
                          ...draft,
                          node_ids: [
                            ...new Set([
                              ...draft.node_ids,
                              ...nodes.filter((node) => !node.hidden && node.available).map((node) => node.id),
                            ]),
                          ],
                        })
                      }
                    >
                      {t('channels.selectAll')}
                    </Button>
                    <Button variant='ghost' onClick={() => setDraft({ ...draft, node_ids: [] })}>
                      {t('channels.selectNone')}
                    </Button>
                  </div>
                  <div className='channel-candidates'>
                    {nodes
                      .filter((node) => !node.hidden && node.available)
                      .map((node) => (
                        <label key={node.id}>
                          <input
                            type='checkbox'
                            checked={draft.node_ids.includes(node.id)}
                            onChange={(event) =>
                              setDraft({
                                ...draft,
                                node_ids: event.target.checked
                                  ? [...draft.node_ids, node.id]
                                  : draft.node_ids.filter((id) => id !== node.id),
                              })
                            }
                          />
                          <span title={node.name}>{node.name}</span>
                          <small title={node.source_name}>{node.source_name}</small>
                        </label>
                      ))}
                  </div>
                  <div className='subscription-settings-fields'>
                    <label htmlFor='group-default'>{t('channels.defaultExit')}</label>
                    <ChannelRouteExitSelect
                      id='group-default'
                      value={draft.default_exit}
                      nodes={candidates}
                      onChange={(default_exit) => setDraft({ ...draft, default_exit })}
                    />
                  </div>
                </>
              )
            : (
                <>
                  <div className='channel-selection-actions'>
                    <Button variant='secondary' onClick={() => newRule(false)}>
                      {t('channels.addRule')}
                    </Button>
                    <Button variant='secondary' onClick={() => newRule(true)}>
                      {t('channels.addRemote')}
                    </Button>
                  </div>
                  <div className='channel-rule-rows'>
                    {draft.rules.map((item, index) => (
                      <div className='channel-rule-row' key={item.id}>
                        <input
                          aria-label={`${t('channels.enabled')} ${item.remote?.name ?? item.value}`}
                          type='checkbox'
                          checked={item.enabled}
                          onChange={(event) =>
                            setDraft({
                              ...draft,
                              rules: draft.rules.map((value) =>
                                value.id === item.id ? { ...value, enabled: event.target.checked } : value,
                              ),
                            })
                          }
                        />
                        <button
                          className='channel-rule-label'
                          title={item.remote?.url ?? item.value}
                          onClick={() => setRule(item)}
                        >
                          {item.remote?.name ?? item.value}
                          {item.remote && !ruleFormats(format).includes(item.remote!.format) && (
                            <small className='subscription-form-error'>{t('channels.formatPending')}</small>
                          )}
                        </button>
                        <div className='subscription-toolbar-actions'>
                          <Button
                            variant='ghost'
                            size='icon'
                            aria-label={t('channels.moveUp')}
                            disabled={index === 0}
                            onClick={() => move(index, -1)}
                          >
                            <ArrowUp />
                          </Button>
                          <Button
                            variant='ghost'
                            size='icon'
                            aria-label={t('channels.moveDown')}
                            disabled={index === draft.rules.length - 1}
                            onClick={() => move(index, 1)}
                          >
                            <ArrowDown />
                          </Button>
                          <Button
                            variant='ghost'
                            size='icon'
                            aria-label={t('channels.editRule')}
                            onClick={() => setRule(item)}
                          >
                            <Pencil />
                          </Button>
                          <Button
                            variant='ghost'
                            size='icon'
                            aria-label={t('channels.deleteRule')}
                            onClick={() =>
                              setDraft({ ...draft, rules: draft.rules.filter((value) => value.id !== item.id) })
                            }
                          >
                            <Trash2 />
                          </Button>
                        </div>
                      </div>
                    ))}
                  </div>
                </>
              )}
          {error && (
            <p role='alert' className='subscription-form-error'>
              {error}
            </p>
          )}
        </div>
        <DialogFooter>
          <Button variant='secondary' disabled={busy} onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button variant='secondary' disabled={busy} onClick={() => void save()}>
            {t('channels.done')}
          </Button>
        </DialogFooter>
        {rule && (
          <ChannelRuleEditor
            rule={rule}
            format={format}
            nodes={candidates}
            onClose={() => setRule(null)}
            onSave={async (value) => {
              setDraft((current) => ({
                ...current,
                rules: current.rules.some((existing) => existing.id === value.id)
                  ? current.rules.map((existing) => (existing.id === value.id ? value : existing))
                  : [...current.rules, value],
              }));
            }}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}
