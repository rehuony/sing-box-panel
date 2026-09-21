import { useCallback, useEffect, useRef, useState } from 'react';

import type { RuntimeStatus } from '@/api/api-client';

import { useApiClient } from '@/api/api-client-context';

export const RUNTIME_VERIFICATION_TIMEOUT_MS = 20_000;
const POLL_INTERVAL_MS = 750;
const SUCCESS_FEEDBACK_DURATION_MS = 2_000;
const ERROR_FEEDBACK_DURATION_MS = 5_000;

export type RuntimeAction = 'restart' | 'start' | 'stop';
export type RuntimeActionPhase
  = | 'idle'
    | 'executing'
    | 'verifying'
    | 'verified'
    | 'failed'
    | 'verification_timeout';

export interface RuntimeControlState {
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
  const busy = state.phase === 'executing' || state.phase === 'verifying';

  useEffect(() => () => controllerRef.current?.abort(), []);

  useEffect(() => {
    if (busy || state.phase === 'idle') return;

    const duration = state.phase === 'verified'
      ? SUCCESS_FEEDBACK_DURATION_MS
      : ERROR_FEEDBACK_DURATION_MS;
    const timer = window.setTimeout(() => {
      setState((current) => current === state ? initialState : current);
    }, duration);
    return () => window.clearTimeout(timer);
  }, [busy, state]);

  const run = useCallback(async (
    action: RuntimeAction,
    currentRuntime: RuntimeStatus | null,
  ) => {
    controllerRef.current?.abort();
    const controller = new AbortController();
    controllerRef.current = controller;
    const previousProcessToken = currentRuntime?.running?.process_start_token;
    setState({ action, error: null, phase: 'executing' });

    try {
      const result = await client[`${action}Runtime`](controller.signal);
      if (controller.signal.aborted) return;
      onRuntimeStatus(result);
      if (runtimeMatchesAction(action, result, previousProcessToken)) {
        setState({ action, error: null, phase: 'verified' });
        return;
      }
      setState({ action, error: null, phase: 'verifying' });
      const verificationDeadline = Date.now() + RUNTIME_VERIFICATION_TIMEOUT_MS;
      let lastError: unknown | null = null;
      while (Date.now() < verificationDeadline) {
        try {
          const runtimeStatus = await client.getRuntimeStatus(controller.signal);
          if (controller.signal.aborted) return;
          onRuntimeStatus(runtimeStatus);
          lastError = null;
          if (runtimeMatchesAction(action, runtimeStatus, previousProcessToken)) {
            setState({ action, error: null, phase: 'verified' });
            return;
          }
        } catch (error) {
          if (isAbortError(error)) throw error;
          lastError = error;
        }
        await delay(POLL_INTERVAL_MS, controller.signal);
      }
      setState({ action, error: lastError, phase: 'verification_timeout' });
    } catch (error) {
      if (!controller.signal.aborted && !isAbortError(error)) {
        setState((current) => ({ ...current, error, phase: 'failed' }));
      }
    }
  }, [client, onRuntimeStatus]);

  return {
    busy,
    run,
    state,
  };
}
