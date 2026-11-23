import { describe, expect, it } from 'vitest';

import type { NodeStatus } from '@/utils/fleet-summary';

import { scopeOf } from '@/utils/shell-windows';
import { testMetrics, testNode } from '@/tests/api/mock-api-client';
import { commandCompletions, parseCommand, resolveNode, resolveTargets } from '@/utils/panel-commands';

function status(id: string, name: string, tags: string[] = []): NodeStatus {
  return { metrics: testMetrics, node: { ...testNode, id, name, tags } };
}

const fleet = [
  status('a', 'hk-1', ['hk', 'premium']),
  status('b', 'hk-2', ['hk']),
  status('c', 'Tokyo edge', ['jp']),
  status('d', 'us-1'),
];

describe('panel commands', () => {
  it('reads verbs, numbers and their arguments', () => {
    expect(parseCommand('  ')).toBeNull();
    expect(parseCommand('2')).toEqual({ kind: 'window', position: 2 });
    expect(parseCommand('OPEN logs')).toEqual({ kind: 'open', window: 'logs' });
    expect(parseCommand('open overview')).toEqual({ kind: 'usage', verb: 'open' });
    expect(parseCommand('goto "Tokyo edge" configuration')).toEqual({ kind: 'goto', node: 'Tokyo edge', window: 'configuration' });
    expect(parseCommand('goto hk-1')).toEqual({ kind: 'goto', node: 'hk-1' });
    expect(parseCommand('goto hk-1 nowhere')).toEqual({ kind: 'usage', verb: 'goto' });
    expect(parseCommand('filter state:offline TAG:HK edge')).toEqual({ kind: 'filter', state: 'offline', tag: 'hk', text: 'edge' });
    expect(parseCommand('filter')).toEqual({ kind: 'filter', state: null, tag: null, text: '' });
    expect(parseCommand('filter state:asleep')).toEqual({ kind: 'usage', verb: 'filter' });
    expect(parseCommand('restart hk-*')).toEqual({ kind: 'batch', target: 'hk-*', verb: 'restart' });
    expect(parseCommand('stop')).toEqual({ kind: 'usage', verb: 'stop' });
    expect(parseCommand('exit')).toEqual({ kind: 'logout' });
    expect(parseCommand('reboot all')).toEqual({ kind: 'notFound', name: 'reboot' });
  });

  it('resolves targets by name, ID, glob, tag and all', () => {
    const ids = (target: string) => resolveTargets(target, fleet).map(item => item.node.id);
    expect(ids('all')).toEqual(['a', 'b', 'c', 'd']);
    expect(ids('Tag:HK')).toEqual(['a', 'b']);
    expect(ids('HK-*')).toEqual(['a', 'b']);
    expect(ids('*edge')).toEqual(['c']);
    expect(ids('us-?')).toEqual(['d']);
    expect(ids('tokyo EDGE')).toEqual(['c']);
    expect(ids('d')).toEqual(['d']);
    expect(ids('tag:none')).toEqual([]);
    expect(resolveNode('missing', fleet)).toEqual([]);
  });

  it('completes verbs, then windows, node names, tags and targets', () => {
    const verbs = [{ name: 'goto' }, { name: 'open' }];
    const names = (words: string[], index: number) =>
      commandCompletions(words, index, { items: fleet, verbs }).map(entry => entry.name);
    expect(names([''], 0)).toEqual(['goto', 'open']);
    expect(names(['open', ''], 1)).toEqual(['nodes', 'subscriptions', 'logs', 'settings']);
    expect(names(['goto', ''], 1)).toEqual(['hk-1', 'hk-2', 'Tokyo edge', 'us-1']);
    expect(names(['goto', 'hk-1', ''], 2)).toEqual(['overview', 'cores', 'configuration', 'logs', 'settings']);
    expect(names(['restart', ''], 1)).toEqual(['all', 'tag:hk', 'tag:jp', 'tag:premium', 'hk-1', 'hk-2', 'Tokyo edge', 'us-1']);
    expect(names(['filter', ''], 1)).toEqual(['state:online', 'state:offline', 'tag:hk', 'tag:jp', 'tag:premium']);
  });

  it('names the scope and window of a path', () => {
    expect(scopeOf('/nodes')).toEqual({ kind: 'fleet', window: 'nodes' });
    expect(scopeOf('/settings')).toEqual({ kind: 'fleet', window: 'settings' });
    expect(scopeOf('/missing')).toEqual({ kind: 'fleet', window: null });
    expect(scopeOf('/nodes/node%20a')).toEqual({ kind: 'node', nodeId: 'node a', window: 'overview' });
    expect(scopeOf('/nodes/a/configuration')).toEqual({ kind: 'node', nodeId: 'a', window: 'configuration' });
    expect(scopeOf('/nodes/a/missing')).toEqual({ kind: 'node', nodeId: 'a', window: null });
  });
});
