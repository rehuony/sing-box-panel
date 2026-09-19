import { useTranslation } from 'react-i18next';

import type { ChannelRouteExit, ChannelRuleGroup, SubscriptionNodeSummary } from '@/api/api-client';

interface Props {
  id: string;
  follow?: boolean;
  value: ChannelRouteExit;
  groups?: ChannelRuleGroup[];
  nodes: SubscriptionNodeSummary[];
  onChange: (value: ChannelRouteExit) => void;
}
export function ChannelRouteExitSelect({ id, value, nodes, groups = [], follow, onChange }: Props) {
  const { t } = useTranslation();
  const key = value.id ? `${value.kind}:${value.id}` : value.kind;
  const options = [
    ...(follow ? [{ key: 'group-default', label: t('channels.follow') }] : []),
    { key: 'direct', label: t('channels.direct') },
    { key: 'reject', label: t('channels.reject') },
    ...groups
      .filter((group) => group.enabled)
      .map((group) => ({ key: `group:${group.id}`, label: group.name })),
    ...nodes.map((node) => ({
      key: `node:${node.id}`,
      label: `${node.name}${node.hidden || !node.available ? ` · ${t('channels.unavailable')}` : ''}`,
    })),
  ];
  if (!options.some((option) => option.key === key)) options.push({ key, label: t('channels.missingNode') });
  return (
    <select
      id={id}
      value={key}
      onChange={(event) => {
        const [kind, ...parts] = event.target.value.split(':');
        onChange({
          kind: kind as ChannelRouteExit['kind'],
          ...(parts.length ? { id: parts.join(':') } : {}),
        });
      }}
    >
      {options.map((option) => (
        <option key={option.key} value={option.key}>
          {option.label}
        </option>
      ))}
    </select>
  );
}
