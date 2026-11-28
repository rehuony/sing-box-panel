import type { Completion, Prompt } from '@shared/components/terminal';

import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { logLine } from '@shared/components/terminal';
import { useQueryClient } from '@tanstack/react-query';

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
import { batchNames, batchNeedsConfirmation, batchTargets, batchWarning, runBatch } from './run-batch';

const CONFIRM_PROMPT: Prompt = { kind: 'confirm' };

/**
 * Runs command lines against the shell. Every command replaces the last
 * one's result with its own, without echoing the line: a command that only
 * opens a window, a node or the filtered fleet closes the line and leaves
 * none. A batch that interrupts, or acts on more than one node, and
 * restarting the panel first ask `[y/N]`, and the answer replaces the
 * question with what follows it.
 */
export function useCommandRunner({ items, prompt, scope }: ShellScopeState) {
  const { t } = useTranslation();
  const client = useApiClient();
  const cache = useQueryClient();
  const navigate = useNavigate();
  const { signOut } = useSignOut();
  const { begin, pending, running, setOpen, setPending } = useCommandSession();

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

  function fail(message: string) {
    begin(logLine('danger', message));
  }

  function run(input: string) {
    const line = input.trim();
    const command = parseCommand(line);
    if (command === null) return;
    // Whatever follows, the last command's result is gone; `clear` asks for nothing more.
    begin();
    const windows = scope.kind === 'node' ? nodeWindows : panelWindows;
    switch (command.kind) {
      case 'help':
        // The verbs in the completion list's columns, then the window numbers.
        begin(
          ...verbCompletions.map(verb => logLine('muted', verb.description ?? '', { term: verb.name, usage: verb.usage })),
          logLine('muted', t('command.help.number'), { term: `1–${windows.length}`, usage: '' }),
        );
        break;
      case 'clear':
        break;
      case 'exit':
        setOpen(false);
        signOut();
        break;
      case 'window': {
        const window = windows[command.position - 1];
        if (window === undefined) {
          fail(t('command.error.noWindow', { position: command.position }));
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
        if (found.length === 0) fail(t('command.error.noNode', { name: command.node }));
        else if (found.length > 1) fail(t('command.error.ambiguous', { name: command.node }));
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
        const matched = resolveTargets(command.target, items);
        const label = t(`command.batch.action.${command.verb}`);
        if (running) {
          fail(t('command.batch.busy'));
          break;
        }
        if (matched.length === 0) {
          fail(t('command.batch.noMatch', { target: command.target }));
          break;
        }
        // As on the selection, the action runs only on the matched nodes it fits.
        const { left, targets: nodes } = batchTargets(command.verb, matched);
        if (nodes.length === 0) {
          fail(t('command.batch.noneFit', { action: label }));
          break;
        }
        if (!batchNeedsConfirmation(command.verb, nodes.length)) {
          void runBatch({ action: command.verb, cache, client, nodes, t });
          break;
        }
        const warning = batchWarning(command.verb, nodes.length, t);
        begin(
          logLine('plain', t('command.batch.confirm', { action: label, count: nodes.length, names: batchNames(nodes, t) })),
          ...(warning ? [logLine('warning', warning)] : []),
          ...(left > 0 ? [logLine('muted', t('command.batch.left', { count: left }))] : []),
        );
        setPending({ action: command.verb, kind: 'batch', nodes });
        break;
      }
      case 'reboot':
        if (usePanelRestart.getState().restarting) {
          fail(t('command.reboot.busy'));
          break;
        }
        begin(logLine('plain', t('command.reboot.confirm')));
        setPending({ kind: 'reboot' });
        break;
      case 'usage':
        fail(t(`command.usage.${command.verb}`));
        break;
      case 'notFound':
        fail(t('command.error.notFound', { command: command.name }));
        break;
    }
  }

  /**
   * Only y or yes runs what the line asked about; anything else cancels it.
   * The prompt outlives closing the line, so another batch or restart may
   * have started since it asked; runBatch and runReboot then refuse it as busy.
   */
  function answer(input: string) {
    const asked = pending;
    setPending(null);
    if (asked === null) return;
    if (!/^y(?:es)?$/i.test(input.trim())) begin(logLine('muted', t('command.batch.cancelled')));
    else if (asked.kind === 'reboot') void runReboot({ cache, client, t });
    else void runBatch({ action: asked.action, cache, client, nodes: asked.nodes, t });
  }

  function cancel() {
    if (pending === null) return;
    begin(logLine('muted', t('command.batch.cancelled')));
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
