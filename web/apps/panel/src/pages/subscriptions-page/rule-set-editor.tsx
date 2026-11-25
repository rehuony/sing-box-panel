import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@shared/components/ui/button';
import { Textarea } from '@shared/components/ui/textarea';
import { toast } from '@shared/components/ui/toast-manager';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { CirclePlus, PencilLine, Save, Trash2 } from 'lucide-react';
import { Tabs, TabsList, TabsTrigger } from '@shared/components/ui/tabs';

import type { SubscriptionRule, SubscriptionRuleSet } from '@/api/api-client';

import { ApiRequestError } from '@/api/api-client';
import { messageLine } from '@/utils/message-line';
import { IconAction } from '@/components/icon-action';
import { StripAction } from '@/components/title-strip';
import { useApiClient } from '@/api/api-client-context';
import { useUnsavedChanges } from '@/hooks/use-unsaved-changes';
import { describeRequestError } from '@/components/error-notice';

import { RuleEditor } from './rule-editor';
import { SubscriptionContentState } from './subscription-content-state';
import { parseRules, ruleLine, ruleType, serializeRules } from './rule-text';

type Editing = { index: number; remote: boolean } | { index: null; remote: boolean };

/** A rule's value as a row shows it: a match value, or a remote rule set's name, link and format. */
function RuleValue({ rule }: { rule: SubscriptionRule }) {
  if (rule.kind !== 'remote' || !rule.remote) {
    return <span className='font-[family-name:var(--font-data)] text-compact break-all'>{rule.value}</span>;
  }
  const { behavior, format, name, url } = rule.remote;
  return (
    <span className='flex min-w-0 flex-col gap-0.5'>
      <span className='font-[family-name:var(--font-data)] text-compact break-all'>{name}</span>
      <span className='font-[family-name:var(--font-data)] text-xs break-all text-[var(--color-text-secondary)]'>
        {url}
        <span aria-hidden='true' className='mx-1.5 text-[var(--color-text-faint)]'>·</span>
        {[format, behavior].filter(Boolean).join(' · ')}
      </span>
    </span>
  );
}

/**
 * One rule set: its name over its client, rule count and the channels that
 * use it, with rename, delete and save; then its rules as entries or as
 * classic rule lines. Changes stay a draft until saved, and saving at a
 * stale version reads the set again and keeps the draft.
 */
export function RuleSetEditor({ onDelete, onRename, ruleSet }: {
  ruleSet: SubscriptionRuleSet;
  /** Asks for a new name; the editor's draft takes it. */
  onRename: (name: string, apply: (name: string) => void) => void;
  onDelete: (ruleSet: SubscriptionRuleSet) => void;
}) {
  const { t } = useTranslation();
  const client = useApiClient();
  const cache = useQueryClient();
  const [name, setName] = useState(ruleSet.name);
  const [rules, setRules] = useState<SubscriptionRule[]>(ruleSet.rules);
  const [text, setText] = useState<string | null>(null);
  const [editing, setEditing] = useState<Editing | null>(null);
  const [version, setVersion] = useState(ruleSet.updated_at);
  const parsed = useMemo(() => text === null ? null : parseRules(text, ruleSet.client), [ruleSet.client, text]);
  const current = parsed && parsed.errors.length === 0 ? parsed.rules : rules;
  const textInvalid = parsed !== null && parsed.errors.length > 0;
  const dirty = name !== ruleSet.name || JSON.stringify(current) !== JSON.stringify(ruleSet.rules) || textInvalid;

  function discard() {
    setName(ruleSet.name);
    setRules(ruleSet.rules);
    setText(null);
  }
  useUnsavedChanges(dirty, discard);

  const save = useMutation({
    mutationFn: () => client.updateSubscriptionRuleSet(ruleSet.id, { name, rules: current }, version),
    onSuccess: (saved) => {
      cache.setQueryData<SubscriptionRuleSet[]>(['ruleSets'], sets => sets?.map(set => set.id === saved.id ? saved : set));
      void cache.invalidateQueries({ queryKey: ['ruleSets'] });
      setText(null);
      toast.add({ title: t('subscriptions.ruleSets.saved'), type: 'success' });
    },
    onError: (error) => {
      if (error instanceof ApiRequestError && error.status === 412) {
        toast.add({ title: t('subscriptions.ruleSets.stale'), type: 'warning' });
        void cache.invalidateQueries({ queryKey: ['ruleSets'] });
        void client.listSubscriptionRuleSets(ruleSet.client)
          .then(list => setVersion(list.items.find(set => set.id === ruleSet.id)?.updated_at ?? version))
          .catch(() => undefined);
        return;
      }
      toast.add({ title: t('subscriptions.ruleSets.saveFailed'), description: describeRequestError(error), type: 'error' });
    },
  });

  function changeView(view: string) {
    if (view === 'text') {
      setText(serializeRules(rules));
    } else if (parsed && parsed.errors.length === 0) {
      setRules(parsed.rules);
      setText(null);
    }
  }

  const remoteNames = (except: number | null) =>
    rules.flatMap((rule, index) => index !== except && rule.remote ? [rule.remote.name] : []);
  const usedBy = ruleSet.channels.length > 0
    ? t('subscriptions.ruleSets.usedBy', { count: ruleSet.channels.length })
    : t('subscriptions.ruleSets.unused');
  const firstError = parsed?.errors[0];
  const status = firstError
    ? t('subscriptions.ruleSets.textError', {
        count: parsed.errors.length,
        line: firstError.line,
        reason: t(`subscriptions.ruleSets.textIssue.${firstError.code}`, { detail: firstError.detail ?? '' }),
      })
    : '';

  return (
    <section aria-label={name} className='flex min-h-0 min-w-0 flex-col overflow-hidden rounded-[var(--radius-card)] border bg-[var(--color-surface-translucent)]'>
      <header className='flex h-16 flex-none items-center gap-2 border-b ps-4 pe-3'>
        <div className='min-w-0 flex-1'>
          <h2 className='truncate text-base font-medium' title={name}>{name}</h2>
          <p className='truncate font-[family-name:var(--font-data)] text-xs text-[var(--color-text-secondary)]'>
            {[ruleSet.client, t('subscriptions.ruleSets.ruleCount', { count: current.length }), usedBy].join(' · ')}
          </p>
        </div>
        <StripAction icon={<PencilLine aria-hidden='true' data-icon='inline-start' />} label={t('subscriptions.ruleSets.rename')} onClick={() => onRename(name, setName)} />
        <StripAction icon={<Trash2 aria-hidden='true' data-icon='inline-start' />} label={t('subscriptions.ruleSets.delete')} onClick={() => onDelete(ruleSet)} />
        <Button
          aria-busy={save.isPending || undefined}
          disabled={!dirty || (parsed !== null && parsed.errors.length > 0)}
          onClick={() => !save.isPending && save.mutate()}
          size='sm'
          variant='flat'
        >
          <Save aria-hidden='true' data-icon='inline-start' />
          {t('subscriptions.ruleSets.save')}
        </Button>
      </header>
      <div className='flex h-12 flex-none items-center gap-1 border-b ps-3 pe-3'>
        <Button disabled={text !== null} onClick={() => setEditing({ index: null, remote: false })} size='sm' variant='flat'>
          <CirclePlus aria-hidden='true' data-icon='inline-start' />
          {t('subscriptions.ruleSets.addRule')}
        </Button>
        <Button disabled={text !== null} onClick={() => setEditing({ index: null, remote: true })} size='sm' variant='flat'>
          <CirclePlus aria-hidden='true' data-icon='inline-start' />
          {t('subscriptions.ruleSets.addRemote')}
        </Button>
        <span className='flex-1' />
        <Tabs className='flex-none gap-0' onValueChange={changeView} value={text === null ? 'entries' : 'text'}>
          <TabsList aria-label={t('subscriptions.ruleSets.view')} variant='pill'>
            <TabsTrigger value='entries'>{t('subscriptions.ruleSets.entries')}</TabsTrigger>
            <TabsTrigger value='text'>{t('subscriptions.ruleSets.text')}</TabsTrigger>
          </TabsList>
        </Tabs>
      </div>
      {text === null
        ? (
            rules.length === 0
              ? <SubscriptionContentState kind='rules' title={t('subscriptions.ruleSets.noRules')} />
              : (
                  <ol aria-label={t('subscriptions.ruleSets.rules')} className='min-h-0 flex-1 overflow-y-auto overscroll-contain'>
                    {rules.map((rule, index) => (
                      <li
                        className='group/rule grid min-h-11 grid-cols-[7rem_minmax(0,1fr)_3.5rem] items-center gap-3 border-b py-2.5 ps-4 pe-3'
                        // Rules repeat only by mistake; their position is their identity.
                        // eslint-disable-next-line react/no-array-index-key
                        key={index}
                        title={ruleLine(rule)}
                      >
                        <span className='font-[family-name:var(--font-data)] text-xs text-[var(--color-text-secondary)]'>{ruleType(rule.kind)}</span>
                        <RuleValue rule={rule} />
                        <span className='invisible flex justify-end gap-0.5 group-focus-within/rule:visible group-hover/rule:visible'>
                          <IconAction label={t('subscriptions.ruleSets.editRule')} onClick={() => setEditing({ index, remote: rule.kind === 'remote' })} size='icon-xs'>
                            <PencilLine aria-hidden='true' />
                          </IconAction>
                          <IconAction label={t('subscriptions.ruleSets.removeRule')} onClick={() => setRules(rules.filter((_, each) => each !== index))} size='icon-xs'>
                            <Trash2 aria-hidden='true' />
                          </IconAction>
                        </span>
                      </li>
                    ))}
                  </ol>
                )
          )
        : (
            <div className='flex min-h-0 flex-1 flex-col'>
              <Textarea
                aria-describedby={`${ruleSet.id}-text-status`}
                aria-invalid={Boolean(firstError) || undefined}
                aria-label={t('subscriptions.ruleSets.textLabel')}
                className='min-h-0 flex-1 resize-none rounded-none border-0 bg-transparent px-4 py-3 font-[family-name:var(--font-data)] text-xs leading-5 focus-visible:outline-none md:text-xs'
                onChange={event => setText(event.target.value)}
                spellCheck={false}
                value={text}
                wrap='off'
              />
              <p
                className='flex h-9 flex-none items-center truncate border-t px-4 text-sm text-[var(--color-danger-text)]'
                id={`${ruleSet.id}-text-status`}
                role='status'
                title={messageLine(status) || undefined}
              >
                {messageLine(status)}
              </p>
            </div>
          )}
      {editing
        ? (
            <RuleEditor
              client={ruleSet.client}
              onClose={() => setEditing(null)}
              onSave={(rule) => {
                const { index: at } = editing;
                setRules(at === null ? [...rules, rule] : rules.map((each, index) => index === at ? rule : each));
                setEditing(null);
              }}
              remote={editing.remote}
              rule={editing.index === null ? null : rules[editing.index] ?? null}
              takenNames={remoteNames(editing.index)}
            />
          )
        : null}
    </section>
  );
}
