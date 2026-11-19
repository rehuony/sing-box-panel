import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@shared/components/ui/button';
import { Textarea } from '@shared/components/ui/textarea';
import { toast } from '@shared/components/ui/toast-manager';
import { CirclePlus, Pencil, Search, Trash2 } from 'lucide-react';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@shared/components/ui/tabs';
import { InputGroup, InputGroupAddon, InputGroupInput } from '@shared/components/ui/input-group';

import type {
  ChannelRouteExit,
  ChannelRule,
  ChannelRuleGroup,
  SubscriptionFormat,
  SubscriptionNode,
} from '@/api/api-client';

import { ErrorNotice } from '@/components/error-notice';

import type { RuleTextResult } from './channel-rule-text';
import type { ChannelNodeCard } from './channel-node-cards';

import { nodeOrigin } from './subscription-node-pool';
import { ChannelNodeCards } from './channel-node-cards';
import { ChannelNodePicker } from './channel-node-picker';
import { ChannelRuleEditor } from './channel-rule-editor';
import { ChannelNodeActions } from './channel-node-actions';
import { reorderVisibleNodes } from './subscription-node-order';
import { SubscriptionContentState } from './subscription-content-state';
import { compareChannelRules, memberKey, ruleFormats, updateGroupMembers } from './channel-policy';

export type ChannelGroupTab = 'nodes' | 'rules';

interface Props {
  busy: boolean;
  tab: ChannelGroupTab;
  nextSortIndex: number;
  group: ChannelRuleGroup;
  /** The pool in its saved order. */
  nodes: SubscriptionNode[];
  format: SubscriptionFormat;
  groups: ChannelRuleGroup[];
  onRuleTextChange: (text: string) => void;
  onChange: (group: ChannelRuleGroup) => void;
  onTabChange: (tab: ChannelGroupTab) => void;
  onRuleView: (view: 'entries' | 'text') => void;
  /** The group's rules as text and what they parse to, while shown as text. */
  ruleText?: { text: string; result: RuleTextResult };
}
export function ChannelGroupEditor({
  group, groups, nodes, format, busy, tab, onTabChange, onChange, nextSortIndex, ruleText, onRuleView, onRuleTextChange,
}: Props) {
  const { t } = useTranslation();
  const [addingNodes, setAddingNodes] = useState(false);
  const [rule, setRule] = useState<ChannelRule | null>(null);
  const [search, setSearch] = useState('');
  const [selected, setSelected] = useState<string[]>([]);
  const matches = (value: string) => value.toLowerCase().includes(search.trim().toLowerCase());
  const order = group.members.map(memberKey);
  const selectedIDs = new Set(order.filter((id) => selected.includes(id)));
  const groupCatalog = new Map(groups.map(value => [value.id, value]));
  const catalog = new Map(nodes.map((node) => [node.id, node]));
  const cards: ChannelNodeCard[] = group.members.flatMap((member) => {
    const id = memberKey(member);
    const builtin = member.kind === 'builtin' ? member.id as 'direct' | 'reject' : undefined;
    const node = member.kind === 'node' ? catalog.get(member.id) : undefined;
    const candidate = member.kind === 'group' ? groupCatalog.get(member.id) : undefined;
    let label = node?.display_name ?? candidate?.name ?? t('channels.missingNode');
    if (builtin) label = builtin.toUpperCase();
    let searched = `${label} ${node ? `${nodeOrigin(node)} ${node.protocol}` : member.id}`;
    if (builtin) searched = `${label} ${t(`channels.${builtin}`)}`;
    if (candidate) searched = `${label} ${t('channels.strategyGroup')} ${t(`channels.groupTypes.${candidate.type}`)}`;
    if (!matches(searched)) return [];
    let unavailable = !node;
    if (builtin) unavailable = (format === 'sing-box' && builtin === 'reject') || (format === 'loon' && group.type !== 'select');
    if (member.kind === 'group') unavailable = !candidate?.enabled;
    return [{ id, label, node, builtin, group: candidate, selected: selectedIDs.has(id), disabled: busy, unavailable }];
  });

  const textResult = ruleText?.result;
  function changeRuleView(view: unknown) {
    if (view === 'text') onRuleView('text');
    if (view !== 'entries' || !textResult) return;
    if (textResult.errors.length) {
      toast.add({ title: t('channels.ruleTextBlocked'), type: 'error' });
      return;
    }
    onRuleView('entries');
  }

  function exitLabel(exit: ChannelRouteExit) {
    if (exit.kind === 'node') return nodes.find((node) => node.id === exit.id)?.display_name ?? t('channels.missingNode');
    return t(exit.kind === 'group-default' ? 'channels.follow' : exit.kind === 'reject' ? 'channels.reject' : 'channels.direct');
  }
  function newRule(remote: boolean) {
    setRule({
      id: crypto.randomUUID(),
      enabled: true,
      sort_index: nextSortIndex,
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
            },
          }
        : { value: '' }),
    });
  }
  return (
    <section
      // Rule text takes the height left below the editor, also when it stacks.
      className='flex min-h-0 min-w-0 flex-col overflow-hidden @max-[740px]/channel-editor:overflow-visible @max-[740px]/channel-editor:data-[rule-text]:flex-[1_0_auto]'
      data-rule-text={ruleText ? '' : undefined}
      aria-label={t('channels.editGroup')}
    >
      {addingNodes && (
        <ChannelNodePicker
          nodes={nodes}
          group={group}
          groups={groups}
          format={format}
          onClose={() => setAddingNodes(false)}
          onAdd={(members) => {
            const present = new Set(order);
            const added = members.filter(member => !present.has(memberKey(member)));
            onChange(updateGroupMembers(group, [...group.members, ...added]));
            setAddingNodes(false);
          }} />
      )}
      <fieldset className='flex min-h-0 min-w-0 flex-1 flex-col @max-[740px]/channel-editor:h-auto' disabled={busy}>
        <Tabs value={tab} onValueChange={(value) => {
          if (value === 'nodes' || value === 'rules') onTabChange(value);
        }} className='min-h-0 min-w-0 flex-1 gap-0'>
          <div className='mb-5 flex min-w-0 flex-none flex-wrap items-center gap-2 pe-1 has-[[data-rule-tools]]:mb-2 *:data-[slot=tabs-list]:flex-none'>
            <TabsList className='max-w-full flex-none justify-start self-start overflow-x-auto [scrollbar-width:none] *:data-[slot=tabs-trigger]:flex-none' aria-label={t('channels.editGroup')}>
              <TabsTrigger value='nodes'>{t('channels.candidates')}</TabsTrigger>
              <TabsTrigger value='rules'>{t('channels.matches')}</TabsTrigger>
            </TabsList>
            {tab === 'rules' && (
              <div className='flex min-w-0 flex-none flex-wrap items-center gap-2 ms-auto' data-rule-tools>
                {ruleText === undefined && (
                  <>
                    <Button size='sm' variant='ghost' onClick={() => newRule(false)}>
                      <CirclePlus data-icon='inline-start' />
                      {t('channels.addRule')}
                    </Button>
                    <Button size='sm' variant='ghost' onClick={() => newRule(true)}>
                      <CirclePlus data-icon='inline-start' />
                      {t('channels.addRemote')}
                    </Button>
                  </>
                )}
                <Tabs className='flex-none gap-0' value={ruleText === undefined ? 'entries' : 'text'} onValueChange={changeRuleView}>
                  <TabsList aria-label={t('channels.ruleView')}>
                    <TabsTrigger value='entries'>{t('channels.ruleEntries')}</TabsTrigger>
                    <TabsTrigger value='text'>{t('channels.ruleText')}</TabsTrigger>
                  </TabsList>
                </Tabs>
              </div>
            )}
            {tab === 'nodes' && (
              <div className='ms-auto flex min-w-0 flex-wrap items-center gap-2 max-[720px]:flex-[1_1_100%]'>
                <ChannelNodeActions
                  count={selectedIDs.size}
                  disabled={busy}
                  canSelectAll={cards.some((card) => !card.selected)}
                  onClear={() => setSelected([])}
                  onSelectAll={() => setSelected((current) => [...new Set([
                    ...current, ...cards.map((card) => card.id),
                  ])])}
                  onRemove={() => {
                    const kept = group.members.filter(member => !selectedIDs.has(memberKey(member)));
                    onChange(updateGroupMembers(group, kept));
                    setSelected([]);
                  }}
                  onAdd={() => setAddingNodes(true)}
                />
                <InputGroup className='h-(--control-height-sm) min-w-30 w-60 max-[900px]:w-50 max-[720px]:flex-1'>
                  <InputGroupAddon><Search aria-hidden='true' /></InputGroupAddon>
                  <InputGroupInput className='h-full min-h-0' aria-label={t('channels.searchNodes')} placeholder={t('channels.searchNodes')} value={search} onChange={(event) => setSearch(event.target.value)} />
                </InputGroup>
              </div>
            )}
          </div>
          <TabsContent value='nodes' className='flex min-h-0 flex-col overflow-y-auto overscroll-contain p-0.5 @max-[740px]/channel-editor:overflow-visible'>
            <ChannelNodeCards
              items={cards}
              onToggle={(id) => setSelected((current) =>
                current.includes(id) ? current.filter((value) => value !== id) : [...current, id])}
              onMove={(active, over) => {
                const byKey = new Map(group.members.map(member => [memberKey(member), member]));
                const next = reorderVisibleNodes(order, cards.map((card) => card.id), active, over);
                onChange({ ...group, members: next.map(key => byKey.get(key)!) });
              }}
            />
            {!cards.length && (
              <SubscriptionContentState kind={search ? 'search' : 'candidates'} title={t(search ? 'channels.noMatchingNodes' : 'channels.emptyCandidates')} />
            )}
          </TabsContent>
          <TabsContent value='rules' className={`flex min-h-0 flex-col overflow-y-auto overscroll-contain p-0.5 @max-[740px]/channel-editor:overflow-visible ${ruleText ? 'flex-1' : ''}`}>
            {ruleText && textResult && (
              <div className='flex min-h-0 flex-1 flex-col gap-2'>
                <Textarea
                  aria-label={t('channels.ruleTextLabel')}
                  aria-invalid={textResult.errors.length > 0 || undefined}
                  aria-describedby={textResult.errors.length || textResult.notes.length ? 'rule-text-issues' : undefined}
                  className='min-h-60 flex-1 resize-none overflow-auto font-[family-name:var(--font-data)] whitespace-pre field-sizing-fixed'
                  placeholder='DOMAIN-SUFFIX,example.com'
                  spellCheck={false} autoCapitalize='off' autoCorrect='off' autoComplete='off' wrap='off'
                  value={ruleText.text}
                  onChange={(event) => onRuleTextChange(event.target.value)}
                />
                {(textResult.errors.length > 0 || textResult.notes.length > 0) && (
                  <ul id='rule-text-issues' className='m-0 grid max-h-32 flex-none list-none gap-1 overflow-y-auto overscroll-contain p-0 text-sm' aria-label={t('channels.ruleTextIssues')}>
                    {[
                      ...textResult.errors.map(issue => ({ ...issue, error: true })),
                      ...textResult.notes.map(issue => ({ ...issue, error: false })),
                    ].sort((left, right) => left.line - right.line).map(issue => (
                      <li className={`flex min-w-0 gap-2 ${issue.error ? 'text-[var(--color-danger-text)]' : 'text-muted-foreground'}`} key={`${issue.line}-${issue.code}-${issue.detail ?? ''}`}>
                        <span className='flex-none font-[family-name:var(--font-data)] tabular-nums'>{t('channels.ruleTextLine', { line: issue.line })}</span>
                        <span className='min-w-0 truncate' title={t(`channels.ruleTextIssue.${issue.code}`, { detail: issue.detail ?? '' })}>
                          {t(`channels.ruleTextIssue.${issue.code}`, { detail: issue.detail ?? '' })}
                        </span>
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            )}
            {ruleText === undefined && (
              <>
                <ul className='m-0 grid list-none p-0' aria-label={t('channels.matches')}>
                  {[...group.rules].sort(compareChannelRules).map((item) => (
                    <li className='grid grid-cols-[minmax(0,1fr)_auto] items-center gap-3 py-3.5 [&+&]:border-t' key={item.id} data-disabled={!item.enabled || undefined}>
                      <div className='flex min-w-0 items-center gap-3 text-left' title={item.remote?.url ?? item.value}>
                        <span className='max-w-1/2 min-w-0 flex-none truncate font-medium in-data-[disabled=true]:text-muted-foreground' title={item.remote?.name ?? item.value}>{item.remote?.name ?? item.value}</span>
                        <span className='min-w-0 truncate text-xs text-muted-foreground [&>span+span]:ms-3 [&>span+span]:border-s [&>span+span]:ps-3' title={[
                          `${t('channels.sortIndex')}: ${item.sort_index ?? 0}`,
                          t(item.kind === 'remote' ? 'channels.ruleSet' : `channels.${item.kind}`),
                          exitLabel(item.exit),
                          ...(!item.enabled ? [t('channels.disabled')] : []),
                        ].join(' · ')}>
                          <span>
                            {t('channels.sortIndex')}
                            :
                            {' '}
                            {item.sort_index ?? 0}
                          </span>
                          <span>{t(item.kind === 'remote' ? 'channels.ruleSet' : `channels.${item.kind}`)}</span>
                          <span>{exitLabel(item.exit)}</span>
                          {!item.enabled && <span>{t('channels.disabled')}</span>}
                        </span>
                        {item.remote && !ruleFormats(format).includes(item.remote.format) && (
                          <ErrorNotice error={t('channels.formatPending')} />
                        )}
                      </div>
                      <div className='flex min-w-0 flex-none items-center gap-2'>
                        <Button variant='ghost' size='icon-sm' aria-label={t('channels.editRule')} title={t('channels.editRule')} onClick={() => setRule(item)}><Pencil /></Button>
                        <Button variant='ghost' size='icon-sm' className='text-[var(--color-danger)]' aria-label={t('channels.deleteRule')} title={t('channels.deleteRule')} onClick={() => onChange({ ...group, rules: group.rules.filter((value) => value.id !== item.id) })}><Trash2 /></Button>
                      </div>
                    </li>
                  ))}
                </ul>
                {!group.rules.length && (
                  <SubscriptionContentState kind='rules' title={t('channels.matches')} />
                )}
              </>
            )}
          </TabsContent>
        </Tabs>
      </fieldset>
      {rule && (
        <ChannelRuleEditor
          rule={rule}
          otherRuleNames={groups.flatMap(value => value.rules
            .filter(item => item.remote && !(value.id === group.id && item.id === rule.id))
            .map(item => item.remote!.name.trim()))}
          format={format}
          onClose={() => setRule(null)}
          onSave={async (value) => onChange({
            ...group,
            rules: group.rules.some((existing) => existing.id === value.id)
              ? group.rules.map((existing) => existing.id === value.id ? value : existing)
              : [...group.rules, value],
          })}
        />
      )}
    </section>
  );
}
