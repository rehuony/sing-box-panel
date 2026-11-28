import type { CSSProperties } from 'react';

import { cn } from '@shared/lib/utils';
import { useTranslation } from 'react-i18next';
import { Button } from '@shared/components/ui/button';
import { PencilLine, Save, Undo2 } from 'lucide-react';
import { toast } from '@shared/components/ui/toast-manager';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { Tabs, TabsList, TabsTrigger } from '@shared/components/ui/tabs';
import { useCallback, useDeferredValue, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';

import type { SubscriptionRule, SubscriptionRuleSet } from '@/api/api-client';

import { ApiRequestError } from '@/api/api-client';
import { messageLine } from '@/utils/message-line';
import { IconAction } from '@/components/icon-action';
import { useApiClient } from '@/api/api-client-context';
import { StripDivider } from '@/components/title-strip';
import { useUnsavedChanges } from '@/hooks/use-unsaved-changes';
import { describeRequestError } from '@/components/error-notice';

import type { Entry, MatchKind, Row } from './rule-lines';
import type { RuleTextHandle, RuleTextMarks } from './rule-text-view';
import type { RuleTextErrorCode, RuleTextIssue, RuleTextNoteCode, RuleTextResult } from './rule-text';

import { RuleEditor } from './rule-editor';
import { RuleTextView } from './rule-text-view';
import { RuleSetIconMenu } from './rule-set-icon-menu';
import { gutterCell, gutterWidth } from './rule-line-style';
import { RuleEditLine, RuleEntry, RuleList } from './rule-lines';
import { SubscriptionContentState } from './subscription-content-state';
import { parseRules, ruleLine, ruleName, ruleType, serializeRules, withRuleType } from './rule-text';

/** What the rail shows of the set being edited. */
export interface RuleSetDraft {
  id: string;
  icon: string;
  name: string;
  dirty: boolean;
  rules: readonly SubscriptionRule[];
}

// One toast says what the editor refused or did, each replacing the last.
const NOTICE = 'rule-set-notice';
// The name's face, at rest and while edited; inputs keep 16px on phones.
const nameFace = 'text-base leading-5 font-medium md:text-sm';
const NO_KEYS: ReadonlySet<number> = new Set();

/** The saved rules as the draft's first rows. */
function savedRows(saved: readonly SubscriptionRule[]): Row[] {
  return saved.map((rule, index) => ({ key: index, origin: index, rule }));
}

/** Rows for rules, each matched to an unused saved rule with the same line. */
function matchRows(
  rules: readonly SubscriptionRule[],
  saved: readonly SubscriptionRule[],
  key: (index: number) => number,
): Row[] {
  const free = new Map<string, number[]>();
  saved.forEach((rule, index) => free.set(ruleLine(rule), [...free.get(ruleLine(rule)) ?? [], index]));
  return rules.map((rule, index) => ({ key: key(index), origin: free.get(ruleLine(rule))?.shift() ?? null, rule }));
}

function changed(row: Row, saved: readonly SubscriptionRule[]): boolean {
  return row.origin === null || ruleLine(row.rule) !== ruleLine(saved[row.origin]!);
}

/** Rules added or changed, and saved rules no row holds any more. */
function countChanges(rows: readonly Row[], saved: readonly SubscriptionRule[]): number {
  const kept = new Set(rows.flatMap(row => row.origin === null ? [] : [row.origin]));
  return rows.filter(row => changed(row, saved)).length + saved.length - kept.size;
}

/** What makes two rules the same: a remote set's name, or a match's kind and value in any case. */
function identity(rule: SubscriptionRule): string {
  return rule.kind === 'remote' ? `remote:${rule.remote?.name ?? ''}` : `${rule.kind}:${rule.value?.toLowerCase() ?? ''}`;
}

function duplicateOf(rule: SubscriptionRule, rows: readonly Row[]): Row | undefined {
  const id = identity(rule);
  return rows.find(row => identity(row.rule) === id);
}

/**
 * One rule set: a toolbar with its name, the entries or text switch, what
 * is unsaved and saving; then its rules as numbered lines over a prompt
 * line that adds them, or as classic rule lines over the same numbers.
 * What it refuses or does goes to one toast; saving or leaving the text with
 * a problem focuses it and says why. Changes stay a draft until saved, and
 * saving at a stale version reads the set again and keeps the draft.
 */
export function RuleSetEditor({ focusEntry, onDraftChange, onEntryFocused, query, ruleSet, takenNames }: {
  ruleSet: SubscriptionRuleSet;
  /** Filters the rules shown, in lower case; line numbers keep counting the whole list. */
  query: string;
  /** Names the client's other sets use, compared ignoring case. */
  takenNames: readonly string[];
  /** Puts the focus on the prompt line as the editor opens, as for a set just created. */
  focusEntry: boolean;
  onEntryFocused: () => void;
  /** Reports the draft to the rail, and null once the editor closes. */
  onDraftChange: (draft: RuleSetDraft | null) => void;
}) {
  const { t } = useTranslation();
  const client = useApiClient();
  const cache = useQueryClient();
  const saved = ruleSet.rules;
  const nextKeyRef = useRef(saved.length);
  const nameRef = useRef<HTMLInputElement>(null);
  const textViewRef = useRef<RuleTextHandle>(null);
  const [name, setName] = useState(ruleSet.name);
  const [icon, setIcon] = useState(ruleSet.icon);
  const [naming, setNaming] = useState(false);
  const [rows, setRows] = useState<Row[]>(() => savedRows(saved));
  const [text, setText] = useState<string | null>(null);
  const [version, setVersion] = useState(ruleSet.updated_at);
  // The line whose last input was refused, its prompt in danger until it changes.
  const [refused, setRefused] = useState<'entry' | 'row' | null>(null);
  const [editingKey, setEditingKey] = useState<number | null>(null);
  const [remote, setRemote] = useState<{ key: number | null } | null>(null);
  const [flash, setFlash] = useState<number | null>(null);
  const [follow, setFollow] = useState(0);
  // Rules added while searching stay shown until the search changes.
  const [pinned, setPinned] = useState<{ query: string; keys: ReadonlySet<number> }>({ query, keys: NO_KEYS });
  const pinnedKeys = pinned.query === query ? pinned.keys : NO_KEYS;

  // Each keystroke shows at once; reading the text, its marks and what changed follow when the editor is idle.
  // Saving and leaving the text read the text as it is.
  const deferredText = useDeferredValue(text);
  const source = text === null ? null : deferredText ?? text;
  const parsed = useMemo(() => source === null ? null : parseRules(source, ruleSet.client), [ruleSet.client, source]);
  const parseNow = (): RuleTextResult | null =>
    text === null || (source === text && parsed) ? parsed : parseRules(text, ruleSet.client);
  const textRows = useMemo(() => parsed ? matchRows(parsed.rules, saved, index => index) : null, [parsed, saved]);
  const current = textRows ?? rows;
  const rules = useMemo(() => current.map(row => row.rule), [current]);
  const textInvalid = (parsed?.errors.length ?? 0) > 0;
  const trimmed = name.trim();
  let nameIssue = '';
  if (!trimmed) {
    nameIssue = t('subscriptions.ruleSets.validation.setName');
  } else if (takenNames.some(each => each.toLowerCase() === trimmed.toLowerCase())) {
    nameIssue = t('subscriptions.ruleSets.validation.setNameTaken', { name: trimmed });
  }
  const changes = useMemo(() => countChanges(current, saved), [current, saved])
    + (trimmed === ruleSet.name ? 0 : 1) + (icon === ruleSet.icon ? 0 : 1);
  const dirty = changes > 0 || textInvalid;

  function discard() {
    setName(ruleSet.name);
    setIcon(ruleSet.icon);
    setNaming(false);
    setRows(savedRows(saved));
    setText(null);
    setEditingKey(null);
    setRefused(null);
  }
  useUnsavedChanges(dirty, discard);

  useEffect(() => {
    onDraftChange({ id: ruleSet.id, name: trimmed || ruleSet.name, icon, rules, dirty });
  }, [dirty, icon, onDraftChange, ruleSet.id, ruleSet.name, rules, trimmed]);
  useEffect(() => () => onDraftChange(null), [onDraftChange]);

  const save = useMutation({
    mutationFn: (next: SubscriptionRule[]) =>
      client.updateSubscriptionRuleSet(ruleSet.id, { name: trimmed, icon, rules: next }, version),
    onSuccess: (updated) => {
      cache.setQueryData<SubscriptionRuleSet[]>(['ruleSets'], sets => sets?.map(set => set.id === updated.id ? updated : set));
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

  const issueText = (issue: RuleTextIssue<RuleTextErrorCode | RuleTextNoteCode>) =>
    t(`subscriptions.ruleSets.textIssue.${issue.code}`, { detail: issue.detail ?? '' });
  const lineIssue = (issue: RuleTextIssue<RuleTextErrorCode | RuleTextNoteCode>, count: number) => {
    const line = t('subscriptions.ruleSets.textError', { line: issue.line, reason: issueText(issue) });
    return count > 1 ? t('subscriptions.ruleSets.issueCount', { issue: line, count }) : line;
  };
  const duplicateText = (rule: SubscriptionRule) => rule.kind === 'remote'
    ? t('subscriptions.ruleSets.validation.nameTaken', { name: rule.remote?.name ?? '' })
    : t('subscriptions.ruleSets.duplicate', { rule: ruleName(rule) });
  const notify = (type: 'error' | 'info' | 'success' | 'warning', title: string, description?: string) => toast.add({
    id: NOTICE,
    title: messageLine(title),
    description: description ? messageLine(description) : undefined,
    type,
  });
  function refuse(line: 'entry' | 'row', type: 'error' | 'warning', reason: string) {
    setRefused(line);
    notify(type, reason);
  }
  function quiet() {
    setRefused(null);
    toast.close(NOTICE);
  }

  // A refused duplicate highlights the rule it repeats for a moment.
  useEffect(() => {
    if (flash === null) return;
    const timer = window.setTimeout(setFlash, 1200, null);
    return () => window.clearTimeout(timer);
  }, [flash]);

  /** Appends rules, each taking a saved rule with the same line that no row holds; the list follows them. */
  function append(added: readonly SubscriptionRule[]) {
    if (added.length === 0) return;
    const held = new Set(rows.flatMap(row => row.origin === null ? [] : [row.origin]));
    const next = added.map((rule) => {
      const origin = saved.findIndex((each, index) => !held.has(index) && ruleLine(each) === ruleLine(rule));
      if (origin >= 0) held.add(origin);
      return { key: nextKeyRef.current++, origin: origin >= 0 ? origin : null, rule };
    });
    if (query) setPinned({ query, keys: new Set([...pinnedKeys, ...next.map(row => row.key)]) });
    setRows([...rows, ...next]);
    setFollow(count => count + 1);
  }

  /** Adds what the prompt line holds: one rule, or every line of a pasted block that can be read. */
  function submitEntry(input: string, kind: MatchKind): boolean {
    const lines = input.split(/\r?\n/).filter(each => each.trim() !== '');
    if (lines.length === 0) {
      refuse('entry', 'error', t('subscriptions.ruleSets.validation.value'));
      return false;
    }
    const result = parseRules(lines.map(each => withRuleType(each, kind)).join('\n'), ruleSet.client);
    if (lines.length === 1) {
      const error = result.errors[0];
      const rule = result.rules[0];
      if (error || !rule) {
        refuse('entry', 'error', error ? issueText(error) : t('subscriptions.ruleSets.validation.value'));
        return false;
      }
      const duplicate = duplicateOf(rule, rows);
      if (duplicate) {
        refuse('entry', 'warning', duplicateText(rule));
        setFlash(duplicate.key);
        return false;
      }
      append([rule]);
      quiet();
      const note = result.notes[0];
      if (note) notify('info', issueText(note));
      return true;
    }
    const held = new Set(rows.map(row => identity(row.rule)));
    const added: SubscriptionRule[] = [];
    for (const rule of result.rules) {
      if (held.has(identity(rule))) continue;
      added.push(rule);
      held.add(identity(rule));
    }
    append(added);
    setRefused(null);
    const parts: string[] = [];
    const skipped = result.rules.length - added.length;
    if (skipped > 0) parts.push(t('subscriptions.ruleSets.pasteSkipped', { count: skipped }));
    if (result.notes.length > 0) parts.push(t('subscriptions.ruleSets.pasteIgnored', { count: result.notes.length }));
    const error = result.errors[0];
    if (error) parts.push(lineIssue(error, result.errors.length));
    notify(error ? 'warning' : 'success', t('subscriptions.ruleSets.pasteAdded', { count: added.length }), parts.join(' · '));
    return true;
  }

  function startEdit(key: number) {
    const row = rows.find(each => each.key === key);
    if (!row || editingKey !== null) return;
    if (row.rule.kind === 'remote') {
      setRemote({ key });
      return;
    }
    setEditingKey(key);
    setRefused(null);
  }

  /** Applies the line being edited; one that cannot be read stays open. */
  function applyEdit(kind: MatchKind, input: string): boolean {
    const row = rows.find(each => each.key === editingKey);
    const value = input.trim();
    if (!row) return true;
    if (row.rule.kind === kind && row.rule.value === value) {
      setEditingKey(null);
      quiet();
      return true;
    }
    const result = parseRules(`${ruleType(kind)},${value}`, ruleSet.client);
    const error = result.errors[0];
    const rule = result.rules[0];
    if (error || !rule || result.notes.length > 0) {
      refuse('row', 'error', error ? issueText(error) : t('subscriptions.ruleSets.validation.value'));
      return false;
    }
    const duplicate = duplicateOf(rule, rows.filter(each => each.key !== row.key));
    if (duplicate) {
      refuse('row', 'warning', duplicateText(rule));
      setFlash(duplicate.key);
      return false;
    }
    setRows(rows.map(each => each.key === row.key ? { ...each, rule } : each));
    setEditingKey(null);
    quiet();
    return true;
  }

  /** Removes a rule from the draft and names it in a toast; reverting the draft brings it back. */
  function removeRow(key: number) {
    const index = rows.findIndex(row => row.key === key);
    const row = rows[index];
    if (!row) return;
    setRows(rows.filter(each => each.key !== key));
    if (editingKey === key) setEditingKey(null);
    setRefused(null);
    toast.add({ id: NOTICE, title: t('subscriptions.ruleSets.removed', { rule: ruleName(row.rule) }), type: 'info' });
  }

  // The list's lines are memoized, so they get callbacks that stay the same and reach the latest draft.
  const handlersRef = useRef({ edit: startEdit, remove: removeRow });
  useLayoutEffect(() => {
    handlersRef.current = { edit: startEdit, remove: removeRow };
  });
  const onEdit = useCallback((key: number) => handlersRef.current.edit(key), []);
  const onRemove = useCallback((key: number) => handlersRef.current.remove(key), []);
  const isChanged = useCallback((row: Row) => changed(row, saved), [saved]);

  useEffect(() => {
    if (!naming) return;
    const input = nameRef.current;
    input?.focus();
    input?.setSelectionRange(input.value.length, input.value.length);
  }, [naming]);

  /** Says why the text cannot be used yet and puts the caret on its first unreadable line; false when it can. */
  function blockedByText(now: RuleTextResult | null): boolean {
    const error = now?.errors[0];
    if (!now || !error) return false;
    notify('error', lineIssue(error, now.errors.length));
    textViewRef.current?.revealLine(error.line);
    return true;
  }

  /** Says why the name cannot be saved and focuses it; false when it can. */
  function blockedByName(): boolean {
    if (!nameIssue) return false;
    notify('error', nameIssue);
    if (naming) nameRef.current?.focus();
    else setNaming(true);
    return true;
  }

  function changeView(view: string) {
    if (view === 'text' && text === null) {
      setText(serializeRules(rows.map(row => row.rule)));
      setEditingKey(null);
      quiet();
    } else if (view === 'entries' && text !== null) {
      const now = parseNow();
      if (!now || blockedByText(now)) return;
      // Text left as it was keeps the rows, and what changed in them.
      if (text !== serializeRules(rows.map(row => row.rule))) {
        setRows(matchRows(now.rules, saved, () => nextKeyRef.current++));
      }
      setText(null);
      const note = now.notes[0];
      if (note) notify('info', lineIssue(note, now.notes.length));
    }
  }

  const shown = useMemo<Entry[]>(() => {
    const matches = (row: Row) =>
      !query || pinnedKeys.has(row.key) || row.key === editingKey || ruleLine(row.rule).toLowerCase().includes(query);
    return rows.flatMap((row, index) => matches(row) ? [{ position: index + 1, row }] : []);
  }, [editingKey, pinnedKeys, query, rows]);
  const showNameInput = naming || name !== ruleSet.name;
  const lineCount = useMemo(() => text === null ? rows.length : text.split('\n').length, [rows.length, text]);
  const marks = useMemo<RuleTextMarks | null>(() => {
    if (!parsed || !textRows) return null;
    // A marked number says on hover why: the line's first problem, or what was left out of it.
    const issues = new Map<number, RuleTextIssue<RuleTextErrorCode | RuleTextNoteCode>>();
    for (const issue of [...parsed.errors, ...parsed.notes]) {
      if (!issues.has(issue.line)) issues.set(issue.line, issue);
    }
    return {
      changed: new Set(textRows.flatMap((row, index) => changed(row, saved) ? [parsed.lines[index]!] : [])),
      errors: new Set(parsed.errors.map(issue => issue.line)),
      issues,
      notes: new Set(parsed.notes.map(issue => issue.line)),
    };
  }, [parsed, saved, textRows]);

  let body;
  if (text !== null && marks) {
    body = (
      <RuleTextView
        describe={issue => messageLine(issueText(issue))}
        invalid={textInvalid}
        label={t('subscriptions.ruleSets.textLabel')}
        lineCount={lineCount}
        marks={marks}
        onChange={setText}
        ref={textViewRef}
        text={text}
      />
    );
  } else {
    const editingRow = editingKey === null ? undefined : rows.find(row => row.key === editingKey);
    let list;
    if (rows.length === 0) {
      list = <SubscriptionContentState kind='rules' title={t('subscriptions.ruleSets.noRules')} />;
    } else if (shown.length === 0) {
      list = <SubscriptionContentState kind='search' title={t('subscriptions.ruleSets.noMatches')} />;
    } else {
      list = (
        <RuleList
          editingKey={editingKey}
          editLine={editingRow
            ? (
                <RuleEditLine
                  onApply={applyEdit}
                  onCancel={() => {
                    setEditingKey(null);
                    quiet();
                  }}
                  onInput={() => refused === 'row' && quiet()}
                  onRemove={() => removeRow(editingRow.key)}
                  position={rows.indexOf(editingRow) + 1}
                  refused={refused === 'row'}
                  row={editingRow}
                />
              )
            : null}
          entries={shown}
          flashKey={flash}
          followSignal={follow}
          isChanged={isChanged}
          label={t('subscriptions.ruleSets.rules')}
          onEdit={onEdit}
          onRemove={onRemove}
        />
      );
    }
    body = (
      <>
        {list}
        <RuleEntry
          autoFocus={focusEntry}
          onAddRemote={() => setRemote({ key: null })}
          onFocused={onEntryFocused}
          onInput={() => refused === 'entry' && quiet()}
          onSubmit={submitEntry}
          refused={refused === 'entry'}
        />
      </>
    );
  }

  return (
    <section
      aria-label={trimmed || ruleSet.name}
      className='@container/editor flex min-h-0 min-w-0 flex-col overflow-hidden rounded-[var(--radius-card)] border bg-[var(--color-surface-translucent)]'
      style={{ '--rule-gutter': gutterWidth(lineCount) } as CSSProperties}
    >
      <div className='flex h-12 flex-none items-center gap-2 border-b px-4'>
        {/* The name is a prompt line in the gutter: faint at rest, lit while edited, in danger while refused. */}
        <span
          aria-hidden='true'
          className={cn(
            gutterCell,
            'me-1 w-(--rule-gutter) flex-none text-compact',
            nameIssue && 'text-[var(--color-danger-text)]',
            !nameIssue && (showNameInput ? 'text-[var(--color-highlight-yellow)]' : 'text-[var(--color-text-faint)]'),
          )}
        >
          λ
        </span>
        {/* The set's icon, changed like its name: in the draft until saved. */}
        <RuleSetIconMenu onChange={setIcon} value={icon} variant='toolbar' />
        <div className='flex min-w-0 flex-1 items-center gap-1'>
          <h2 className='flex min-w-0'>
            {showNameInput
              ? (
                  // The field takes the name's width, so the edit action after it stays where it was.
                  <span className='inline-grid max-w-full'>
                    <span aria-hidden='true' className={cn(nameFace, 'invisible col-start-1 row-start-1 min-w-0 overflow-hidden whitespace-pre')}>{name || ' '}</span>
                    <input
                      aria-invalid={nameIssue ? true : undefined}
                      aria-label={t('subscriptions.ruleSets.field.setName')}
                      autoComplete='off'
                      className={cn(nameFace, 'terminal-input col-start-1 row-start-1 w-full min-w-[2ch] bg-transparent p-0 text-foreground outline-none')}
                      data-slot='terminal-input'
                      maxLength={128}
                      onBlur={() => {
                        if (name.trim() !== ruleSet.name) return;
                        setName(ruleSet.name);
                        setNaming(false);
                      }}
                      onChange={event => setName(event.target.value)}
                      onKeyDown={(event) => {
                        if (event.key === 'Escape') {
                          event.preventDefault();
                          setName(ruleSet.name);
                          setNaming(false);
                        } else if (event.key === 'Enter' && !event.nativeEvent.isComposing) {
                          event.preventDefault();
                          if (!blockedByName()) event.currentTarget.blur();
                        }
                      }}
                      ref={nameRef}
                      size={1}
                      spellCheck={false}
                      value={name}
                    />
                  </span>
                )
              : (
                  // The name edits itself; a narrow editor keeps only it.
                  <button
                    aria-description={t('subscriptions.ruleSets.rename')}
                    className={cn(nameFace, 'block max-w-full cursor-text truncate rounded-[var(--radius-control)] outline-none focus-visible:shadow-[inset_0_0_0_1px_var(--ring)]')}
                    onClick={() => setNaming(true)}
                    type='button'
                  >
                    {name}
                  </button>
                )}
          </h2>
          <IconAction
            className='flex-none @max-md/editor:hidden'
            label={t('subscriptions.ruleSets.rename')}
            onClick={() => showNameInput ? nameRef.current?.focus() : setNaming(true)}
            size='icon-xs'
          >
            <PencilLine aria-hidden='true' />
          </IconAction>
        </div>
        <Tabs className='flex-none gap-0' onValueChange={changeView} value={text === null ? 'entries' : 'text'}>
          <TabsList aria-label={t('subscriptions.ruleSets.view')} variant='pill'>
            <TabsTrigger value='entries'>{t('subscriptions.ruleSets.entries')}</TabsTrigger>
            <TabsTrigger value='text'>{t('subscriptions.ruleSets.text')}</TabsTrigger>
          </TabsList>
        </Tabs>
        <StripDivider className='@max-md/editor:hidden' />
        {dirty
          ? (
              <>
                <span className='text-compact whitespace-nowrap text-[var(--color-text-faint)] tabular-nums @max-2xl/editor:hidden'>
                  {t('subscriptions.ruleSets.unsaved', { count: Math.max(changes, 1) })}
                </span>
                <Button className='@max-xl/editor:w-(--control-height-sm) @max-xl/editor:px-0' onClick={discard} size='sm' type='button' variant='flat'>
                  <Undo2 aria-hidden='true' data-icon='inline-start' />
                  <span className='@max-xl/editor:sr-only'>{t('subscriptions.ruleSets.revert')}</span>
                </Button>
                <StripDivider className='@max-md/editor:hidden' />
              </>
            )
          : null}
        <Button
          aria-busy={save.isPending || undefined}
          className='@max-md/editor:w-(--control-height-sm) @max-md/editor:px-0'
          disabled={!dirty}
          onClick={() => {
            if (save.isPending || blockedByName()) return;
            const now = parseNow();
            if (blockedByText(now)) return;
            save.mutate(now?.rules ?? rules);
          }}
          size='sm'
          type='button'
          variant='flat'
        >
          <Save aria-hidden='true' data-icon='inline-start' />
          <span className='@max-md/editor:sr-only'>{t('subscriptions.ruleSets.save')}</span>
        </Button>
      </div>
      {body}
      {remote
        ? (
            <RuleEditor
              client={ruleSet.client}
              onClose={() => setRemote(null)}
              onSave={(rule) => {
                if (remote.key === null) append([rule]);
                else setRows(rows.map(row => row.key === remote.key ? { ...row, rule } : row));
                setRemote(null);
              }}
              remote
              rule={remote.key === null ? null : rows.find(row => row.key === remote.key)?.rule ?? null}
              takenNames={rows.flatMap(row => row.key !== remote.key && row.rule.remote ? [row.rule.remote.name] : [])}
            />
          )
        : null}
    </section>
  );
}
