import type { PagePath } from '@/routes/page-loaders';

import { nodePath, nodeTabs } from './node-path';

/** The panel scope's windows, in the order the status line numbers them. */
export const panelWindows = ['nodes', 'subscriptions', 'logs', 'settings'] as const;
export type PanelWindow = typeof panelWindows[number];

/** One node's windows, in the order the status line numbers them. */
export const nodeWindows = ['overview', ...nodeTabs] as const;
export type NodeWindow = typeof nodeWindows[number];

/** The translation key of each window's name, which its tooltip, accessible name and completion read. */
export const windowNameKeys: Record<NodeWindow | PanelWindow, string> = {
  configuration: 'node.tabs.configuration',
  cores: 'node.tabs.cores',
  logs: 'nav.logs',
  nodes: 'nav.nodes',
  overview: 'node.tabs.overview',
  settings: 'nav.settings',
  subscriptions: 'nav.subscriptions',
};

/** What the window shows: the fleet, or one node; a path no window shows has none. */
export type ShellScope
  = | { kind: 'fleet'; window: PanelWindow | null }
    | { kind: 'node'; nodeId: string; window: NodeWindow | null };

export const panelWindowPages: Record<PanelWindow, PagePath> = {
  logs: 'logs',
  nodes: 'nodes',
  settings: 'settings',
  subscriptions: 'subscriptions',
};

export const nodeWindowPages: Record<NodeWindow, PagePath> = {
  configuration: 'node-configuration',
  cores: 'node-cores',
  logs: 'node-logs',
  overview: 'node-overview',
  settings: 'node-settings',
};

export function panelWindowPath(window: PanelWindow): string {
  return `/${window}`;
}

export function nodeWindowPath(nodeID: string, window: NodeWindow): string {
  return nodePath(nodeID, window === 'overview' ? undefined : window);
}

function decode(segment: string): string {
  try {
    return decodeURIComponent(segment);
  } catch {
    return segment;
  }
}

/** The scope and window a path opens. */
export function scopeOf(pathname: string): ShellScope {
  const [first, second, third, ...rest] = pathname.split('/').filter(Boolean);
  if (first === 'nodes' && second !== undefined) {
    const nodeId = decode(second);
    if (rest.length > 0) return { kind: 'node', nodeId, window: null };
    if (third === undefined) return { kind: 'node', nodeId, window: 'overview' };
    return { kind: 'node', nodeId, window: (nodeTabs as readonly string[]).includes(third) ? third as NodeWindow : null };
  }
  const window = second === undefined && (panelWindows as readonly string[]).includes(first ?? '') ? first as PanelWindow : null;
  return { kind: 'fleet', window };
}
