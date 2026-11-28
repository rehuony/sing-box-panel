import type { CSSProperties } from 'react';
import type { DragStartEvent } from '@dnd-kit/core';

import { useDndMonitor } from '@dnd-kit/core';
import { useCallback, useLayoutEffect, useMemo, useRef, useState } from 'react';

/** A list this long or shorter draws every item; a longer one draws only the items near its view. */
const SORT_WINDOW_FROM = 300;
/** Room drawn beyond the view on each side, so scrolling never shows a gap. */
const OVERSCAN = 960;
/** A row's height until one is drawn. */
const ROW_ESTIMATE = 52;
/** Rows drawn before the list is first measured. */
const FIRST_ROWS = 24;

/** The room of the rows left out between two runs of drawn ones, as one row across the list. */
interface SortGap {
  'aria-hidden': true;
  'data-sort-gap': '';
  'style': CSSProperties;
}

/** What a window draws in order: an item by its index, or a gap. */
type SortSlot = number | SortGap;

interface Layout {
  /** The space between rows. */
  gap: number;
  /** The rows drawn around the view: from `first` up to `last`. */
  last: number;
  first: number;
  /** Items a row holds: the grid's columns, or one in a list. */
  columns: number;
  /** Each row's top from the list's top, then the list's height plus one gap. */
  offsets: readonly number[];
}

const initialLayout: Layout = { columns: 1, gap: 0, offsets: [], first: 0, last: FIRST_ROWS };

/** The nearest ancestor that scrolls the list, or null when the page does. */
function scrollParent(element: HTMLElement): HTMLElement | null {
  for (let node = element.parentElement; node; node = node.parentElement) {
    // A browser gives a visible y axis only beside a visible x axis, so the shorthand then says the same;
    // jsdom leaves the axis visible under a shorthand that scrolls.
    const { overflow, overflowY } = getComputedStyle(node);
    const value = overflowY === 'visible' ? overflow : overflowY;
    if (value === 'auto' || value === 'scroll') return node;
  }
  return null;
}

/** The row at a point this far from the list's top. */
function rowAt(offsets: readonly number[], y: number): number {
  let low = 0;
  let high = Math.max(0, offsets.length - 2);
  while (low < high) {
    const middle = Math.ceil((low + high) / 2);
    if (offsets[middle]! <= y) low = middle;
    else high = middle - 1;
  }
  return low;
}

function sameLayout(left: Layout, right: Layout): boolean {
  return left.columns === right.columns && left.gap === right.gap
    && left.first === right.first && left.last === right.last
    && left.offsets.length === right.offsets.length
    && left.offsets.every((value, index) => value === right.offsets[index]);
}

/**
 * Draws a long list or grid of sortable items only near its view, so only
 * the drawn items mount and register with the sort context; a list up to
 * `SORT_WINDOW_FROM` items draws them all. The rows left out keep their room
 * as the list's padding, so the scrollbar measures the whole list. The item
 * `keep` names, the one being dragged, stays drawn in its row wherever the
 * view goes, the room between them a gap. Rows are measured as they are
 * drawn; a grid's are as tall as each other. The list's own padding is the
 * window's, and its children are the slots, in order.
 */
export function useSortWindow(ids: readonly string[], keep: string | null = null) {
  const windowed = ids.length > SORT_WINDOW_FROM;
  const [list, setList] = useState<HTMLElement | null>(null);
  const [layout, setLayout] = useState(initialLayout);
  const heightsRef = useRef(new Map<string, number>());
  const scrollerRef = useRef<HTMLElement | null>(null);
  const idsRef = useRef(ids);
  const drawnRef = useRef<readonly string[]>([]);

  const measure = useCallback(() => {
    if (!list) return;
    const all = idsRef.current;
    const style = getComputedStyle(list);
    const tracks = style.gridTemplateColumns.split(' ').filter(track => track !== '' && track !== 'none');
    const columns = Math.max(1, tracks.length);
    const gap = Number.parseFloat(style.rowGap) || 0;
    const heights = heightsRef.current;
    let drawn = 0;
    for (const child of list.children) {
      if (child.hasAttribute('data-sort-gap')) continue;
      const id = drawnRef.current[drawn++];
      if (id === undefined) break;
      // A moving item is only translated, which leaves its height.
      const height = child.getBoundingClientRect().height;
      if (height > 0) heights.set(id, height);
    }
    let total = 0;
    for (const height of heights.values()) total += height;
    const estimate = heights.size ? total / heights.size : ROW_ESTIMATE;
    const rows = Math.ceil(all.length / columns);
    const offsets = [0];
    for (let row = 0; row < rows; row += 1) {
      offsets.push(offsets[row]! + (heights.get(all[row * columns]!) ?? estimate) + gap);
    }
    const scroller = scrollerRef.current;
    const view = scroller ? scroller.getBoundingClientRect().top + scroller.clientTop : 0;
    const top = view - list.getBoundingClientRect().top;
    const height = scroller ? scroller.clientHeight : window.innerHeight;
    const next: Layout = {
      columns,
      gap,
      offsets,
      first: rowAt(offsets, top - OVERSCAN),
      last: Math.min(rows, rowAt(offsets, top + height + OVERSCAN) + 1),
    };
    // Measuring the rows just drawn settles them before they paint; an unchanged layout renders nothing.
    // eslint-disable-next-line react/set-state-in-effect
    setLayout(previous => (sameLayout(previous, next) ? previous : next));
  }, [list]);

  // The view follows the scroll and the sizes of the list and its scroller.
  useLayoutEffect(() => {
    if (!windowed || !list) return;
    const scroller = scrollParent(list);
    scrollerRef.current = scroller;
    let frame = 0;
    const schedule = () => {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(measure);
    };
    const target = scroller ?? window;
    target.addEventListener('scroll', schedule, { passive: true });
    const observer = new ResizeObserver(schedule);
    observer.observe(list);
    if (scroller) observer.observe(scroller);
    return () => {
      target.removeEventListener('scroll', schedule);
      observer.disconnect();
      cancelAnimationFrame(frame);
    };
  }, [list, measure, windowed]);

  const drawing = useMemo(() => {
    const count = ids.length;
    if (!windowed) return { drawn: ids, slots: Array.from({ length: count }, (_, index) => index), style: undefined };
    const { columns, gap, offsets } = layout;
    const rows = Math.ceil(count / columns);
    // Offsets measured before the items changed stand in until the list is measured again, before it paints.
    const pitch = offsets.length > 1 ? offsets.at(-1)! / (offsets.length - 1) : ROW_ESTIMATE + gap;
    const rowTop = (row: number) => offsets[row] ?? row * pitch;
    const first = Math.min(layout.first, Math.max(0, rows - 1));
    const last = Math.max(first + 1, Math.min(layout.last, rows));
    const kept = keep === null ? -1 : ids.indexOf(keep);
    const keptRow = kept < 0 ? -1 : Math.floor(kept / columns);
    const runs: [number, number][] = [[first, last]];
    if (keptRow >= 0 && keptRow < first - 1) runs.unshift([keptRow, keptRow + 1]);
    else if (keptRow >= 0 && keptRow === first - 1) runs[0] = [keptRow, last];
    else if (keptRow === last) runs[0] = [first, last + 1];
    else if (keptRow > last) runs.push([keptRow, keptRow + 1]);
    const slots: SortSlot[] = [];
    runs.forEach(([from, to], run) => {
      if (run > 0) {
        const height = rowTop(from) - rowTop(runs[run - 1]![1]) - gap;
        slots.push({ 'aria-hidden': true, 'data-sort-gap': '', 'style': { gridColumn: '1 / -1', height } });
      }
      for (let index = from * columns; index < Math.min(count, to * columns); index += 1) slots.push(index);
    });
    const style: CSSProperties = {
      paddingTop: rowTop(runs[0]![0]),
      paddingBottom: rowTop(rows) - rowTop(runs.at(-1)![1]),
    };
    return { slots, style, drawn: slots.flatMap(slot => (typeof slot === 'number' ? [ids[slot]!] : [])) };
  }, [ids, keep, layout, windowed]);

  // Each drawing is measured before it paints, which settles the rows it drew.
  useLayoutEffect(() => {
    idsRef.current = ids;
    drawnRef.current = drawing.drawn;
    if (windowed) measure();
  });

  return { ref: setList, slots: drawing.slots, style: drawing.style };
}

/** The ID of the item being dragged in the surrounding drag context, or null, for a window to keep drawn. */
export function useDraggedId(): string | null {
  const [dragged, setDragged] = useState<string | null>(null);
  useDndMonitor(useMemo(() => ({
    onDragStart: ({ active }: DragStartEvent) => setDragged(String(active.id)),
    onDragCancel: () => setDragged(null),
    onDragEnd: () => setDragged(null),
  }), []));
  return dragged;
}
