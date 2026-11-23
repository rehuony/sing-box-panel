import { useTranslation } from 'react-i18next';

import type { ChannelRouteExit, ChannelRuleGroup, SubscriptionNode } from '@/api/api-client';

/** An exit as a choice's value: `direct`, `reject`, `group:<id>` or `node:<id>`. */
export function exitValue(exit: ChannelRouteExit): string {
  return exit.kind === 'group' || exit.kind === 'node' ? `${exit.kind}:${exit.id ?? ''}` : exit.kind;
}

export function parseExit(value: string): ChannelRouteExit {
  const at = value.indexOf(':');
  if (at < 0) return { kind: value as 'direct' | 'reject' };
  return { kind: value.slice(0, at) as 'group' | 'node', id: value.slice(at + 1) };
}

/** The built-in exits by their client keywords, which stay untranslated. */
export const BUILTIN_EXIT_NAMES = { direct: 'DIRECT', reject: 'REJECT' } as const;

/**
 * Names exits and lists the ones an exit rule can take: DIRECT, REJECT and
 * the enabled groups, then the current exit when it is none of those, such
 * as a pool node, by its display name.
 */
export function useExits(groups: readonly ChannelRuleGroup[], nodes: readonly SubscriptionNode[]) {
  const { t } = useTranslation();
  function label(exit: ChannelRouteExit): string {
    if (exit.kind === 'direct' || exit.kind === 'reject') return BUILTIN_EXIT_NAMES[exit.kind];
    if (exit.kind === 'group') {
      const group = groups.find(value => value.id === exit.id);
      return group ? group.name || t('channels.groupName') : t('channels.missingGroup');
    }
    return nodes.find(node => node.id === exit.id)?.display_name ?? t('channels.missingNode');
  }
  function choices(current: ChannelRouteExit): { value: string; label: string }[] {
    const items = [
      { value: 'direct', label: BUILTIN_EXIT_NAMES.direct },
      { value: 'reject', label: BUILTIN_EXIT_NAMES.reject },
      ...groups.filter(group => group.enabled).map(group => ({ value: exitValue({ kind: 'group', id: group.id }), label: group.name || t('channels.groupName') })),
    ];
    const value = exitValue(current);
    return items.some(item => item.value === value) ? items : [...items, { value, label: label(current) }];
  }
  return { label, choices };
}
