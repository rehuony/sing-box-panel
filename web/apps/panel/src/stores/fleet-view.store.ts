import { create } from 'zustand';

import type { StateFilter } from '@/utils/panel-commands';

export type FleetStateFilter = 'all' | StateFilter;

interface FleetViewState {
  search: string;
  /** Ends the signed-in session's filters, selection and selection mode. */
  reset: () => void;
  /** While on, a click on a pane toggles it instead of opening the node. */
  selecting: boolean;
  tag: string | null;
  /** Where a Shift+click range starts. */
  anchor: string | null;
  /** The home screen's filters, which the command line sets too. */
  state: FleetStateFilter;
  clearFilters: () => void;
  /** Clears the selection and leaves selection mode. */
  clearSelection: () => void;
  /** The nodes the home screen shows, in its order, which select all and ranges follow. */
  visible: readonly string[];
  /** Selected node IDs, in the order they were selected. */
  selected: ReadonlySet<string>;
  select: (ids: readonly string[]) => void;
  /** Enters selection mode, or leaves it and clears the selection. */
  setSelecting: (selecting: boolean) => void;
  /**
   * Sets the nodes the home screen shows and drops the selected ones it no
   * longer shows, so a batch only names nodes the operator can see.
   */
  setVisible: (ids: readonly string[]) => void;
  /**
   * Toggles one node, entering selection mode; with a range, selects every
   * node from the anchor to it in the order given, which is the order the
   * screen shows.
   */
  toggle: (id: string, range?: readonly string[]) => void;
  setFilter: (filter: Partial<Pick<FleetViewState, 'search' | 'state' | 'tag'>>) => void;
}

const cleared = { anchor: null, selected: new Set<string>(), selecting: false };

export const useFleetView = create<FleetViewState>()(set => ({
  anchor: null,
  clearFilters: () => set({ search: '', state: 'all', tag: null }),
  clearSelection: () => set(cleared),
  reset: () => set({ ...cleared, search: '', state: 'all', tag: null, visible: [] }),
  search: '',
  select: ids => set(state => ({ selected: new Set([...state.selected, ...ids]), selecting: true })),
  selected: new Set(),
  selecting: false,
  setFilter: filter => set(filter),
  setSelecting: selecting => set(selecting ? { selecting } : cleared),
  setVisible: visible => set((state) => {
    const shown = new Set(visible);
    const selected = [...state.selected].filter(id => shown.has(id));
    return selected.length === state.selected.size ? { visible } : { selected: new Set(selected), visible };
  }),
  state: 'all',
  tag: null,
  visible: [],
  toggle: (id, range) => set((state) => {
    const start = state.anchor === null || range === undefined ? -1 : range.indexOf(state.anchor);
    const end = range?.indexOf(id) ?? -1;
    if (start >= 0 && end >= 0) {
      const ids = range!.slice(Math.min(start, end), Math.max(start, end) + 1);
      return { anchor: id, selected: new Set([...state.selected, ...ids]), selecting: true };
    }
    const selected = new Set(state.selected);
    if (selected.has(id)) selected.delete(id);
    else selected.add(id);
    return { anchor: id, selected, selecting: true };
  }),
}));
