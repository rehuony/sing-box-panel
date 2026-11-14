import type { Completion, Prompt } from '@shared/components/terminal';

import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { echo, logLine } from '@shared/components/terminal';

import type { CommandVerb } from '@/utils/panel-commands';

import { useApiClient } from '@/api/api-client-context';
import { useFleetView } from '@/stores/fleet-view.store';
import { useCommandSession } from '@/stores/command-session.store';
import { nodeWindowPath, nodeWindows, panelWindowPath, panelWindows } from '@/utils/shell-windows';
import { commandCompletions, commandVerbs, parseCommand, resolveNode, resolveTargets } from '@/utils/panel-commands';

import type { ShellScopeState } from './use-shell-scope';

import { useSignOut } from './use-sign-out';
import { batchNames, batchNeedsConfirmation, batchWarning, runBatch } from './run-batch';

const CONFIRM_PROMPT: Prompt = { kind: 'confirm' };

/**
 * Runs command lines against the shell. A command that only opens a window,
 * a node or the filtered fleet closes the line and prints nothing; every
 * other command echoes after its prompt and prints its output below. A batch
 * that interrupts, or acts on more than one node, first asks `[y/N]`.
 */
export function useCommandRunner({ items, prompt, scope }: ShellScopeState) {
  const { t } = useTranslation();
  const client = useApiClient();
  const navigate = useNavigate();
  const { signOut } = useSignOut();
  const { clear, pending, print, running, setOpen, setPending } = useCommandSession();

  const verbCompletions: Completion[] = commandVerbs.map(verb => ({ description: t(`command.help.${verb}`), name: verb }));

  function go(path: string) {
    setOpen(false);
    void navigate(path);
  }

  function fail(line: string, message: string) {
    print(echo(prompt, line), logLine('danger', message));
  }

  function run(input: string) {
    const line = input.trim();
    const command = parseCommand(line);
    if (command === null) return;
    const windows = scope.kind === 'node' ? nodeWindows : panelWindows;
    switch (command.kind) {
      case 'help':
        print(
          echo(prompt, line),
          ...commandVerbs.map((verb: CommandVerb) => logLine('muted', t(`command.help.${verb}`), { term: verb })),
          logLine('muted', t('command.help.number'), { term: `1–${windows.length}` }),
        );
        break;
      case 'clear':
        clear();
        break;
      case 'logout':
        setOpen(false);
        signOut();
        break;
      case 'window': {
        const window = windows[command.position - 1];
        if (window === undefined) fail(line, t('command.error.noWindow', { position: command.position }));
        else if (scope.kind === 'node') go(nodeWindowPath(scope.nodeId, window as typeof nodeWindows[number]));
        else go(panelWindowPath(window as typeof panelWindows[number]));
        break;
      }
      case 'open':
        go(panelWindowPath(command.window));
        break;
      case 'goto': {
        const found = resolveNode(command.node, items);
        const window = command.window ?? (scope.kind === 'node' && scope.window ? scope.window : 'overview');
        if (found.length === 0) fail(line, t('command.error.noNode', { name: command.node }));
        else if (found.length > 1) fail(line, t('command.error.ambiguous', { name: command.node }));
        else go(nodeWindowPath(found[0]!.node.id, window));
        break;
      }
      case 'filter': {
        const { setFilter } = useFleetView.getState();
        setFilter({ search: command.text, state: command.state ?? 'all', tag: command.tag });
        go(panelWindowPath('nodes'));
        break;
      }
      case 'batch': {
        const nodes = resolveTargets(command.target, items);
        if (running) {
          fail(line, t('command.batch.busy'));
          break;
        }
        if (nodes.length === 0) {
          fail(line, t('command.batch.noMatch', { target: command.target }));
          break;
        }
        print(echo(prompt, line));
        if (!batchNeedsConfirmation(command.verb, nodes.length)) {
          void runBatch({ action: command.verb, client, nodes, t });
          break;
        }
        const label = t(`command.batch.action.${command.verb}`);
        print(logLine('plain', t('command.batch.confirm', { action: label, count: nodes.length, names: batchNames(nodes, t) })));
        const warning = batchWarning(command.verb, t);
        if (warning) print(logLine('warning', warning));
        setPending({ action: command.verb, nodes });
        break;
      }
      case 'usage':
        fail(line, t(`command.usage.${command.verb}`));
        break;
      case 'notFound':
        fail(line, t('command.error.notFound', { command: command.name }));
        break;
    }
  }

  /** Only y or yes runs the batch the line asked about; anything else cancels it. */
  function answer(input: string) {
    const reply = input.trim();
    print(echo(CONFIRM_PROMPT, reply));
    const batch = pending;
    setPending(null);
    if (batch === null) return;
    if (/^y(?:es)?$/i.test(reply)) void runBatch({ action: batch.action, client, nodes: batch.nodes, t });
    else print(logLine('muted', t('command.batch.cancelled')));
  }

  function cancel() {
    if (pending === null) return;
    print(echo(CONFIRM_PROMPT, '^C'), logLine('muted', t('command.batch.cancelled')));
    setPending(null);
  }

  return {
    cancel,
    completions: (words: readonly string[], index: number) =>
      commandCompletions(words, index, { items, verbs: verbCompletions }),
    prompt: pending ? CONFIRM_PROMPT : prompt,
    submit: pending ? answer : run,
  };
}
