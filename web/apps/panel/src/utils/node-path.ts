/** The node workspace's tabs other than its overview, in display order. */
export const nodeTabs = ['cores', 'configuration', 'logs', 'settings'] as const;

export type NodeTab = typeof nodeTabs[number];

export function nodePath(nodeID: string, tab?: NodeTab): string {
  const base = `/nodes/${encodeURIComponent(nodeID)}`;
  return tab === undefined ? base : `${base}/${tab}`;
}
