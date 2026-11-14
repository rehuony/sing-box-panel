import type { LogLine, Prompt } from '@shared/components/terminal';

import { nextEntryId } from '@shared/components/terminal';

import type { ConsoleStatus } from '@/api/generated';

import type { TrafficRates } from './status-format';

/** What one output of the status command shows. */
export interface Readout {
  now: number;
  rates: TrafficRates | null;
  status: ConsoleStatus | null;
}

/** An output of the status command; the newest one follows the node, earlier ones keep what they showed. */
export interface StatusBlock {
  id: number;
  kind: 'status';
  frozen?: Readout;
}

export type BufferEntry = LogLine | StatusBlock;

/** The console's shell prompt on its host. */
export function shellPrompt(host: string): Prompt {
  return { host, kind: 'shell', path: '~', user: 'console' };
}

export function statusBlock(): StatusBlock {
  return { id: nextEntryId(), kind: 'status' };
}

/** Stops every status output from following the node, keeping what each shows now. */
export function freezeStatus(entries: readonly BufferEntry[], readout: Readout): BufferEntry[] {
  return entries.map(entry => entry.kind === 'status' && !entry.frozen ? { ...entry, frozen: readout } : entry);
}
