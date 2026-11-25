import type { Completion, Prompt } from '@shared/components/terminal';

import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { useQueryClient } from '@tanstack/react-query';
import { echo, logLine } from '@shared/components/terminal';

import type { PagePath } from '@/routes/page-loaders';
import type { CompletionDescriptions } from '@/utils/panel-commands';

import { preloadPage } from '@/routes/page-loaders';
import { useApiClient } from '@/api/api-client-context';
import { useFleetView } from '@/stores/fleet-view.store';
import { coreVersionLabel } from '@/utils/fleet-summary';
import { usePanelRestart } from '@/stores/panel-restart.store';
import { useCommandSession } from '@/stores/command-session.store';
import {
  commandCompletions,
  commandVerbs,
  parseCommand,
  resolveNode,
  resolveTargets,
  takesArguments,
} from '@/utils/panel-commands';
import {
  nodeWindowPages,
  nodeWindowPath,
  nodeWindows,
  panelWindowPages,
  panelWindowPath,
  panelWindows,
  windowNameKeys,
} from '@/utils/shell-windows';

import type { ShellScopeState } from './use-shell-scope';

import { runReboot } from './run-restart';
import { useSignOut } from './use-sign-out';
import { batchNames, batchNeedsConfirmation, batchWarning, runBatch } from './run-batch';

const CONFIRM_PROMPT: Prompt = { kind: 'confirm' };

/**
 * Runs command lines against the shell. A command that only opens a window,
 * a node or the filtered fleet closes the line and prints nothing; every
 * other command echoes after its prompt and prints its output below. A batch
 * that interrupts, or acts on more than one node, and restarting the panel
 * first ask `[y/N]`.
 */
export function useCommandRunner({ items, prompt, scope }: ShellScopeState) {
  const { t } = useTranslation();
  const client = useApiClient();
  const cache = useQueryClient();
  const navigate = useNavigate();
  const { signOut } = useSignOut();
  const { clear, pending, print, running, setOpen, setPending } = useCommandSession();

  // Every verb keeps the arguments column, empty where it takes none.
  const verbCompletions: Completion[] = commandVerbs.map(verb => ({
    description: t(`command.help.${verb}`),
    name: verb,
    usage: takesArguments(verb) ? t(`command.arguments.${verb}`) : '',
  }));
  const describe: CompletionDescriptions = {
    all: t('command.completion.all'),
    node: item => [t(`nodes.state.${item.node.state}`), coreVersionLabel(item)].filter(Boolean).join(' · '),
    tag: tag => t('command.completion.tag', { tag }),
    window: window => t(windowNameKeys[window]),
  };

  /** Opens a window as its link does: the navigation shares the page's preload, which recovers a failed one. */
  function go(path: string, page: PagePath) {
    preloadPage(page);
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
        // The verbs in the completion list's columns, then the window numbers and what a target can be.
        print(
          echo(prompt, line),
          ...verbCompletions.map(verb => logLine('muted', verb.description ?? '', { term: verb.name, usage: verb.usage })),
          logLine('muted', t('command.help.number'), { term: `1–${windows.length}`, usage: '' }),
          logLine('muted', t('command.help.target')),
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
        if (window === undefined) {
          fail(line, t('command.error.noWindow', { position: command.position }));
        } else if (scope.kind === 'node') {
          const nodeWindow = window as typeof nodeWindows[number];
          go(nodeWindowPath(scope.nodeId, nodeWindow), nodeWindowPages[nodeWindow]);
        } else {
          const panelWindow = window as typeof panelWindows[number];
          go(panelWindowPath(panelWindow), panelWindowPages[panelWindow]);
        }
        break;
      }
      case 'open':
        go(panelWindowPath(command.window), panelWindowPages[command.window]);
        break;
      case 'goto': {
        const found = resolveNode(command.node, items);
        const window = command.window ?? (scope.kind === 'node' && scope.window ? scope.window : 'overview');
        if (found.length === 0) fail(line, t('command.error.noNode', { name: command.node }));
        else if (found.length > 1) fail(line, t('command.error.ambiguous', { name: command.node }));
        else go(nodeWindowPath(found[0]!.node.id, window), nodeWindowPages[window]);
        break;
      }
      case 'filter': {
        const { setFilter } = useFleetView.getState();
        setFilter({ search: command.text, state: command.state ?? 'all', tag: command.tag });
        go(panelWindowPath('nodes'), panelWindowPages.nodes);
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
          void runBatch({ action: command.verb, cache, client, nodes, t });
          break;
        }
        const label = t(`command.batch.action.${command.verb}`);
        print(logLine('plain', t('command.batch.confirm', { action: label, count: nodes.length, names: batchNames(nodes, t) })));
        const warning = batchWarning(command.verb, t);
        if (warning) print(logLine('warning', warning));
        setPending({ action: command.verb, kind: 'batch', nodes });
        break;
      }
      case 'reboot':
        if (usePanelRestart.getState().restarting) {
          fail(line, t('command.reboot.busy'));
          break;
        }
        print(echo(prompt, line), logLine('plain', t('command.reboot.confirm')));
        setPending({ kind: 'reboot' });
        break;
      case 'usage':
        fail(line, t(`command.usage.${command.verb}`));
        break;
      case 'notFound':
        fail(line, t('command.error.notFound', { command: command.name }));
        break;
    }
  }

  /**
   * Only y or yes runs what the line asked about; anything else cancels it.
   * The prompt outlives closing the line, so another batch or restart may
   * have started since it asked; runBatch and runReboot then refuse it as busy.
   */
  function answer(input: string) {
    const reply = input.trim();
    print(echo(CONFIRM_PROMPT, reply));
    const asked = pending;
    setPending(null);
    if (asked === null) return;
    if (!/^y(?:es)?$/i.test(reply)) print(logLine('muted', t('command.batch.cancelled')));
    else if (asked.kind === 'reboot') void runReboot({ cache, client, t });
    else void runBatch({ action: asked.action, cache, client, nodes: asked.nodes, t });
  }

  function cancel() {
    if (pending === null) return;
    print(echo(CONFIRM_PROMPT, '^C'), logLine('muted', t('command.batch.cancelled')));
    setPending(null);
  }

  return {
    cancel,
    completions: (words: readonly string[], index: number) =>
      commandCompletions(words, index, { describe, items, verbs: verbCompletions }),
    prompt: pending ? CONFIRM_PROMPT : prompt,
    submit: pending ? answer : run,
  };
}
