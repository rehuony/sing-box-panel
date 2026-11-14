import type { Completion } from '@shared/components/terminal';

import { splitWords } from '@shared/components/terminal';

import type { NodeStatus } from './fleet-summary';
import type { NodeWindow, PanelWindow } from './shell-windows';

import { nodeWindows, panelWindows } from './shell-windows';

/** The commands the panel's command line takes, in the order help lists them. */
export const commandVerbs = ['help', 'open', 'goto', 'filter', 'start', 'stop', 'restart', 'update', 'clear', 'logout', 'exit'] as const;
export type CommandVerb = typeof commandVerbs[number];

/** The runtime and update commands, which act on one or more nodes. */
const batchVerbs = ['start', 'stop', 'restart', 'update'] as const;
type BatchVerb = typeof batchVerbs[number];

const stateFilters = ['online', 'offline', 'pending'] as const;
export type StateFilter = typeof stateFilters[number];

export type PanelCommand
  = | { kind: 'help' }
    | { kind: 'clear' }
    | { kind: 'logout' }
    /** A bare number: the window at that position, from 1. */
    | { kind: 'window'; position: number }
    | { kind: 'open'; window: PanelWindow }
    /** A node by name or ID, at a window or, without one, where the caller decides. */
    | { kind: 'goto'; node: string; window?: NodeWindow }
    /** An empty filter clears every part. */
    | { kind: 'filter'; state: StateFilter | null; tag: string | null; text: string }
    | { kind: 'batch'; verb: BatchVerb; target: string }
    /** The verb is known but its arguments are not. */
    | { kind: 'usage'; verb: CommandVerb }
    | { kind: 'notFound'; name: string };

function isOneOf<T extends string>(values: readonly T[], value: string): value is T {
  return (values as readonly string[]).includes(value);
}

/** Reads one command line; an empty line is no command. */
export function parseCommand(line: string): PanelCommand | null {
  const words = splitWords(line).map(word => word.value);
  const [first, ...rest] = words;
  if (first === undefined) return null;
  const verb = first.toLowerCase();
  if (/^\d+$/.test(verb)) return { kind: 'window', position: Number(verb) };
  if (!isOneOf(commandVerbs, verb)) return { kind: 'notFound', name: first };
  switch (verb) {
    case 'help':
    case 'clear':
      return { kind: verb };
    case 'logout':
    case 'exit':
      return { kind: 'logout' };
    case 'open': {
      const window = rest[0]?.toLowerCase() ?? '';
      return rest.length === 1 && isOneOf(panelWindows, window) ? { kind: 'open', window } : { kind: 'usage', verb };
    }
    case 'goto': {
      const [node, window, ...extra] = rest;
      if (!node || extra.length > 0) return { kind: 'usage', verb };
      if (window === undefined) return { kind: 'goto', node };
      const name = window.toLowerCase();
      return isOneOf(nodeWindows, name) ? { kind: 'goto', node, window: name } : { kind: 'usage', verb };
    }
    case 'filter': {
      let state: StateFilter | null = null;
      let tag: string | null = null;
      const text: string[] = [];
      for (const word of rest) {
        const lower = word.toLowerCase();
        if (lower.startsWith('state:')) {
          const value = lower.slice('state:'.length);
          if (!isOneOf(stateFilters, value)) return { kind: 'usage', verb };
          state = value;
        } else if (lower.startsWith('tag:')) {
          tag = word.slice('tag:'.length);
          if (tag === '') return { kind: 'usage', verb };
        } else {
          text.push(word);
        }
      }
      return { kind: 'filter', state, tag, text: text.join(' ') };
    }
    default: {
      const [target, ...extra] = rest;
      return target && extra.length === 0 ? { kind: 'batch', target, verb } : { kind: 'usage', verb };
    }
  }
}

/** Whether a name matches a glob of `*` and `?`, ignoring case. */
function globMatches(glob: string, name: string): boolean {
  const pattern = glob.split('').map((character) => {
    if (character === '*') return '.*';
    if (character === '?') return '.';
    return character.replace(/[.+^${}()|[\]\\]/g, '\\$&');
  }).join('');
  return new RegExp(`^${pattern}$`, 'iu').test(name);
}

/** The nodes a name or ID names: an exact name, ignoring case, or else an ID. */
export function resolveNode(value: string, items: readonly NodeStatus[]): NodeStatus[] {
  const name = value.toLowerCase();
  const named = items.filter(item => item.node.name.toLowerCase() === name);
  return named.length > 0 ? named : items.filter(item => item.node.id === value);
}

/**
 * The nodes a batch target selects, in the fleet's order: `all`, every node
 * with a tag as `tag:<tag>`, the names a glob such as `hk-*` matches, or one
 * node by name or ID.
 */
export function resolveTargets(target: string, items: readonly NodeStatus[]): NodeStatus[] {
  if (target.toLowerCase() === 'all') return [...items];
  if (target.toLowerCase().startsWith('tag:')) {
    const tag = target.slice('tag:'.length);
    return items.filter(item => item.node.tags.includes(tag));
  }
  if (/[*?]/.test(target)) return items.filter(item => globMatches(target, item.node.name));
  return resolveNode(target, items);
}

/** Every tag the fleet's nodes carry, sorted. */
export function fleetTags(items: readonly NodeStatus[]): string[] {
  return [...new Set(items.flatMap(item => item.node.tags))].sort();
}

interface CompletionContext {
  items: readonly NodeStatus[];
  /** The verbs the line offers, with what each does. */
  verbs: readonly Completion[];
}

/** What Tab completes for a word: verbs first, then windows, node names, tags and targets after them. */
export function commandCompletions(words: readonly string[], index: number, context: CompletionContext): Completion[] {
  if (index === 0) return [...context.verbs];
  const verb = words[0]?.toLowerCase() ?? '';
  const names = (): Completion[] => context.items.map(item => ({
    description: item.node.hostname || undefined,
    name: item.node.name,
  }));
  const tags = () => fleetTags(context.items).map(tag => ({ name: `tag:${tag}` }));
  if (verb === 'open' && index === 1) return panelWindows.map(name => ({ name }));
  if (verb === 'goto') {
    if (index === 1) return names();
    if (index === 2) return nodeWindows.map(name => ({ name }));
  }
  if (verb === 'filter') return [...stateFilters.map(state => ({ name: `state:${state}` })), ...tags()];
  if (isOneOf(batchVerbs, verb) && index === 1) return [{ name: 'all' }, ...tags(), ...names()];
  return [];
}
