import { useCallback, useEffect, useRef, useState } from 'react';

import type { RuntimeStatus, Task } from '@/api/api-client';

import { useApiClient } from '@/api/api-client-context';

export const RUNTIME_VERIFICATION_TIMEOUT_MS = 20_000;
const TASK_TRACKING_TIMEOUT_MS = 60_000;
const POLL_INTERVAL_MS = 750;

export type RuntimeAction = 'restart' | 'start' | 'stop';
export type RuntimeActionPhase
  = | 'idle'
    | 'queueing'
    | 'tracking'
    | 'verifying'
    | 'verified'
    | 'failed'
    | 'task_timeout'
    | 'verification_timeout';

export interface RuntimeControlState {
  task: Task | null;
  error: unknown | null;
  phase: RuntimeActionPhase;
  action: RuntimeAction | null;
}

interface RuntimeControlOptions {
  onRuntimeStatus: (status: RuntimeStatus) => void;
}

const initialState: RuntimeControlState = {
  action: null,
  error: null,
  phase: 'idle',
  task: null,
};

function abortError(): DOMException {
  return new DOMException('Runtime action was aborted', 'AbortError');
}

function delay(milliseconds: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    if (signal.aborted) {
      reject(abortError());
      return;
    }
    let timer = 0;
    const onAbort = () => {
      window.clearTimeout(timer);
      reject(abortError());
    };
    timer = window.setTimeout(() => {
      signal.removeEventListener('abort', onAbort);
      resolve();
    }, milliseconds);
    signal.addEventListener('abort', onAbort, { once: true });
  });
}

function isAbortError(error: unknown): boolean {
  return error instanceof DOMException && error.name === 'AbortError';
}

function isTerminal(task: Task): boolean {
  return task.status !== 'queued' && task.status !== 'running';
}

export function runtimeMatchesAction(
  action: RuntimeAction,
  status: RuntimeStatus,
  previousProcessToken?: string,
): boolean {
  if (action === 'stop') return status.observation_state === 'stopped';
  if (status.observation_state !== 'running' || status.running === undefined) return false;
  if (action !== 'restart' || previousProcessToken === undefined) return true;
  return status.running.process_start_token !== previousProcessToken;
}

export function useRuntimeControl({ onRuntimeStatus }: RuntimeControlOptions) {
  const client = useApiClient();
  const controllerRef = useRef<AbortController | null>(null);
  const [state, setState] = useState<RuntimeControlState>(initialState);

  useEffect(() => () => controllerRef.current?.abort(), []);

  const run = useCallback(async (
    action: RuntimeAction,
    currentRuntime: RuntimeStatus | null,
  ) => {
    controllerRef.current?.abort();
    const controller = new AbortController();
    controllerRef.current = controller;
    const previousProcessToken = currentRuntime?.running?.process_start_token;
    setState({ action, error: null, phase: 'queueing', task: null });

    try {
      let task = await client[`${action}Runtime`](controller.signal);
      setState({ action, error: null, phase: 'tracking', task });

      const taskDeadline = Date.now() + TASK_TRACKING_TIMEOUT_MS;
      while (!isTerminal(task) && Date.now() < taskDeadline) {
        await delay(POLL_INTERVAL_MS, controller.signal);
        try {
          task = await client.getTask(task.id, controller.signal);
          setState({ action, error: null, phase: 'tracking', task });
        } catch (error) {
          if (isAbortError(error)) throw error;
        }
      }

      if (!isTerminal(task)) {
        setState({ action, error: null, phase: 'task_timeout', task });
        return;
      }
      if (task.status !== 'succeeded') {
        setState({ action, error: task.failure ?? null, phase: 'failed', task });
        return;
      }

      setState({ action, error: null, phase: 'verifying', task });
      const verificationDeadline = Date.now() + RUNTIME_VERIFICATION_TIMEOUT_MS;
      let lastError: unknown | null = null;
      while (Date.now() < verificationDeadline) {
        try {
          const runtimeStatus = await client.getRuntimeStatus(controller.signal);
          onRuntimeStatus(runtimeStatus);
          lastError = null;
          if (runtimeMatchesAction(action, runtimeStatus, previousProcessToken)) {
            setState({ action, error: null, phase: 'verified', task });
            return;
          }
        } catch (error) {
          if (isAbortError(error)) throw error;
          lastError = error;
        }
        await delay(POLL_INTERVAL_MS, controller.signal);
      }
      setState({ action, error: lastError, phase: 'verification_timeout', task });
    } catch (error) {
      if (!isAbortError(error)) {
        setState((current) => ({ ...current, error, phase: 'failed' }));
      }
    }
  }, [client, onRuntimeStatus]);

  return {
    busy: state.phase === 'queueing' || state.phase === 'tracking' || state.phase === 'verifying',
    run,
    state,
  };
}
