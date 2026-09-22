import { describe, expect, it } from 'vitest';

import type { RuntimeHistoryPage } from '@/api/api-client';

import { buildRuntimeSlots, buildRuntimeTimeline } from '@/pages/dashboard-page/runtime-timeline';

const from = Date.parse('2026-08-30T00:00:00Z');
const to = Date.parse('2026-08-30T12:00:00Z');

function history(overrides: Partial<RuntimeHistoryPage>): RuntimeHistoryPage {
  return {
    history_started_at: '2026-08-29T00:00:00Z',
    items: [],
    ...overrides,
  };
}

describe('buildRuntimeTimeline', () => {
  it('marks the omitted prefix unknown when the snapshot record or byte cap leaves a next cursor', () => {
    const timeline = buildRuntimeTimeline(
      history({
        items: [
          { id: 30, occurred_at: '2026-08-30T08:00:00Z', reason: 'recovered', state: 'running' },
          {
            id: 20,
            occurred_at: '2026-08-30T04:00:00Z',
            reason: 'controlled_stop',
            state: 'stopped',
          },
        ],
        next: { id: 20, occurred_at: '2026-08-30T04:00:00Z' },
        preceding: {
          id: 1,
          occurred_at: '2026-08-29T23:00:00Z',
          reason: 'started',
          state: 'running',
        },
      }),
      from,
      to,
    );

    expect(timeline).toEqual([
      {
        id: `unknown-${from}`,
        start: from,
        end: Date.parse('2026-08-30T04:00:00Z'),
        reason: 'history_unavailable',
        state: 'unknown',
      },
      {
        id: `20-${Date.parse('2026-08-30T04:00:00Z')}`,
        start: Date.parse('2026-08-30T04:00:00Z'),
        end: Date.parse('2026-08-30T08:00:00Z'),
        reason: 'controlled_stop',
        state: 'stopped',
      },
      {
        id: `30-${Date.parse('2026-08-30T08:00:00Z')}`,
        start: Date.parse('2026-08-30T08:00:00Z'),
        end: to,
        reason: 'recovered',
        state: 'running',
      },
    ]);
  });

  it('starts an unknown interval at uncertain_since and supersedes intervening state claims', () => {
    const uncertainSince = Date.parse('2026-08-30T04:00:00Z');
    const timeline = buildRuntimeTimeline(
      history({
        items: [
          {
            id: 12,
            occurred_at: '2026-08-30T08:00:00Z',
            reason: 'inspection_lost',
            state: 'unknown',
            uncertain_since: '2026-08-30T04:00:00Z',
          },
          {
            id: 11,
            occurred_at: '2026-08-30T06:00:00Z',
            reason: 'controlled_stop',
            state: 'stopped',
          },
        ],
        preceding: {
          id: 10,
          occurred_at: '2026-08-29T23:00:00Z',
          reason: 'started',
          state: 'running',
        },
      }),
      from,
      to,
    );

    expect(timeline).toEqual([
      { id: `10-${from}`, start: from, end: uncertainSince, reason: 'started', state: 'running' },
      {
        id: `12-${uncertainSince}`,
        start: uncertainSince,
        end: to,
        reason: 'inspection_lost',
        state: 'unknown',
      },
    ]);
  });

  it('applies equal timestamps in ascending ID order so the newest stable-cursor ID wins', () => {
    const transitionTime = Date.parse('2026-08-30T06:00:00Z');
    const timeline = buildRuntimeTimeline(
      history({
        items: [
          {
            id: 42,
            occurred_at: '2026-08-30T06:00:00Z',
            reason: 'unexpected_exit',
            state: 'failed',
          },
          { id: 41, occurred_at: '2026-08-30T06:00:00Z', reason: 'recovered', state: 'running' },
        ],
        preceding: {
          id: 9,
          occurred_at: '2026-08-29T23:00:00Z',
          reason: 'controlled_stop',
          state: 'stopped',
        },
      }),
      from,
      to,
    );

    expect(timeline).toEqual([
      {
        id: `9-${from}`,
        start: from,
        end: transitionTime,
        reason: 'controlled_stop',
        state: 'stopped',
      },
      {
        id: `42-${transitionTime}`,
        start: transitionTime,
        end: to,
        reason: 'unexpected_exit',
        state: 'failed',
      },
    ]);
  });
});

it('summarizes equal slots conservatively without hiding outages or gaps', () => {
  const start = Date.parse('2026-08-30T00:00:00Z');
  const end = start + 86_400_000;
  const slots = buildRuntimeSlots(
    history({
      preceding: {
        id: 1,
        state: 'running',
        reason: 'started',
        occurred_at: '2026-08-29T23:00:00Z',
      },
      items: [
        { id: 2, state: 'failed', reason: 'crash', occurred_at: '2026-08-30T00:05:00Z' },
        { id: 3, state: 'running', reason: 'started', occurred_at: '2026-08-30T00:06:00Z' },
        { id: 4, state: 'unknown', reason: 'heartbeat_lost', occurred_at: '2026-08-30T00:59:00Z' },
        { id: 5, state: 'running', reason: 'started', occurred_at: '2026-08-30T01:00:00Z' },
      ],
    }),
    start,
    end,
  );
  expect(slots).toHaveLength(48);
  expect(slots.every((slot) => slot.end - slot.start === 1_800_000)).toBe(true);
  expect(slots[0].state).toBe('failed');
  expect(slots[1].state).toBe('unknown');
  expect(slots[2].state).toBe('running');
});
